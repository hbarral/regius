package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// EncryptedPrefix marks the start of an encrypted config value.
	EncryptedPrefix = "ENC("
	// EncryptedSuffix marks the end of an encrypted config value.
	EncryptedSuffix = ")"
	// ConfigEncryptionKeyEnv is the environment variable that holds the
	// master key used to decrypt ENC(...) values in config files.
	// The key must be exactly 32 bytes (raw or base64-encoded).
	ConfigEncryptionKeyEnv = "CONFIG_ENCRYPTION_KEY"
)

// IsEncryptedValue returns true if the value is an encrypted reference
// wrapped in ENC(...).
func IsEncryptedValue(value string) bool {
	return strings.HasPrefix(value, EncryptedPrefix) && strings.HasSuffix(value, EncryptedSuffix)
}

// DecryptValue decrypts an ENC(...) value using the given 32-byte key.
// The algorithm (AES-CFB with prepended IV, base64 URL encoding) is
// compatible with the framework's Encryption.Encrypt/Decrypt methods.
func DecryptValue(value string, key []byte) (string, error) {
	if !IsEncryptedValue(value) {
		return value, nil
	}

	encrypted := strings.TrimSuffix(strings.TrimPrefix(value, EncryptedPrefix), EncryptedSuffix)
	return decryptAESCFB(encrypted, key)
}

// EncryptValue encrypts a plaintext value and wraps it in ENC(...).
// The resulting string can be placed directly in a config file.
func EncryptValue(plaintext string, key []byte) (string, error) {
	encrypted, err := encryptAESCFB(plaintext, key)
	if err != nil {
		return "", err
	}
	return EncryptedPrefix + encrypted + EncryptedSuffix, nil
}

// decryptAESCFB decrypts a base64-encoded AES-CFB ciphertext.
func decryptAESCFB(cryptoText string, key []byte) (string, error) {
	ct, err := base64.URLEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", fmt.Errorf("config: invalid base64 in encrypted value: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("config: invalid encryption key (must be 32 bytes): %w", err)
	}

	if len(ct) < aes.BlockSize {
		return "", fmt.Errorf("config: encrypted value too short")
	}

	iv := ct[:aes.BlockSize]
	ct = ct[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ct, ct)

	return string(ct), nil
}

// encryptAESCFB encrypts plaintext using AES-CFB and returns base64-encoded
// ciphertext with a prepended IV.
func encryptAESCFB(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("config: invalid encryption key (must be 32 bytes): %w", err)
	}

	cipherText := make([]byte, aes.BlockSize+len(plaintext))
	iv := cipherText[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("config: failed to generate IV: %w", err)
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(cipherText[aes.BlockSize:], []byte(plaintext))

	return base64.URLEncoding.EncodeToString(cipherText), nil
}

// getConfigEncryptionKey reads and validates the config encryption key
// from the CONFIG_ENCRYPTION_KEY environment variable. Returns nil if
// no key is set. The key may be raw 32-byte string or base64-encoded.
func getConfigEncryptionKey() []byte {
	keyStr := os.Getenv(ConfigEncryptionKeyEnv)
	if keyStr == "" {
		return nil
	}

	// Try base64 decode first.
	if decoded, err := base64.StdEncoding.DecodeString(keyStr); err == nil && len(decoded) == 32 {
		return decoded
	}

	// Fall back to raw bytes if the string is exactly 32 characters.
	if len(keyStr) == 32 {
		return []byte(keyStr)
	}

	return nil
}

// processValues resolves secret references and decrypts encrypted values
// in a config map. This is called before setting environment variables
// during config loading and hot-reload.
func processValues(values map[string]string) (map[string]string, error) {
	// 1. Resolve secret:// references.
	if defaultSecretsResolver != nil && defaultSecretsResolver.HasProviders() {
		resolved, err := defaultSecretsResolver.Resolve(values)
		if err != nil {
			return nil, err
		}
		values = resolved
	}

	// 2. Decrypt ENC(...) values.
	key := getConfigEncryptionKey()
	if key != nil {
		for k, v := range values {
			if IsEncryptedValue(v) {
				decrypted, err := DecryptValue(v, key)
				if err != nil {
					return nil, fmt.Errorf("config: failed to decrypt value for key %q: %w", k, err)
				}
				values[k] = decrypted
			}
		}
	}

	return values, nil
}
