package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_NoRules(t *testing.T) {
	v := NewValidator()
	err := v.Validate()
	assert.NoError(t, err)
}

func TestValidator_OptionalUnset(t *testing.T) {
	v := NewValidator()
	v.AddRule(ValidationRule{
		Key:      "OPTIONAL_UNSET_KEY",
		Validate: IsBool,
	})
	err := v.Validate()
	assert.NoError(t, err)
}

func TestValidator_RequiredUnset(t *testing.T) {
	v := NewValidator()
	v.AddRule(ValidationRule{
		Key:      "REQUIRED_UNSET_KEY",
		Required: true,
	})
	err := v.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REQUIRED_UNSET_KEY: is required")
}

func TestValidator_PassesValidation(t *testing.T) {
	t.Setenv("VALID_BOOL_KEY", "true")
	t.Setenv("VALID_INT_KEY", "8080")

	v := NewValidator()
	v.AddRule(ValidationRule{Key: "VALID_BOOL_KEY", Validate: IsBool})
	v.AddRule(ValidationRule{Key: "VALID_INT_KEY", Validate: IsInt})
	err := v.Validate()
	assert.NoError(t, err)
}

func TestValidator_MultipleFailures(t *testing.T) {
	t.Setenv("BAD_BOOL", "notabool")
	t.Setenv("BAD_PORT", "99999")
	t.Setenv("BAD_ENUM", "invalid")

	v := NewValidator()
	v.AddRule(ValidationRule{Key: "BAD_BOOL", Validate: IsBool})
	v.AddRule(ValidationRule{Key: "BAD_PORT", Validate: IsPort})
	v.AddRule(ValidationRule{Key: "BAD_ENUM", Validate: OneOf("a", "b", "c")})
	v.AddRule(ValidationRule{Key: "MISSING_REQUIRED", Required: true})

	err := v.Validate()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "BAD_BOOL")
	assert.Contains(t, msg, "BAD_PORT")
	assert.Contains(t, msg, "BAD_ENUM")
	assert.Contains(t, msg, "MISSING_REQUIRED: is required")
	assert.True(t, strings.Contains(msg, "\n  -"))
}

func TestIsBool(t *testing.T) {
	assert.NoError(t, IsBool("true"))
	assert.NoError(t, IsBool("false"))
	assert.NoError(t, IsBool("TRUE"))
	assert.NoError(t, IsBool("1"))
	assert.NoError(t, IsBool("0"))
	assert.Error(t, IsBool("yes"))
	assert.Error(t, IsBool("notabool"))
}

func TestIsInt(t *testing.T) {
	assert.NoError(t, IsInt("42"))
	assert.NoError(t, IsInt("-1"))
	assert.NoError(t, IsInt("0"))
	assert.Error(t, IsInt("abc"))
	assert.Error(t, IsInt("12.5"))
}

func TestIsPositiveInt(t *testing.T) {
	assert.NoError(t, IsPositiveInt("1"))
	assert.NoError(t, IsPositiveInt("100"))
	assert.Error(t, IsPositiveInt("0"))
	assert.Error(t, IsPositiveInt("-1"))
	assert.Error(t, IsPositiveInt("abc"))
}

func TestIsNonNegInt(t *testing.T) {
	assert.NoError(t, IsNonNegInt("0"))
	assert.NoError(t, IsNonNegInt("100"))
	assert.Error(t, IsNonNegInt("-1"))
	assert.Error(t, IsNonNegInt("abc"))
}

func TestIsPort(t *testing.T) {
	assert.NoError(t, IsPort("80"))
	assert.NoError(t, IsPort("443"))
	assert.NoError(t, IsPort("8080"))
	assert.NoError(t, IsPort("1"))
	assert.NoError(t, IsPort("65535"))
	assert.Error(t, IsPort("0"))
	assert.Error(t, IsPort("65536"))
	assert.Error(t, IsPort("-1"))
	assert.Error(t, IsPort("abc"))
}

func TestIsHTTPStatusCode(t *testing.T) {
	assert.NoError(t, IsHTTPStatusCode("200"))
	assert.NoError(t, IsHTTPStatusCode("404"))
	assert.NoError(t, IsHTTPStatusCode("500"))
	assert.NoError(t, IsHTTPStatusCode("100"))
	assert.NoError(t, IsHTTPStatusCode("599"))
	assert.Error(t, IsHTTPStatusCode("99"))
	assert.Error(t, IsHTTPStatusCode("600"))
	assert.Error(t, IsHTTPStatusCode("abc"))
}

func TestIsIP(t *testing.T) {
	assert.NoError(t, IsIP("127.0.0.1"))
	assert.NoError(t, IsIP("192.168.1.1"))
	assert.NoError(t, IsIP("::1"))
	assert.Error(t, IsIP("notanip"))
	assert.Error(t, IsIP("999.999.999.999"))
}

func TestOneOf(t *testing.T) {
	validate := OneOf("postgres", "mysql", "sqlite")
	assert.NoError(t, validate("postgres"))
	assert.NoError(t, validate("MySQL"))
	assert.NoError(t, validate("SQLITE"))
	assert.Error(t, validate("oracle"))
	assert.Error(t, validate(""))
}

func TestHasLen(t *testing.T) {
	validate := HasLen(32)
	assert.NoError(t, validate("abcdefghijklmnopqrstuvwxyz012345"))
	assert.Error(t, validate("short"))
	assert.Error(t, validate(""))
}

func TestDefaultValidator_AllValid(t *testing.T) {
	save := saveEnv("PORT", "DATABASE_TYPE", "CACHE", "SESSION_TYPE",
		"HASH_ALGORITHM", "SECURE", "DEBUG", "CORS_ENABLED",
		"SECURITY_HEADERS_ENABLED", "API_KEY_AUTH_ENABLED",
		"REQUEST_ID_ENABLED", "I18N_ENABLED", "CORS_MAX_AGE",
		"HSTS_MAX_AGE", "IP_FILTER_STATUS_CODE", "SMTP_PORT")
	defer restoreEnv(save)

	t.Setenv("PORT", "8080")
	t.Setenv("DATABASE_TYPE", "postgres")
	t.Setenv("CACHE", "redis")
	t.Setenv("SESSION_TYPE", "cookie")
	t.Setenv("HASH_ALGORITHM", "bcrypt")
	t.Setenv("SECURE", "true")
	t.Setenv("DEBUG", "true")
	t.Setenv("CORS_ENABLED", "true")
	t.Setenv("SECURITY_HEADERS_ENABLED", "true")
	t.Setenv("API_KEY_AUTH_ENABLED", "false")
	t.Setenv("REQUEST_ID_ENABLED", "true")
	t.Setenv("I18N_ENABLED", "true")
	t.Setenv("CORS_MAX_AGE", "300")
	t.Setenv("HSTS_MAX_AGE", "31536000")
	t.Setenv("IP_FILTER_STATUS_CODE", "403")
	t.Setenv("SMTP_PORT", "587")

	v := DefaultValidator()
	err := v.Validate()
	assert.NoError(t, err)
}

func TestDefaultValidator_InvalidValues(t *testing.T) {
	save := saveEnv("PORT", "DATABASE_TYPE", "CACHE", "HASH_ALGORITHM", "SMTP_PORT")
	defer restoreEnv(save)

	t.Setenv("PORT", "99999")
	t.Setenv("DATABASE_TYPE", "oracle")
	t.Setenv("CACHE", "memcached")
	t.Setenv("HASH_ALGORITHM", "md5")
	t.Setenv("SMTP_PORT", "notaport")

	v := DefaultValidator()
	err := v.Validate()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "PORT")
	assert.Contains(t, msg, "DATABASE_TYPE")
	assert.Contains(t, msg, "CACHE")
	assert.Contains(t, msg, "HASH_ALGORITHM")
	assert.Contains(t, msg, "SMTP_PORT")
}

func TestDefaultValidator_OptionalUnset(t *testing.T) {
	save := saveEnv("PORT", "DATABASE_TYPE", "CACHE", "SESSION_TYPE",
		"HASH_ALGORITHM", "SECURE", "DEBUG", "CORS_ENABLED",
		"SECURITY_HEADERS_ENABLED", "API_KEY_AUTH_ENABLED",
		"REQUEST_ID_ENABLED", "I18N_ENABLED", "CORS_MAX_AGE",
		"HSTS_MAX_AGE", "IP_FILTER_STATUS_CODE", "SMTP_PORT")
	defer restoreEnv(save)

	v := DefaultValidator()
	err := v.Validate()
	assert.NoError(t, err)
}

func TestDefaultValidator_CustomRule(t *testing.T) {
	t.Setenv("CUSTOM_KEY", "abcdefghijklmnopqrstuvwxyz012345")

	v := DefaultValidator()
	v.AddRule(ValidationRule{
		Key:      "CUSTOM_KEY",
		Validate: HasLen(32),
	})
	err := v.Validate()
	assert.NoError(t, err)
}

// saveEnv saves the current values of the given env vars so they can be
// restored later. Returns a map of key → previous value (or ok=false).
func saveEnv(keys ...string) map[string]string {
	saved := make(map[string]string)
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	return saved
}

func restoreEnv(saved map[string]string) {
	for k, v := range saved {
		if v != "" {
			os.Setenv(k, v)
		} else {
			os.Unsetenv(k)
		}
	}
}
