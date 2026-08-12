package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Format represents a supported configuration file format.
type Format string

const (
	FormatEnv  Format = "env"
	FormatYAML Format = "yaml"
	FormatJSON Format = "json"
	FormatTOML Format = "toml"
)

// supportedExtensions maps file extensions to their formats.
var supportedExtensions = map[string]Format{
	".env":  FormatEnv,
	".yaml": FormatYAML,
	".yml":  FormatYAML,
	".json": FormatJSON,
	".toml": FormatTOML,
}

// SupportedExtensions returns the list of supported file extensions.
func SupportedExtensions() []string {
	exts := make([]string, 0, len(supportedExtensions))
	for ext := range supportedExtensions {
		exts = append(exts, ext)
	}
	return exts
}

// DetectFormat determines the config format from a file path extension.
// Returns FormatEnv for files named ".env", ending in ".env", or matching
// the ".env.{profile}" pattern (e.g., ".env.dev").
func DetectFormat(path string) (Format, error) {
	base := filepath.Base(path)
	if base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env") {
		return FormatEnv, nil
	}
	ext := filepath.Ext(path)
	fmt2, ok := supportedExtensions[ext]
	if !ok {
		return "", fmt.Errorf("config: unsupported file extension %q for %q", ext, path)
	}
	return fmt2, nil
}

// LoadFile loads a single configuration file and populates os.Environ.
// Existing environment variables are NOT overridden by values from the file.
func LoadFile(path string) error {
	values := make(map[string]string)
	if err := loadIntoMap(path, values); err != nil {
		return err
	}
	setEnvIfNotExists(values)
	return nil
}

// loadIntoMap parses a config file and merges its values into target.
// Later values overwrite earlier ones, allowing callers to layer base
// and profile configs before calling setEnvIfNotExists.
func loadIntoMap(path string, target map[string]string) error {
	format, err := DetectFormat(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: failed to read %s: %w", path, err)
	}

	values, err := parse(data, format)
	if err != nil {
		return fmt.Errorf("config: failed to parse %s: %w", path, err)
	}

	for k, v := range values {
		target[k] = v
	}
	return nil
}

// LoadDir loads all supported configuration files from a directory.
// Files are sorted alphabetically so that later files can override earlier
// ones (but none override existing OS environment variables).
func LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("config: failed to read directory %s: %w", dir, err)
	}

	values := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == ".env" || strings.HasSuffix(name, ".env") {
			if err := loadIntoMap(filepath.Join(dir, name), values); err != nil {
				return err
			}
			continue
		}
		ext := filepath.Ext(name)
		if _, ok := supportedExtensions[ext]; ok {
			if err := loadIntoMap(filepath.Join(dir, name), values); err != nil {
				return err
			}
		}
	}

	setEnvIfNotExists(values)
	return nil
}

// parse dispatches to the appropriate parser based on format.
func parse(data []byte, format Format) (map[string]string, error) {
	switch format {
	case FormatEnv:
		return parseEnv(data)
	case FormatYAML:
		return parseYAML(data)
	case FormatJSON:
		return parseJSON(data)
	case FormatTOML:
		return parseTOML(data)
	default:
		return nil, fmt.Errorf("config: unknown format %q", format)
	}
}

// setEnvIfNotExists sets environment variables only when they are not already
// defined. This preserves the standard precedence: OS env vars > config files.
func setEnvIfNotExists(values map[string]string) {
	for key, val := range values {
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

// flatten converts a nested map[string]interface{} into a flat map[string]string
// by joining keys with underscores and uppercasing the result.
//
// Example:
//
//	{"database": {"type": "postgres", "host": "localhost"}}
//	→ {"DATABASE_TYPE": "postgres", "DATABASE_HOST": "localhost"}
func flatten(m map[string]interface{}, prefix string) map[string]string {
	result := make(map[string]string)
	for key, val := range m {
		fullKey := strings.ToUpper(prefix + keyToEnv(key))
		switch v := val.(type) {
		case map[string]interface{}:
			for k, v2 := range flatten(v, fullKey+"_") {
				result[k] = v2
			}
		case map[interface{}]interface{}:
			converted := make(map[string]interface{})
			for mk, mv := range v {
				converted[fmt.Sprintf("%v", mk)] = mv
			}
			for k, v2 := range flatten(converted, fullKey+"_") {
				result[k] = v2
			}
		case []interface{}:
			result[fullKey] = joinSlice(v)
		case string:
			result[fullKey] = v
		case bool:
			result[fullKey] = fmt.Sprintf("%t", v)
		case int:
			result[fullKey] = fmt.Sprintf("%d", v)
		case int64:
			result[fullKey] = fmt.Sprintf("%d", v)
		case float64:
			result[fullKey] = formatFloat(v)
		case nil:
			result[fullKey] = ""
		default:
			result[fullKey] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// keyToEnv converts a config key to env-var convention: uppercase with
// hyphens replaced by underscores.
func keyToEnv(key string) string {
	return strings.ReplaceAll(strings.ToUpper(key), "-", "_")
}

// joinSlice converts a slice of values to a comma-separated string.
func joinSlice(slice []interface{}) string {
	parts := make([]string, 0, len(slice))
	for _, item := range slice {
		switch v := item.(type) {
		case string:
			parts = append(parts, v)
		case bool:
			parts = append(parts, fmt.Sprintf("%t", v))
		case int:
			parts = append(parts, fmt.Sprintf("%d", v))
		case int64:
			parts = append(parts, fmt.Sprintf("%d", v))
		case float64:
			parts = append(parts, formatFloat(v))
		default:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	return strings.Join(parts, ",")
}

// formatFloat formats a float64 as a string, dropping the decimal part
// when the value is a whole number (e.g. 8080.0 → "8080").
func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
