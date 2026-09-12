package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

// SecretProvider fetches secret values from an external secrets management
// system. Implementations include AWS Secrets Manager, HashiCorp Vault
// (via REST API), and environment variables (for development).
type SecretProvider interface {
	// GetSecret retrieves the secret value at the given path/key.
	GetSecret(path string) (string, error)
}

// SecretsResolver resolves secret references in config values. A secret
// reference uses the format:
//
//	secret://{provider}/{path}
//
// Examples:
//
//	secret://aws/myapp/database-password
//	secret://vault/secret/data/myapp/db
//	secret://env/DB_PASSWORD
//
// The provider name maps to a registered SecretProvider. Values without
// the secret:// prefix are returned as-is.
type SecretsResolver struct {
	providers map[string]SecretProvider
}

// NewSecretsResolver creates a new resolver with no registered providers.
func NewSecretsResolver() *SecretsResolver {
	return &SecretsResolver{
		providers: make(map[string]SecretProvider),
	}
}

// RegisterProvider registers a secret provider under the given name.
// The name is used in secret:// references (e.g., "aws", "vault", "env").
func (r *SecretsResolver) RegisterProvider(name string, provider SecretProvider) {
	r.providers[strings.ToLower(name)] = provider
}

// HasProviders returns true if at least one provider is registered.
func (r *SecretsResolver) HasProviders() bool {
	return len(r.providers) > 0
}

// ResolveValue checks if a value is a secret reference and resolves it.
// Non-secret values are returned as-is. Returns an error if the provider
// is not registered or the secret cannot be fetched.
func (r *SecretsResolver) ResolveValue(value string) (string, error) {
	if !strings.HasPrefix(value, "secret://") {
		return value, nil
	}

	ref := strings.TrimPrefix(value, "secret://")
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("config: invalid secret reference %q: expected secret://{provider}/{path}", value)
	}

	providerName := strings.ToLower(parts[0])
	secretPath := parts[1]

	provider, ok := r.providers[providerName]
	if !ok {
		return "", fmt.Errorf("config: unknown secret provider %q in reference %q", providerName, value)
	}

	secret, err := provider.GetSecret(secretPath)
	if err != nil {
		return "", fmt.Errorf("config: failed to resolve secret %q: %w", value, err)
	}

	return secret, nil
}

// Resolve scans a map of config values and resolves all secret references.
// Non-secret values are passed through unchanged. Returns a new map with
// resolved values.
func (r *SecretsResolver) Resolve(values map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(values))
	for key, val := range values {
		result, err := r.ResolveValue(val)
		if err != nil {
			return nil, fmt.Errorf("config: error resolving secret for key %q: %w", key, err)
		}
		resolved[key] = result
	}
	return resolved, nil
}

// IsSecretReference returns true if the value is a secret reference.
func IsSecretReference(value string) bool {
	return strings.HasPrefix(value, "secret://")
}

// EnvSecretProvider reads secrets from environment variables. It is useful
// for development and testing where no external secrets manager is available.
//
// The path is used directly as the environment variable name.
type EnvSecretProvider struct{}

// NewEnvSecretProvider creates a new EnvSecretProvider.
func NewEnvSecretProvider() *EnvSecretProvider {
	return &EnvSecretProvider{}
}

func (p *EnvSecretProvider) GetSecret(path string) (string, error) {
	value, exists := os.LookupEnv(path)
	if !exists {
		return "", fmt.Errorf("config: env secret %q not found", path)
	}
	return value, nil
}

// AWSSecretsManagerProvider fetches secrets from AWS Secrets Manager using
// the aws-sdk-go v1 library.
type AWSSecretsManagerProvider struct {
	client *secretsmanager.SecretsManager
}

// NewAWSSecretsManagerProvider creates a provider for AWS Secrets Manager
// in the given region. Credentials are resolved from the standard AWS
// credential chain (env vars, shared config, IAM role, etc.).
func NewAWSSecretsManagerProvider(region string) (*AWSSecretsManagerProvider, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("config: failed to create AWS session: %w", err)
	}

	return &AWSSecretsManagerProvider{
		client: secretsmanager.New(sess),
	}, nil
}

func (p *AWSSecretsManagerProvider) GetSecret(path string) (string, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(path),
	}

	result, err := p.client.GetSecretValue(input)
	if err != nil {
		return "", fmt.Errorf("config: AWS Secrets Manager error for %q: %w", path, err)
	}

	if result.SecretString != nil {
		return *result.SecretString, nil
	}

	if result.SecretBinary != nil {
		return string(result.SecretBinary), nil
	}

	return "", fmt.Errorf("config: AWS Secrets Manager returned empty secret for %q", path)
}

// VaultProvider fetches secrets from HashiCorp Vault via its HTTP REST API.
// This avoids pulling in the heavy Vault SDK while supporting the most
// common KV secret engine operations.
//
// For KV v2 secrets, the path should include the mount and data path:
//
//	secret/data/myapp/database
//
// For KV v1 secrets:
//
//	secret/myapp/database
type VaultProvider struct {
	addr   string
	token  string
	client *http.Client
}

// NewVaultProvider creates a provider for HashiCorp Vault.
// The addr should include the scheme and port (e.g., "http://vault:8200").
func NewVaultProvider(addr, token string) *VaultProvider {
	return &VaultProvider{
		addr:  strings.TrimRight(addr, "/"),
		token: token,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *VaultProvider) GetSecret(path string) (string, error) {
	url := fmt.Sprintf("%s/v1/%s", p.addr, path)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("config: failed to create Vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("config: failed to reach Vault at %s: %w", p.addr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("config: Vault returned HTTP %d for %q", resp.StatusCode, path)
	}

	var body struct {
		Data struct {
			Data     map[string]interface{} `json:"data"`
			Metadata map[string]interface{} `json:"metadata"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("config: failed to decode Vault response: %w", err)
	}

	// For KV v2, the actual secret data is nested under data.data.
	// For KV v1, it's under data directly (no nested data field).
	secretData := body.Data.Data
	if len(secretData) == 0 {
		// KV v1: treat metadata fields as the secret data fallback.
		for k, v := range body.Data.Metadata {
			secretData[k] = v
		}
	}

	// If there's a single key, return its value directly.
	// Otherwise, return the JSON-encoded secret data.
	if len(secretData) == 1 {
		for _, v := range secretData {
			return fmt.Sprintf("%v", v), nil
		}
	}

	if len(secretData) > 1 {
		encoded, err := json.Marshal(secretData)
		if err != nil {
			return "", fmt.Errorf("config: failed to encode Vault secret: %w", err)
		}
		return string(encoded), nil
	}

	return "", fmt.Errorf("config: Vault secret %q contains no data", path)
}

// defaultSecretsResolver is the package-level resolver used by setEnvIfNotExists.
var defaultSecretsResolver *SecretsResolver

// SetSecretsResolver sets the package-level secrets resolver. When set,
// all subsequent config loads will resolve secret:// references before
// setting environment variables.
func SetSecretsResolver(r *SecretsResolver) {
	defaultSecretsResolver = r
}

// GetSecretsResolver returns the current package-level secrets resolver,
// or nil if none is set.
func GetSecretsResolver() *SecretsResolver {
	return defaultSecretsResolver
}
