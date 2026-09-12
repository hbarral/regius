package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// ValidationRule defines a single configuration validation rule.
type ValidationRule struct {
	Key         string
	Description string
	Required    bool
	Validate    func(value string) error
}

// Validator holds a set of validation rules and checks them against the
// current process environment.
type Validator struct {
	rules []ValidationRule
}

// NewValidator creates an empty Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// AddRule adds a validation rule to the validator.
func (v *Validator) AddRule(rule ValidationRule) {
	v.rules = append(v.rules, rule)
}

// Validate checks all rules against the current environment and returns a
// combined error listing every failure. Returns nil if all rules pass.
func (v *Validator) Validate() error {
	var failures []string
	for _, rule := range v.rules {
		value := os.Getenv(rule.Key)
		if value == "" {
			if rule.Required {
				failures = append(failures, fmt.Sprintf("%s: is required", rule.Key))
			}
			continue
		}
		if rule.Validate != nil {
			if err := rule.Validate(value); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %s", rule.Key, err))
			}
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("config validation failed:\n  - %s", strings.Join(failures, "\n  - "))
}

// DefaultValidator returns a Validator pre-configured with rules for the
// standard Regius environment variables. Applications can extend it with
// additional rules before calling Validate.
func DefaultValidator() *Validator {
	v := NewValidator()

	v.AddRule(ValidationRule{
		Key:         "PORT",
		Description: "server listen port",
		Validate:    IsPort,
	})

	v.AddRule(ValidationRule{
		Key:         "DATABASE_TYPE",
		Description: "database driver",
		Validate:    OneOf("postgres", "postgresql", "mysql", "mariadb", "sqlite", "sqlite3"),
	})

	v.AddRule(ValidationRule{
		Key:         "CACHE",
		Description: "cache backend",
		Validate:    OneOf("redis", "badger"),
	})

	v.AddRule(ValidationRule{
		Key:         "SESSION_TYPE",
		Description: "session store backend",
		Validate:    OneOf("cookie", "redis", "mysql", "postgres", "postgresql", "mariadb"),
	})

	v.AddRule(ValidationRule{
		Key:         "HASH_ALGORITHM",
		Description: "password hashing algorithm",
		Validate:    OneOf("bcrypt", "scrypt", "argon2"),
	})

	v.AddRule(ValidationRule{
		Key:         "SECURE",
		Description: "HTTPS mode toggle",
		Validate:    IsBool,
	})

	v.AddRule(ValidationRule{
		Key:         "DEBUG",
		Description: "debug mode toggle",
		Validate:    IsBool,
	})

	v.AddRule(ValidationRule{
		Key:         "CORS_ENABLED",
		Description: "CORS middleware toggle",
		Validate:    IsBool,
	})

	v.AddRule(ValidationRule{
		Key:         "SECURITY_HEADERS_ENABLED",
		Description: "security headers toggle",
		Validate:    IsBool,
	})

	v.AddRule(ValidationRule{
		Key:         "API_KEY_AUTH_ENABLED",
		Description: "API key auth toggle",
		Validate:    IsBool,
	})

	v.AddRule(ValidationRule{
		Key:         "REQUEST_ID_ENABLED",
		Description: "request ID tracing toggle",
		Validate:    IsBool,
	})

	v.AddRule(ValidationRule{
		Key:         "I18N_ENABLED",
		Description: "i18n toggle",
		Validate:    IsBool,
	})

	v.AddRule(ValidationRule{
		Key:         "CORS_MAX_AGE",
		Description: "CORS max age in seconds",
		Validate:    IsPositiveInt,
	})

	v.AddRule(ValidationRule{
		Key:         "HSTS_MAX_AGE",
		Description: "HSTS max age in seconds",
		Validate:    IsNonNegInt,
	})

	v.AddRule(ValidationRule{
		Key:         "IP_FILTER_STATUS_CODE",
		Description: "IP filter rejection status code",
		Validate:    IsHTTPStatusCode,
	})

	v.AddRule(ValidationRule{
		Key:         "SMTP_PORT",
		Description: "SMTP server port",
		Validate:    IsPort,
	})

	return v
}

// IsBool checks that the value is a valid boolean string.
func IsBool(value string) error {
	_, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("must be true or false, got %q", value)
	}
	return nil
}

// IsInt checks that the value is a valid integer.
func IsInt(value string) error {
	_, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be an integer, got %q", value)
	}
	return nil
}

// IsPositiveInt checks that the value is a positive integer (> 0).
func IsPositiveInt(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a positive integer, got %q", value)
	}
	if n <= 0 {
		return fmt.Errorf("must be greater than 0, got %d", n)
	}
	return nil
}

// IsNonNegInt checks that the value is a non-negative integer (>= 0).
func IsNonNegInt(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a non-negative integer, got %q", value)
	}
	if n < 0 {
		return fmt.Errorf("must be >= 0, got %d", n)
	}
	return nil
}

// IsPort checks that the value is a valid TCP port number (1-65535).
func IsPort(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a valid port number, got %q", value)
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("must be between 1 and 65535, got %d", n)
	}
	return nil
}

// IsHTTPStatusCode checks that the value is a valid HTTP status code (100-599).
func IsHTTPStatusCode(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a valid HTTP status code, got %q", value)
	}
	if n < 100 || n > 599 {
		return fmt.Errorf("must be between 100 and 599, got %d", n)
	}
	return nil
}

// IsIP checks that the value is a valid IP address.
func IsIP(value string) error {
	if net.ParseIP(value) == nil {
		return fmt.Errorf("must be a valid IP address, got %q", value)
	}
	return nil
}

// OneOf returns a validation function that checks the value is in the
// allowed set (case-insensitive).
func OneOf(allowed ...string) func(string) error {
	return func(value string) error {
		lower := strings.ToLower(value)
		for _, a := range allowed {
			if strings.ToLower(a) == lower {
				return nil
			}
		}
		return fmt.Errorf("must be one of %s, got %q", strings.Join(allowed, ", "), value)
	}
}

// HasLen returns a validation function that checks the value has the exact
// given length.
func HasLen(length int) func(string) error {
	return func(value string) error {
		if len(value) != length {
			return fmt.Errorf("must be exactly %d characters, got %d", length, len(value))
		}
		return nil
	}
}
