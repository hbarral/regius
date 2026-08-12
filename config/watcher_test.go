package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracker_SetIfNotOS(t *testing.T) {
	globalTracker.reset()

	t.Setenv("PRE_EXISTING_OS_KEY", "os_value")

	set := globalTracker.setIfNotOS("PRE_EXISTING_OS_KEY", "config_value")
	assert.False(t, set)
	assert.Equal(t, "os_value", os.Getenv("PRE_EXISTING_OS_KEY"))

	set = globalTracker.setIfNotOS("NEW_CONFIG_KEY", "config_value")
	assert.True(t, set)
	assert.Equal(t, "config_value", os.Getenv("NEW_CONFIG_KEY"))
}

func TestTracker_Reload_Added(t *testing.T) {
	globalTracker.reset()
	os.Unsetenv("RELOAD_ADDED_KEY")

	changes := globalTracker.reload(map[string]string{
		"RELOAD_ADDED_KEY": "new_value",
	})

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeAdded, changes[0].Type)
	assert.Equal(t, "RELOAD_ADDED_KEY", changes[0].Key)
	assert.Equal(t, "new_value", changes[0].NewValue)
	assert.Equal(t, "new_value", os.Getenv("RELOAD_ADDED_KEY"))
}

func TestTracker_Reload_Modified(t *testing.T) {
	globalTracker.reset()
	os.Unsetenv("RELOAD_MODIFIED_KEY")

	globalTracker.reload(map[string]string{"RELOAD_MODIFIED_KEY": "old_value"})
	assert.Equal(t, "old_value", os.Getenv("RELOAD_MODIFIED_KEY"))

	changes := globalTracker.reload(map[string]string{"RELOAD_MODIFIED_KEY": "new_value"})

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeModified, changes[0].Type)
	assert.Equal(t, "RELOAD_MODIFIED_KEY", changes[0].Key)
	assert.Equal(t, "old_value", changes[0].OldValue)
	assert.Equal(t, "new_value", changes[0].NewValue)
	assert.Equal(t, "new_value", os.Getenv("RELOAD_MODIFIED_KEY"))
}

func TestTracker_Reload_Removed(t *testing.T) {
	globalTracker.reset()
	os.Unsetenv("RELOAD_REMOVED_KEY")

	globalTracker.reload(map[string]string{"RELOAD_REMOVED_KEY": "value"})
	assert.Equal(t, "value", os.Getenv("RELOAD_REMOVED_KEY"))

	changes := globalTracker.reload(map[string]string{})

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeRemoved, changes[0].Type)
	assert.Equal(t, "RELOAD_REMOVED_KEY", changes[0].Key)
	assert.Equal(t, "value", changes[0].OldValue)
	assert.Empty(t, changes[0].NewValue)
	_, exists := os.LookupEnv("RELOAD_REMOVED_KEY")
	assert.False(t, exists)
}

func TestTracker_Reload_NoChanges(t *testing.T) {
	globalTracker.reset()
	os.Unsetenv("RELOAD_NOCHANGE_KEY")

	globalTracker.reload(map[string]string{"RELOAD_NOCHANGE_KEY": "same_value"})
	changes := globalTracker.reload(map[string]string{"RELOAD_NOCHANGE_KEY": "same_value"})

	assert.Empty(t, changes)
}

func TestTracker_Reload_OSEnvPreserved(t *testing.T) {
	globalTracker.reset()
	t.Setenv("OS_PRESERVED_KEY", "os_value")

	changes := globalTracker.reload(map[string]string{"OS_PRESERVED_KEY": "config_value"})

	assert.Empty(t, changes)
	assert.Equal(t, "os_value", os.Getenv("OS_PRESERVED_KEY"))
}

func TestChangeType_String(t *testing.T) {
	assert.Equal(t, "added", ChangeAdded.String())
	assert.Equal(t, "modified", ChangeModified.String())
	assert.Equal(t, "removed", ChangeRemoved.String())
}

func TestNewWatcher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("KEY: value"), 0644))

	w, err := NewWatcher(path)
	require.NoError(t, err)
	defer w.Stop()

	assert.NotNil(t, w)
}

func TestNewWatcher_FileNotFound(t *testing.T) {
	_, err := NewWatcher("/nonexistent/config.yaml")
	assert.Error(t, err)
}

func TestWatcher_StartStop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("KEY: value"), 0644))

	w, err := NewWatcher(path)
	require.NoError(t, err)

	require.NoError(t, w.Start())
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, w.Stop())
	require.NoError(t, w.Stop()) // idempotent
}

func TestWatcher_OnChange(t *testing.T) {
	globalTracker.reset()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("WATCH_TEST_KEY: initial"), 0644))

	os.Unsetenv("WATCH_TEST_KEY")

	// Load initial config
	require.NoError(t, LoadFile(path))
	assert.Equal(t, "initial", os.Getenv("WATCH_TEST_KEY"))

	var mu sync.Mutex
	var receivedChanges []ValueChange

	w, err := NewWatcher(path)
	require.NoError(t, err)
	w.WithDebounce(100 * time.Millisecond)
	w.OnChange(func(changes []ValueChange) {
		mu.Lock()
		defer mu.Unlock()
		receivedChanges = changes
	})
	require.NoError(t, w.Start())
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Modify the config file
	require.NoError(t, os.WriteFile(path, []byte("WATCH_TEST_KEY: updated"), 0644))

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, receivedChanges, 1)
	assert.Equal(t, "WATCH_TEST_KEY", receivedChanges[0].Key)
	assert.Equal(t, "initial", receivedChanges[0].OldValue)
	assert.Equal(t, "updated", receivedChanges[0].NewValue)
	assert.Equal(t, ChangeModified, receivedChanges[0].Type)
}

func TestWatcher_AddPath(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path1, []byte("KEY1: value1"), 0644))

	w, err := NewWatcher(path1)
	require.NoError(t, err)
	defer w.Stop()

	path2 := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path2, []byte(`{"KEY2": "value2"}`), 0644))

	err = w.AddPath(path2)
	assert.NoError(t, err)
}

func TestWatcher_WithProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("KEY: base"), 0644))

	w, err := NewWatcher(path)
	require.NoError(t, err)
	w.WithProfile("dev")
	defer w.Stop()

	assert.Equal(t, "dev", w.profile)
}

func TestWatcher_WithDebounce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("KEY: value"), 0644))

	w, err := NewWatcher(path)
	require.NoError(t, err)
	w.WithDebounce(1 * time.Second)
	defer w.Stop()

	assert.Equal(t, 1*time.Second, w.debounce)
}
