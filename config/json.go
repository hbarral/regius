package config

import (
	"encoding/json"
	"fmt"
)

// parseJSON parses JSON content into a flat map[string]string.
// Both flat (uppercase env-var-style keys) and nested structures are supported.
//
// Example:
//
//	{
//	  "APP_NAME": "myapp",
//	  "PORT": "8080",
//	  "database": {
//	    "type": "postgres",
//	    "host": "localhost"
//	  }
//	}
func parseJSON(data []byte) (map[string]string, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: JSON parse error: %w", err)
	}

	if raw == nil {
		return map[string]string{}, nil
	}

	return flatten(raw, ""), nil
}
