package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors config files for changes and reloads them automatically,
// updating environment variables and notifying registered callbacks.
type Watcher struct {
	paths    []string
	profile  string
	callback func([]ValueChange)
	stop     chan struct{}
	fsw      *fsnotify.Watcher
	mu       sync.Mutex
	stopped  bool
	debounce time.Duration
}

// NewWatcher creates a Watcher that monitors the given config file paths.
// The files must exist when NewWatcher is called.
func NewWatcher(paths ...string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("config: failed to create watcher: %w", err)
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			fsw.Close()
			return nil, fmt.Errorf("config: file not found: %s", p)
		}
		if err := fsw.Add(p); err != nil {
			fsw.Close()
			return nil, fmt.Errorf("config: failed to watch %s: %w", p, err)
		}
	}

	return &Watcher{
		paths:    paths,
		stop:     make(chan struct{}),
		fsw:      fsw,
		debounce: 500 * time.Millisecond,
	}, nil
}

// WithProfile sets the config profile for reload operations. When set,
// profile-specific files are also watched and loaded alongside base files.
func (w *Watcher) WithProfile(profile string) *Watcher {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.profile = profile
	return w
}

// OnChange registers a callback that is called with the list of value
// changes detected during each reload.
func (w *Watcher) OnChange(fn func([]ValueChange)) *Watcher {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callback = fn
	return w
}

// WithDebounce sets the debounce duration for file change events.
// Editors often trigger multiple write events in quick succession; the
// watcher waits for the debounce period to elapse before reloading.
func (w *Watcher) WithDebounce(d time.Duration) *Watcher {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.debounce = d
	return w
}

// Start begins watching config files in a background goroutine.
func (w *Watcher) Start() error {
	go w.loop()
	return nil
}

// Stop stops watching and releases resources. Safe to call multiple times.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return nil
	}
	w.stopped = true
	close(w.stop)
	w.mu.Unlock()
	return w.fsw.Close()
}

// AddPath adds a new file path to the watcher.
func (w *Watcher) AddPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return fmt.Errorf("config: watcher already stopped")
	}
	if err := w.fsw.Add(path); err != nil {
		return fmt.Errorf("config: failed to watch %s: %w", path, err)
	}
	w.paths = append(w.paths, path)
	return nil
}

func (w *Watcher) loop() {
	var timer *time.Timer
	for {
		select {
		case <-w.stop:
			if timer != nil {
				timer.Stop()
			}
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if timer != nil {
					timer.Stop()
				}
				w.mu.Lock()
				debounce := w.debounce
				w.mu.Unlock()
				timer = time.AfterFunc(debounce, w.reload)
			}
		case <-w.fsw.Errors:
			// Ignore watcher errors; keep running.
		}
	}
}

func (w *Watcher) reload() {
	w.mu.Lock()
	paths := make([]string, len(w.paths))
	copy(paths, w.paths)
	profile := w.profile
	callback := w.callback
	w.mu.Unlock()

	values := make(map[string]string)
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := loadIntoMap(path, values); err != nil {
			continue
		}
		if profile != "" {
			profilePath := ProfileFilename(path, profile)
			if fileExists(profilePath) {
				_ = loadIntoMap(profilePath, values)
			}
		}
	}

	changes := globalTracker.reload(values)
	if callback != nil && len(changes) > 0 {
		callback(changes)
	}
}
