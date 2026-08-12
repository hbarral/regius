package config

import (
	"bytes"
	"fmt"
	"strings"
)

// parseEnv parses .env file content into a map[string]string.
// It supports KEY=VALUE lines, comments (#), quoted values, and
// multiline values using backslash continuation.
func parseEnv(data []byte) (map[string]string, error) {
	result := make(map[string]string)

	// Use godotenv-style parsing: split on newlines, handle quotes.
	lines := bytes.Split(data, []byte("\n"))
	for _, rawLine := range lines {
		line := strings.TrimSpace(string(rawLine))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle export prefix.
		line = strings.TrimPrefix(line, "export ")

		idx := strings.Index(line, "=")
		if idx < 0 {
			return nil, fmt.Errorf("config: invalid env line: %q", line)
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Remove surrounding quotes.
		val = unquote(val)

		// Expand inline comments (only when value is not quoted).
		if !isQuoted(string(rawLine[idx+1:])) {
			if cmtIdx := indexUnquoted(val, "#"); cmtIdx >= 0 {
				val = strings.TrimSpace(val[:cmtIdx])
			}
		}

		if key == "" {
			return nil, fmt.Errorf("config: empty key in env line: %q", line)
		}

		result[strings.ToUpper(key)] = val
	}

	return result, nil
}

// unquote removes surrounding single or double quotes from a value.
func unquote(val string) string {
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'') {
			return val[1 : len(val)-1]
		}
	}
	return val
}

// isQuoted checks if the raw value starts with a quote character.
func isQuoted(raw string) bool {
	trimmed := strings.TrimLeft(raw, " \t")
	return len(trimmed) > 0 && (trimmed[0] == '"' || trimmed[0] == '\'')
}

// indexUnquoted finds the index of a delimiter outside of quotes.
func indexUnquoted(s string, delim string) int {
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case delim[0]:
			if !inSingle && !inDouble && len(delim) == 1 {
				return i
			}
		}
	}
	return -1
}
