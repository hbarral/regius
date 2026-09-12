package config

import (
	"os"
	"strings"
	"sync"
)

// ChangeType describes what happened to a config value during a reload.
type ChangeType int

const (
	ChangeAdded ChangeType = iota
	ChangeModified
	ChangeRemoved
)

func (c ChangeType) String() string {
	switch c {
	case ChangeAdded:
		return "added"
	case ChangeModified:
		return "modified"
	case ChangeRemoved:
		return "removed"
	default:
		return "unknown"
	}
}

// ValueChange represents a single configuration value change detected
// during a hot-reload.
type ValueChange struct {
	Key      string
	OldValue string
	NewValue string
	Type     ChangeType
}

// tracker records which environment variables were set by config files
// versus those that existed in the OS environment before any config
// loading occurred. This allows the hot-reload watcher to safely update
// only config-sourced values without touching OS env vars.
type tracker struct {
	mu         sync.Mutex
	osKeys     map[string]bool
	configKeys map[string]string
	initOnce   sync.Once
}

var globalTracker = &tracker{
	osKeys:     make(map[string]bool),
	configKeys: make(map[string]string),
}

// ensureInit snapshots the OS environment once, before the first config
// load, so that subsequently-set config vars can be distinguished from
// pre-existing OS env vars.
func (t *tracker) ensureInit() {
	t.initOnce.Do(func() {
		for _, kv := range os.Environ() {
			idx := strings.IndexByte(kv, '=')
			if idx > 0 {
				t.osKeys[kv[:idx]] = true
			}
		}
	})
}

// setIfNotOS sets an env var only if it is not already defined in the OS
// environment or by a prior config load. Returns true if the value was set.
func (t *tracker) setIfNotOS(key, value string) bool {
	t.ensureInit()
	if t.osKeys[key] {
		return false
	}
	if _, exists := os.LookupEnv(key); exists {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.configKeys[key] = value
	_ = os.Setenv(key, value)
	return true
}

// reload re-applies config values from the given map, overriding only
// values that were previously set by config files (not OS env vars).
// It returns the list of changes detected.
func (t *tracker) reload(values map[string]string) []ValueChange {
	t.ensureInit()
	t.mu.Lock()
	defer t.mu.Unlock()

	var changes []ValueChange

	for key, newVal := range values {
		if t.osKeys[key] {
			continue
		}
		oldVal, wasSet := t.configKeys[key]
		if !wasSet {
			if _, envExists := os.LookupEnv(key); envExists {
				continue
			}
			t.configKeys[key] = newVal
			_ = os.Setenv(key, newVal)
			changes = append(changes, ValueChange{
				Key: key, OldValue: "", NewValue: newVal, Type: ChangeAdded,
			})
		} else if oldVal != newVal {
			t.configKeys[key] = newVal
			_ = os.Setenv(key, newVal)
			changes = append(changes, ValueChange{
				Key: key, OldValue: oldVal, NewValue: newVal, Type: ChangeModified,
			})
		}
	}

	for key, oldVal := range t.configKeys {
		if _, exists := values[key]; !exists {
			if !t.osKeys[key] {
				_ = os.Unsetenv(key)
				delete(t.configKeys, key)
				changes = append(changes, ValueChange{
					Key: key, OldValue: oldVal, NewValue: "", Type: ChangeRemoved,
				})
			}
		}
	}

	return changes
}

// reset clears the tracker state. Intended for testing.
func (t *tracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.osKeys = make(map[string]bool)
	t.configKeys = make(map[string]string)
	t.initOnce = sync.Once{}
}
