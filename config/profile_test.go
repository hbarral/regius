package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileFilename(t *testing.T) {
	tests := []struct {
		path     string
		profile  string
		expected string
	}{
		{".env", "dev", ".env.dev"},
		{"config.yaml", "dev", "config.dev.yaml"},
		{"config.yml", "staging", "config.staging.yml"},
		{"config.json", "prod", "config.prod.json"},
		{"config.toml", "prod", "config.prod.toml"},
		{"/app/config.yaml", "dev", "/app/config.dev.yaml"},
		{"config.yaml", "", "config.yaml"},
		{".env", "", ".env"},
	}

	for _, tt := range tests {
		t.Run(tt.path+"+"+tt.profile, func(t *testing.T) {
			result := ProfileFilename(tt.path, tt.profile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetProfile(t *testing.T) {
	t.Setenv(ProfileEnvVar, "staging")
	assert.Equal(t, "staging", GetProfile())
}

func TestGetProfile_Empty(t *testing.T) {
	os.Unsetenv(ProfileEnvVar)
	assert.Equal(t, "", GetProfile())
}

func TestLoadFileWithProfile(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(basePath, []byte("APP_NAME: base_app\nPORT: \"8080\"\nDEBUG: false"), 0644))

	profilePath := filepath.Join(dir, "config.dev.yaml")
	require.NoError(t, os.WriteFile(profilePath, []byte("DEBUG: true\nPORT: \"9090\""), 0644))

	os.Unsetenv("APP_NAME")
	os.Unsetenv("PORT")
	os.Unsetenv("DEBUG")

	err := LoadFileWithProfile(basePath, "dev")
	require.NoError(t, err)

	assert.Equal(t, "base_app", os.Getenv("APP_NAME"))
	assert.Equal(t, "9090", os.Getenv("PORT"))
	assert.Equal(t, "true", os.Getenv("DEBUG"))
}

func TestLoadFileWithProfile_NoProfileFile(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(basePath, []byte("APP_NAME: base_app\nPORT: \"8080\""), 0644))

	os.Unsetenv("APP_NAME")
	os.Unsetenv("PORT")

	err := LoadFileWithProfile(basePath, "dev")
	require.NoError(t, err)

	assert.Equal(t, "base_app", os.Getenv("APP_NAME"))
	assert.Equal(t, "8080", os.Getenv("PORT"))
}

func TestLoadFileWithProfile_NoProfile(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(basePath, []byte("APP_NAME: base_app"), 0644))

	os.Unsetenv("APP_NAME")

	err := LoadFileWithProfile(basePath, "")
	require.NoError(t, err)

	assert.Equal(t, "base_app", os.Getenv("APP_NAME"))
}

func TestLoadFileWithProfile_EnvFile(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(basePath, []byte("APP_NAME=base_app\nDEBUG=false"), 0644))

	profilePath := filepath.Join(dir, ".env.dev")
	require.NoError(t, os.WriteFile(profilePath, []byte("DEBUG=true\nPORT=3000"), 0644))

	os.Unsetenv("APP_NAME")
	os.Unsetenv("DEBUG")
	os.Unsetenv("PORT")

	err := LoadFileWithProfile(basePath, "dev")
	require.NoError(t, err)

	assert.Equal(t, "base_app", os.Getenv("APP_NAME"))
	assert.Equal(t, "true", os.Getenv("DEBUG"))
	assert.Equal(t, "3000", os.Getenv("PORT"))
}

func TestLoadFileWithProfile_OSEnvPrecedence(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(basePath, []byte("PORT: \"8080\""), 0644))

	profilePath := filepath.Join(dir, "config.dev.yaml")
	require.NoError(t, os.WriteFile(profilePath, []byte("PORT: \"9090\""), 0644))

	t.Setenv("PORT", "7000")

	err := LoadFileWithProfile(basePath, "dev")
	require.NoError(t, err)

	assert.Equal(t, "7000", os.Getenv("PORT"))
}

func TestLoadDirWithProfile(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("APP_NAME: base_app\nDEBUG: false"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.dev.yaml"), []byte("DEBUG: true"), 0644))

	os.Unsetenv("APP_NAME")
	os.Unsetenv("DEBUG")

	err := LoadDirWithProfile(dir, "dev")
	require.NoError(t, err)

	assert.Equal(t, "base_app", os.Getenv("APP_NAME"))
	assert.Equal(t, "true", os.Getenv("DEBUG"))
}

func TestLoadDirWithProfile_Subdirectory(t *testing.T) {
	dir := t.TempDir()
	profileDir := filepath.Join(dir, "dev")
	require.NoError(t, os.Mkdir(profileDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("APP_NAME: base_app\nDEBUG: false\nPORT: \"8080\""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "config.yaml"), []byte("DEBUG: true\nPORT: \"3000\""), 0644))

	os.Unsetenv("APP_NAME")
	os.Unsetenv("DEBUG")
	os.Unsetenv("PORT")

	err := LoadDirWithProfile(dir, "dev")
	require.NoError(t, err)

	assert.Equal(t, "base_app", os.Getenv("APP_NAME"))
	assert.Equal(t, "true", os.Getenv("DEBUG"))
	assert.Equal(t, "3000", os.Getenv("PORT"))
}

func TestLoadDirWithProfile_NoProfile(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("APP_NAME: base_app"), 0644))

	os.Unsetenv("APP_NAME")

	err := LoadDirWithProfile(dir, "")
	require.NoError(t, err)

	assert.Equal(t, "base_app", os.Getenv("APP_NAME"))
}

func TestLoadDirWithProfile_NoProfileFilesExist(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("APP_NAME: base_app\nDEBUG: false"), 0644))

	os.Unsetenv("APP_NAME")
	os.Unsetenv("DEBUG")

	err := LoadDirWithProfile(dir, "prod")
	require.NoError(t, err)

	assert.Equal(t, "base_app", os.Getenv("APP_NAME"))
	assert.Equal(t, "false", os.Getenv("DEBUG"))
}

func TestHasProfileSegment(t *testing.T) {
	tests := []struct {
		filename string
		profile  string
		expected bool
	}{
		{"config.dev.yaml", "dev", true},
		{"config.prod.json", "prod", true},
		{".env.dev", "dev", true},
		{"config.yaml", "dev", false},
		{"config.dev.yaml", "prod", false},
		{"config.yaml", "", false},
		{"config.staging.toml", "staging", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename+"+"+tt.profile, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasProfileSegment(tt.filename, tt.profile))
		})
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("test"), 0644))

	assert.True(t, fileExists(path))
	assert.False(t, fileExists(filepath.Join(dir, "nonexistent.txt")))
	assert.False(t, fileExists(dir))
}

func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0755))

	assert.True(t, dirExists(subdir))
	assert.False(t, dirExists(filepath.Join(dir, "nonexistent")))
	assert.False(t, dirExists(filepath.Join(dir, "test.txt")))
}
