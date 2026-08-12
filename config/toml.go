package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// parseTOML parses TOML content into a flat map[string]string.
// Both flat (uppercase env-var-style keys) and nested tables are supported.
//
// Example:
//
//	APP_NAME = "myapp"
//	PORT = "8080"
//
//	[database]
//	type = "postgres"
//	host = "localhost"
func parseTOML(data []byte) (map[string]string, error) {
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: TOML parse error: %w", err)
	}

	if raw == nil {
		return map[string]string{}, nil
	}

	return flatten(raw, ""), nil
}
