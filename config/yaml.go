package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// parseYAML parses YAML content into a flat map[string]string.
// Both flat (uppercase env-var-style keys) and nested structures are supported.
//
// Example flat:
//
//	APP_NAME: myapp
//	PORT: "8080"
//
// Example nested:
//
//	database:
//	  type: postgres
//	  host: localhost
func parseYAML(data []byte) (map[string]string, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: YAML parse error: %w", err)
	}

	if raw == nil {
		return map[string]string{}, nil
	}

	return flatten(raw, ""), nil
}
