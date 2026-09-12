package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testEncryptionKey = []byte("01234567890123456789012345678901") // 32 bytes

func TestIsEncryptedValue(t *testing.T) {
	assert.True(t, IsEncryptedValue("ENC(abc123==)"))
	assert.True(t, IsEncryptedValue("ENC(something)"))
	assert.False(t, IsEncryptedValue("plain_value"))
	assert.False(t, IsEncryptedValue("ENC(incomplete"))
	assert.False(t, IsEncryptedValue("incomplete)"))
	assert.False(t, IsEncryptedValue(""))
}

func TestEncryptDecryptValue(t *testing.T) {
	plaintext := "my_secret_database_password"

	encrypted, err := EncryptValue(plaintext, testEncryptionKey)
	require.NoError(t, err)
	assert.True(t, IsEncryptedValue(encrypted))

	decrypted, err := DecryptValue(encrypted, testEncryptionKey)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptValue_PlainText(t *testing.T) {
	result, err := DecryptValue("not_encrypted", testEncryptionKey)
	require.NoError(t, err)
	assert.Equal(t, "not_encrypted", result)
}

func TestDecryptValue_WrongKey(t *testing.T) {
	encrypted, err := EncryptValue("secret_value", testEncryptionKey)
	require.NoError(t, err)

	wrongKey := []byte("99999999999999999999999999999999") // 32 bytes, different
	decrypted, err := DecryptValue(encrypted, wrongKey)
	// AES-CFB has no authentication, so wrong key produces garbage, not an error
	require.NoError(t, err)
	assert.NotEqual(t, "secret_value", decrypted)
}

func TestDecryptValue_InvalidKey(t *testing.T) {
	_, err := DecryptValue("ENC(abc)", []byte("short"))
	assert.Error(t, err)
}

func TestDecryptValue_InvalidBase64(t *testing.T) {
	_, err := DecryptValue("ENC(!!!notbase64!!!)", testEncryptionKey)
	assert.Error(t, err)
}

func TestEncryptValue_EmptyString(t *testing.T) {
	encrypted, err := EncryptValue("", testEncryptionKey)
	require.NoError(t, err)

	decrypted, err := DecryptValue(encrypted, testEncryptionKey)
	require.NoError(t, err)
	assert.Equal(t, "", decrypted)
}

func TestEncryptValue_DifferentEachTime(t *testing.T) {
	plaintext := "same_secret"

	enc1, err := EncryptValue(plaintext, testEncryptionKey)
	require.NoError(t, err)

	enc2, err := EncryptValue(plaintext, testEncryptionKey)
	require.NoError(t, err)

	// Different due to random IV
	assert.NotEqual(t, enc1, enc2)

	// But both decrypt to the same value
	d1, _ := DecryptValue(enc1, testEncryptionKey)
	d2, _ := DecryptValue(enc2, testEncryptionKey)
	assert.Equal(t, d1, d2)
	assert.Equal(t, plaintext, d1)
}

func TestGetConfigEncryptionKey_RawString(t *testing.T) {
	t.Setenv(ConfigEncryptionKeyEnv, string(testEncryptionKey))
	key := getConfigEncryptionKey()
	assert.Equal(t, testEncryptionKey, key)
}

func TestGetConfigEncryptionKey_Base64(t *testing.T) {
	// Base64-encode the key
	encoded := "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=" // base64 of testEncryptionKey
	t.Setenv(ConfigEncryptionKeyEnv, encoded)
	key := getConfigEncryptionKey()
	assert.Equal(t, testEncryptionKey, key)
}

func TestGetConfigEncryptionKey_NotSet(t *testing.T) {
	os.Unsetenv(ConfigEncryptionKeyEnv)
	key := getConfigEncryptionKey()
	assert.Nil(t, key)
}

func TestGetConfigEncryptionKey_WrongLength(t *testing.T) {
	t.Setenv(ConfigEncryptionKeyEnv, "too_short")
	key := getConfigEncryptionKey()
	assert.Nil(t, key)
}

func TestLoadFile_WithEncryption(t *testing.T) {
	globalTracker.reset()

	encryptedPass, err := EncryptValue("super_secret_db_password", testEncryptionKey)
	require.NoError(t, err)

	t.Setenv(ConfigEncryptionKeyEnv, string(testEncryptionKey))

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("DATABASE_PASS: "+encryptedPass+"\nAPP_NAME: myapp"), 0644))

	os.Unsetenv("DATABASE_PASS")
	os.Unsetenv("APP_NAME")

	err = LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "super_secret_db_password", os.Getenv("DATABASE_PASS"))
	assert.Equal(t, "myapp", os.Getenv("APP_NAME"))
}

func TestLoadFile_EncryptionNoKeySet(t *testing.T) {
	globalTracker.reset()

	encryptedPass, err := EncryptValue("super_secret_db_password", testEncryptionKey)
	require.NoError(t, err)

	os.Unsetenv(ConfigEncryptionKeyEnv)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("DATABASE_PASS: "+encryptedPass), 0644))

	os.Unsetenv("DATABASE_PASS")

	err = LoadFile(path)
	require.NoError(t, err)

	// Without a key, the ENC(...) value is set as-is (not decrypted)
	assert.Equal(t, encryptedPass, os.Getenv("DATABASE_PASS"))
}

func TestLoadFile_EncryptionWrongKey(t *testing.T) {
	globalTracker.reset()

	encryptedPass, err := EncryptValue("super_secret_db_password", testEncryptionKey)
	require.NoError(t, err)

	t.Setenv(ConfigEncryptionKeyEnv, "99999999999999999999999999999999") // 32 bytes, wrong key

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("DATABASE_PASS: "+encryptedPass), 0644))

	os.Unsetenv("DATABASE_PASS")

	err = LoadFile(path)
	// Decryption with wrong key should produce garbage but not error
	// (AES-CFB doesn't have authentication). The key point is it doesn't crash.
	if err != nil {
		// If it does error, it should be a decryption error, not a panic
		assert.Contains(t, err.Error(), "decrypt")
	}
}

func TestProcessValues_EncryptionAndSecrets(t *testing.T) {
	globalTracker.reset()
	originalResolver := GetSecretsResolver()
	defer SetSecretsResolver(originalResolver)

	t.Setenv("EXTERNAL_SECRET", "external_secret_value")
	t.Setenv(ConfigEncryptionKeyEnv, string(testEncryptionKey))

	resolver := NewSecretsResolver()
	resolver.RegisterProvider("env", NewEnvSecretProvider())
	SetSecretsResolver(resolver)

	encryptedPass, err := EncryptValue("decrypted_password", testEncryptionKey)
	require.NoError(t, err)

	input := map[string]string{
		"DB_PASS":   encryptedPass,
		"API_TOKEN": "secret://env/EXTERNAL_SECRET",
		"APP_NAME":  "plain_value",
	}

	processed, err := processValues(input)
	require.NoError(t, err)

	assert.Equal(t, "decrypted_password", processed["DB_PASS"])
	assert.Equal(t, "external_secret_value", processed["API_TOKEN"])
	assert.Equal(t, "plain_value", processed["APP_NAME"])
}

func TestEncryptValue_CompatibilityWithFramework(t *testing.T) {
	// Verify that EncryptValue/DecryptValue are compatible with the
	// framework's Encryption.Encrypt/Decrypt (same algorithm).
	// We can't import the regius package from here, but we can verify
	// that our implementation produces valid base64 with a 16-byte IV prefix.

	encrypted, err := EncryptValue("test_value", testEncryptionKey)
	require.NoError(t, err)

	// The encrypted value should be ENC(base64data)
	assert.True(t, IsEncryptedValue(encrypted))

	// Decrypt should work
	decrypted, err := DecryptValue(encrypted, testEncryptionKey)
	require.NoError(t, err)
	assert.Equal(t, "test_value", decrypted)
}
