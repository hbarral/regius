package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProfileEnvVar is the environment variable used to select the active
// configuration profile (e.g., "dev", "staging", "prod"). When set, the
// loader looks for profile-specific files alongside base config files.
const ProfileEnvVar = "APP_PROFILE"

// ProfileFilename derives the profile-specific filename from a base config
// file path. The profile segment is inserted before the file extension.
//
// Examples:
//
//	ProfileFilename(".env", "dev")          → ".env.dev"
//	ProfileFilename("config.yaml", "dev")   → "config.dev.yaml"
//	ProfileFilename("config.json", "prod")  → "config.prod.json"
func ProfileFilename(path, profile string) string {
	if profile == "" {
		return path
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	if base == ".env" {
		return filepath.Join(dir, ".env."+profile)
	}

	ext := filepath.Ext(base)
	if ext == "" {
		return filepath.Join(dir, base+"."+profile)
	}
	name := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, name+"."+profile+ext)
}

// LoadFileWithProfile loads a base config file and, if profile is non-empty,
// a profile-specific variant. Profile values override base values, but
// neither overrides existing OS environment variables.
//
// If the profile-specific file does not exist, only the base file is loaded.
func LoadFileWithProfile(path, profile string) error {
	values := make(map[string]string)
	if err := loadIntoMap(path, values); err != nil {
		return err
	}

	if profile != "" {
		profilePath := ProfileFilename(path, profile)
		if fileExists(profilePath) {
			if err := loadIntoMap(profilePath, values); err != nil {
				return err
			}
		}
	}

	return setEnvIfNotExists(values)
}

// LoadDirWithProfile loads all supported config files from a directory and,
// if profile is non-empty, from a profile-specific subdirectory. Profile
// values override base values, but neither overrides existing OS env vars.
//
// The profile subdirectory is expected at `{dir}/{profile}/`. If it does
// not exist, only the base directory files are loaded.
//
// Additionally, profile-specific files in the base directory following the
// `{name}.{profile}.{ext}` naming convention are loaded (e.g.,
// `config.dev.yaml`).
func LoadDirWithProfile(dir, profile string) error {
	values := make(map[string]string)

	// Load base directory files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("config: failed to read directory %s: %w", dir, err)
	}

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
			// Skip profile-specific files here; they are loaded later
			// so they override base values correctly.
			if !hasProfileSegment(name, profile) {
				if err := loadIntoMap(filepath.Join(dir, name), values); err != nil {
					return err
				}
			}
		}
	}

	if profile != "" {
		// Load profile-specific files from the base directory.
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if hasProfileSegment(name, profile) {
				if err := loadIntoMap(filepath.Join(dir, name), values); err != nil {
					return err
				}
			}
		}

		// Load files from the profile subdirectory if it exists.
		profileDir := filepath.Join(dir, profile)
		if dirExists(profileDir) {
			profEntries, err := os.ReadDir(profileDir)
			if err != nil {
				return fmt.Errorf("config: failed to read profile directory %s: %w", profileDir, err)
			}
			for _, entry := range profEntries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if name == ".env" || strings.HasSuffix(name, ".env") {
					if err := loadIntoMap(filepath.Join(profileDir, name), values); err != nil {
						return err
					}
					continue
				}
				ext := filepath.Ext(name)
				if _, ok := supportedExtensions[ext]; ok {
					if err := loadIntoMap(filepath.Join(profileDir, name), values); err != nil {
						return err
					}
				}
			}
		}
	}

	return setEnvIfNotExists(values)
}

// GetProfile reads the active profile from the APP_PROFILE environment
// variable. Returns an empty string if no profile is set.
func GetProfile() string {
	return os.Getenv(ProfileEnvVar)
}

// hasProfileSegment checks if a filename contains the profile segment,
// e.g., "config.dev.yaml" has the "dev" profile segment.
func hasProfileSegment(filename, profile string) bool {
	if profile == "" {
		return false
	}
	base := filepath.Base(filename)

	// Handle .env.{profile} files (e.g., ".env.dev").
	if strings.HasPrefix(base, ".env.") {
		parts := strings.Split(base, ".")
		for _, p := range parts[2:] {
			if p == profile {
				return true
			}
		}
		return false
	}

	// Handle {name}.{profile}.{ext} files (e.g., "config.dev.yaml").
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)
	parts := strings.Split(nameWithoutExt, ".")
	for _, p := range parts {
		if p == profile {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
