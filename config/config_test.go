package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		path    string
		format  Format
		wantErr bool
	}{
		{".env", FormatEnv, false},
		{"config.env", FormatEnv, false},
		{"config.yaml", FormatYAML, false},
		{"config.yml", FormatYAML, false},
		{"config.json", FormatJSON, false},
		{"config.toml", FormatTOML, false},
		{"config.txt", "", true},
		{"config.ini", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			format, err := DetectFormat(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, Format(""), format)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.format, format)
			}
		})
	}
}

func TestSupportedExtensions(t *testing.T) {
	exts := SupportedExtensions()
	assert.Contains(t, exts, ".env")
	assert.Contains(t, exts, ".yaml")
	assert.Contains(t, exts, ".yml")
	assert.Contains(t, exts, ".json")
	assert.Contains(t, exts, ".toml")
}

func TestParseEnv(t *testing.T) {
	data := []byte(`# Comment line
APP_NAME=myapp
PORT=8080
DEBUG=true
EMPTY=
QUOTED="hello world"
SINGLE='single quote'
export EXPORTED=exported_value
WITH_COMMENT=hello # inline comment
QUOTED_WITH_HASH="a#b"
`)

	result, err := parseEnv(data)
	require.NoError(t, err)

	assert.Equal(t, "myapp", result["APP_NAME"])
	assert.Equal(t, "8080", result["PORT"])
	assert.Equal(t, "true", result["DEBUG"])
	assert.Equal(t, "", result["EMPTY"])
	assert.Equal(t, "hello world", result["QUOTED"])
	assert.Equal(t, "single quote", result["SINGLE"])
	assert.Equal(t, "exported_value", result["EXPORTED"])
	assert.Equal(t, "hello", result["WITH_COMMENT"])
	assert.Equal(t, "a#b", result["QUOTED_WITH_HASH"])
}

func TestParseEnv_Invalid(t *testing.T) {
	data := []byte(`KEY_WITHOUT_VALUE`)
	_, err := parseEnv(data)
	assert.Error(t, err)
}

func TestParseYAML_Flat(t *testing.T) {
	data := []byte(`
APP_NAME: myapp
PORT: "8080"
DEBUG: true
KEY: abc123
`)

	result, err := parseYAML(data)
	require.NoError(t, err)

	assert.Equal(t, "myapp", result["APP_NAME"])
	assert.Equal(t, "8080", result["PORT"])
	assert.Equal(t, "true", result["DEBUG"])
	assert.Equal(t, "abc123", result["KEY"])
}

func TestParseYAML_Nested(t *testing.T) {
	data := []byte(`
app_name: myapp
database:
  type: postgres
  host: localhost
  port: 5432
  ssl_mode: require
redis:
  host: redis.local
  password: secret
`)

	result, err := parseYAML(data)
	require.NoError(t, err)

	assert.Equal(t, "myapp", result["APP_NAME"])
	assert.Equal(t, "postgres", result["DATABASE_TYPE"])
	assert.Equal(t, "localhost", result["DATABASE_HOST"])
	assert.Equal(t, "5432", result["DATABASE_PORT"])
	assert.Equal(t, "require", result["DATABASE_SSL_MODE"])
	assert.Equal(t, "redis.local", result["REDIS_HOST"])
	assert.Equal(t, "secret", result["REDIS_PASSWORD"])
}

func TestParseYAML_Lists(t *testing.T) {
	data := []byte(`
allowed_filetypes:
  - jpg
  - png
  - gif
supported_locales:
  - en
  - es
  - fr
cors:
  allowed_origins:
    - http://localhost:3000
    - http://example.com
`)

	result, err := parseYAML(data)
	require.NoError(t, err)

	assert.Equal(t, "jpg,png,gif", result["ALLOWED_FILETYPES"])
	assert.Equal(t, "en,es,fr", result["SUPPORTED_LOCALES"])
	assert.Equal(t, "http://localhost:3000,http://example.com", result["CORS_ALLOWED_ORIGINS"])
}

func TestParseYAML_Empty(t *testing.T) {
	data := []byte(``)
	result, err := parseYAML(data)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestParseYAML_Invalid(t *testing.T) {
	data := []byte(`: invalid: yaml: [`)
	_, err := parseYAML(data)
	assert.Error(t, err)
}

func TestParseJSON_Flat(t *testing.T) {
	data := []byte(`{
	"APP_NAME": "myapp",
	"PORT": "8080",
	"DEBUG": true,
	"KEY": "abc123"
}`)

	result, err := parseJSON(data)
	require.NoError(t, err)

	assert.Equal(t, "myapp", result["APP_NAME"])
	assert.Equal(t, "8080", result["PORT"])
	assert.Equal(t, "true", result["DEBUG"])
	assert.Equal(t, "abc123", result["KEY"])
}

func TestParseJSON_Nested(t *testing.T) {
	data := []byte(`{
	"app_name": "myapp",
	"database": {
		"type": "postgres",
		"host": "localhost",
		"port": 5432
	},
	"redis": {
		"host": "redis.local",
		"password": "secret"
	}
}`)

	result, err := parseJSON(data)
	require.NoError(t, err)

	assert.Equal(t, "myapp", result["APP_NAME"])
	assert.Equal(t, "postgres", result["DATABASE_TYPE"])
	assert.Equal(t, "localhost", result["DATABASE_HOST"])
	assert.Equal(t, "5432", result["DATABASE_PORT"])
	assert.Equal(t, "redis.local", result["REDIS_HOST"])
	assert.Equal(t, "secret", result["REDIS_PASSWORD"])
}

func TestParseJSON_Lists(t *testing.T) {
	data := []byte(`{
	"allowed_filetypes": ["jpg", "png", "gif"],
	"cors": {
		"allowed_origins": ["http://localhost:3000", "http://example.com"]
	}
}`)

	result, err := parseJSON(data)
	require.NoError(t, err)

	assert.Equal(t, "jpg,png,gif", result["ALLOWED_FILETYPES"])
	assert.Equal(t, "http://localhost:3000,http://example.com", result["CORS_ALLOWED_ORIGINS"])
}

func TestParseJSON_Empty(t *testing.T) {
	data := []byte(`{}`)
	result, err := parseJSON(data)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestParseJSON_Invalid(t *testing.T) {
	data := []byte(`{"key": "value"`)
	_, err := parseJSON(data)
	assert.Error(t, err)
}

func TestParseTOML_Flat(t *testing.T) {
	data := []byte(`
APP_NAME = "myapp"
PORT = "8080"
DEBUG = true
KEY = "abc123"
`)

	result, err := parseTOML(data)
	require.NoError(t, err)

	assert.Equal(t, "myapp", result["APP_NAME"])
	assert.Equal(t, "8080", result["PORT"])
	assert.Equal(t, "true", result["DEBUG"])
	assert.Equal(t, "abc123", result["KEY"])
}

func TestParseTOML_Nested(t *testing.T) {
	data := []byte(`
app_name = "myapp"

[database]
type = "postgres"
host = "localhost"
port = 5432
ssl_mode = "require"

[redis]
host = "redis.local"
password = "secret"
`)

	result, err := parseTOML(data)
	require.NoError(t, err)

	assert.Equal(t, "myapp", result["APP_NAME"])
	assert.Equal(t, "postgres", result["DATABASE_TYPE"])
	assert.Equal(t, "localhost", result["DATABASE_HOST"])
	assert.Equal(t, "5432", result["DATABASE_PORT"])
	assert.Equal(t, "require", result["DATABASE_SSL_MODE"])
	assert.Equal(t, "redis.local", result["REDIS_HOST"])
	assert.Equal(t, "secret", result["REDIS_PASSWORD"])
}

func TestParseTOML_Lists(t *testing.T) {
	data := []byte(`
allowed_filetypes = ["jpg", "png", "gif"]

[cors]
allowed_origins = ["http://localhost:3000", "http://example.com"]
`)

	result, err := parseTOML(data)
	require.NoError(t, err)

	assert.Equal(t, "jpg,png,gif", result["ALLOWED_FILETYPES"])
	assert.Equal(t, "http://localhost:3000,http://example.com", result["CORS_ALLOWED_ORIGINS"])
}

func TestParseTOML_Empty(t *testing.T) {
	data := []byte(``)
	result, err := parseTOML(data)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestParseTOML_Invalid(t *testing.T) {
	data := []byte(`= invalid toml =`)
	_, err := parseTOML(data)
	assert.Error(t, err)
}

func TestLoadFile_Env(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.env")
	err := os.WriteFile(path, []byte("TEST_KEY=env_value\nPORT=3000"), 0644)
	require.NoError(t, err)

	t.Setenv("TEST_KEY", "")
	os.Unsetenv("TEST_KEY")

	err = LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "env_value", os.Getenv("TEST_KEY"))
	assert.Equal(t, "3000", os.Getenv("PORT"))
}

func TestLoadFile_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte("TEST_YAML_KEY: yaml_value\nport: 9090"), 0644)
	require.NoError(t, err)

	os.Unsetenv("TEST_YAML_KEY")
	os.Unsetenv("PORT")

	err = LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "yaml_value", os.Getenv("TEST_YAML_KEY"))
	assert.Equal(t, "9090", os.Getenv("PORT"))
}

func TestLoadFile_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(`{"TEST_JSON_KEY": "json_value", "port": "7070"}`), 0644)
	require.NoError(t, err)

	os.Unsetenv("TEST_JSON_KEY")
	os.Unsetenv("PORT")

	err = LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "json_value", os.Getenv("TEST_JSON_KEY"))
	assert.Equal(t, "7070", os.Getenv("PORT"))
}

func TestLoadFile_TOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	err := os.WriteFile(path, []byte(`TEST_TOML_KEY = "toml_value"`+"\n"+`port = "6060"`), 0644)
	require.NoError(t, err)

	os.Unsetenv("TEST_TOML_KEY")
	os.Unsetenv("PORT")

	err = LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "toml_value", os.Getenv("TEST_TOML_KEY"))
	assert.Equal(t, "6060", os.Getenv("PORT"))
}

func TestLoadFile_EnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte("EXISTING_KEY: from_file"), 0644)
	require.NoError(t, err)

	t.Setenv("EXISTING_KEY", "from_os")

	err = LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "from_os", os.Getenv("EXISTING_KEY"))
}

func TestLoadFile_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	err := os.WriteFile(path, []byte("key=value"), 0644)
	require.NoError(t, err)

	err = LoadFile(path)
	assert.Error(t, err)
}

func TestLoadFile_NotFound(t *testing.T) {
	err := LoadFile("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, ".env"), []byte("FROM_ENV=env_val"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("FROM_YAML: yaml_val"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"FROM_JSON": "json_val"}`), 0644)
	require.NoError(t, err)

	os.Unsetenv("FROM_ENV")
	os.Unsetenv("FROM_YAML")
	os.Unsetenv("FROM_JSON")

	err = LoadDir(dir)
	require.NoError(t, err)

	assert.Equal(t, "env_val", os.Getenv("FROM_ENV"))
	assert.Equal(t, "yaml_val", os.Getenv("FROM_YAML"))
	assert.Equal(t, "json_val", os.Getenv("FROM_JSON"))
}

func TestLoadDir_NotFound(t *testing.T) {
	err := LoadDir("/nonexistent/directory")
	assert.Error(t, err)
}

func TestFlatten(t *testing.T) {
	input := map[string]interface{}{
		"flat_key": "flat_value",
		"nested": map[string]interface{}{
			"key": "nested_value",
			"deep": map[string]interface{}{
				"key": "deep_value",
			},
		},
		"boolean":    true,
		"number":     42,
		"float":      3.14,
		"list":       []interface{}{"a", "b", "c"},
		"hyphen-key": "value",
	}

	result := flatten(input, "")

	assert.Equal(t, "flat_value", result["FLAT_KEY"])
	assert.Equal(t, "nested_value", result["NESTED_KEY"])
	assert.Equal(t, "deep_value", result["NESTED_DEEP_KEY"])
	assert.Equal(t, "true", result["BOOLEAN"])
	assert.Equal(t, "42", result["NUMBER"])
	assert.Equal(t, "3.14", result["FLOAT"])
	assert.Equal(t, "a,b,c", result["LIST"])
	assert.Equal(t, "value", result["HYPHEN_KEY"])
}

func TestFormatFloat(t *testing.T) {
	assert.Equal(t, "8080", formatFloat(8080.0))
	assert.Equal(t, "3.14", formatFloat(3.14))
	assert.Equal(t, "0", formatFloat(0.0))
}

func TestKeyToEnv(t *testing.T) {
	assert.Equal(t, "DATABASE_TYPE", keyToEnv("database-type"))
	assert.Equal(t, "SIMPLE", keyToEnv("simple"))
	assert.Equal(t, "MULTI_WORD_KEY", keyToEnv("multi-word-key"))
}
