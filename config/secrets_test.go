package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSecretReference(t *testing.T) {
	assert.True(t, IsSecretReference("secret://aws/myapp/db-password"))
	assert.True(t, IsSecretReference("secret://vault/secret/data/myapp"))
	assert.True(t, IsSecretReference("secret://env/DB_PASSWORD"))
	assert.False(t, IsSecretReference("plain_value"))
	assert.False(t, IsSecretReference("http://example.com"))
	assert.False(t, IsSecretReference(""))
}

func TestSecretsResolver_NoProviders(t *testing.T) {
	r := NewSecretsResolver()
	assert.False(t, r.HasProviders())

	result, err := r.ResolveValue("plain_value")
	require.NoError(t, err)
	assert.Equal(t, "plain_value", result)
}

func TestSecretsResolver_ResolveValue_Plain(t *testing.T) {
	r := NewSecretsResolver()
	r.RegisterProvider("env", NewEnvSecretProvider())

	result, err := r.ResolveValue("not_a_secret")
	require.NoError(t, err)
	assert.Equal(t, "not_a_secret", result)
}

func TestSecretsResolver_ResolveValue_Secret(t *testing.T) {
	t.Setenv("MY_SECRET_KEY", "super_secret_value")

	r := NewSecretsResolver()
	r.RegisterProvider("env", NewEnvSecretProvider())

	result, err := r.ResolveValue("secret://env/MY_SECRET_KEY")
	require.NoError(t, err)
	assert.Equal(t, "super_secret_value", result)
}

func TestSecretsResolver_ResolveValue_UnknownProvider(t *testing.T) {
	r := NewSecretsResolver()
	r.RegisterProvider("env", NewEnvSecretProvider())

	_, err := r.ResolveValue("secret://aws/myapp/db")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown secret provider")
}

func TestSecretsResolver_ResolveValue_InvalidFormat(t *testing.T) {
	r := NewSecretsResolver()

	_, err := r.ResolveValue("secret://")
	assert.Error(t, err)

	_, err = r.ResolveValue("secret://env")
	assert.Error(t, err)

	_, err = r.ResolveValue("secret://env/")
	assert.Error(t, err)
}

func TestSecretsResolver_Resolve(t *testing.T) {
	t.Setenv("SECRET_DB_PASS", "db_password_123")
	t.Setenv("SECRET_REDIS_PASS", "redis_password_456")

	r := NewSecretsResolver()
	r.RegisterProvider("env", NewEnvSecretProvider())

	input := map[string]string{
		"DATABASE_PASS": "secret://env/SECRET_DB_PASS",
		"REDIS_PASS":    "secret://env/SECRET_REDIS_PASS",
		"APP_NAME":      "myapp",
		"DEBUG":         "true",
	}

	resolved, err := r.Resolve(input)
	require.NoError(t, err)

	assert.Equal(t, "db_password_123", resolved["DATABASE_PASS"])
	assert.Equal(t, "redis_password_456", resolved["REDIS_PASS"])
	assert.Equal(t, "myapp", resolved["APP_NAME"])
	assert.Equal(t, "true", resolved["DEBUG"])
}

func TestSecretsResolver_Resolve_Error(t *testing.T) {
	r := NewSecretsResolver()
	r.RegisterProvider("env", NewEnvSecretProvider())

	os.Unsetenv("NONEXISTENT_SECRET_12345")

	input := map[string]string{
		"DB_PASS": "secret://env/NONEXISTENT_SECRET_12345",
	}

	_, err := r.Resolve(input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PASS")
}

func TestEnvSecretProvider(t *testing.T) {
	t.Setenv("ENV_PROVIDER_TEST_KEY", "env_secret_value")

	p := NewEnvSecretProvider()
	result, err := p.GetSecret("ENV_PROVIDER_TEST_KEY")
	require.NoError(t, err)
	assert.Equal(t, "env_secret_value", result)
}

func TestEnvSecretProvider_NotFound(t *testing.T) {
	os.Unsetenv("DEFINITELY_NONEXISTENT_SECRET_67890")

	p := NewEnvSecretProvider()
	_, err := p.GetSecret("DEFINITELY_NONEXISTENT_SECRET_67890")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestVaultProvider_KVv2_SingleKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))
		assert.Equal(t, "/v1/secret/data/myapp/db", r.URL.Path)

		response := map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"password": "vault_secret_password",
				},
				"metadata": map[string]interface{}{
					"version": 1,
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewVaultProvider(server.URL, "test-token")
	result, err := provider.GetSecret("secret/data/myapp/db")
	require.NoError(t, err)
	assert.Equal(t, "vault_secret_password", result)
}

func TestVaultProvider_KVv2_MultipleKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"username": "dbuser",
					"password": "dbpass",
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewVaultProvider(server.URL, "test-token")
	result, err := provider.GetSecret("secret/data/myapp/db")
	require.NoError(t, err)

	var secrets map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result), &secrets))
	assert.Equal(t, "dbuser", secrets["username"])
	assert.Equal(t, "dbpass", secrets["password"])
}

func TestVaultProvider_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := NewVaultProvider(server.URL, "test-token")
	_, err := provider.GetSecret("secret/data/nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestVaultProvider_ConnectionError(t *testing.T) {
	provider := NewVaultProvider("http://127.0.0.1:1", "test-token")
	_, err := provider.GetSecret("secret/data/test")
	assert.Error(t, err)
}

func TestSetGetSecretsResolver(t *testing.T) {
	original := GetSecretsResolver()
	defer SetSecretsResolver(original)

	r := NewSecretsResolver()
	r.RegisterProvider("env", NewEnvSecretProvider())
	SetSecretsResolver(r)

	assert.Equal(t, r, GetSecretsResolver())

	SetSecretsResolver(nil)
	assert.Nil(t, GetSecretsResolver())
}

func TestLoadFile_WithSecrets(t *testing.T) {
	globalTracker.reset()
	original := GetSecretsResolver()
	defer SetSecretsResolver(original)

	t.Setenv("SECRET_DB_PASSWORD", "resolved_db_password")

	resolver := NewSecretsResolver()
	resolver.RegisterProvider("env", NewEnvSecretProvider())
	SetSecretsResolver(resolver)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("DB_PASSWORD: secret://env/SECRET_DB_PASSWORD\nAPP_NAME: testapp"), 0644))

	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("APP_NAME")

	err := LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "resolved_db_password", os.Getenv("DB_PASSWORD"))
	assert.Equal(t, "testapp", os.Getenv("APP_NAME"))
}

func TestLoadFile_SecretsResolverNotSet(t *testing.T) {
	globalTracker.reset()
	original := GetSecretsResolver()
	defer SetSecretsResolver(original)
	SetSecretsResolver(nil)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("DB_PASSWORD: secret://env/SOME_SECRET\nAPP_NAME: testapp"), 0644))

	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("APP_NAME")

	err := LoadFile(path)
	require.NoError(t, err)

	// Without a resolver, the secret:// reference is set as-is
	assert.Equal(t, "secret://env/SOME_SECRET", os.Getenv("DB_PASSWORD"))
	assert.Equal(t, "testapp", os.Getenv("APP_NAME"))
}
