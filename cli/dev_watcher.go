package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// devWatcher watches the project tree for changes to restart-worthy files
// (Go sources, templates, config) and emits debounced rebuild signals on
// the restarts channel. The compiled stylesheet (public/css/output.css) is
// a carve-out from the otherwise-ignored public/ tree: changes there emit
// a cssReloads signal instead (a tailwind rebuild means "tell the browser
// to reload", not "restart the app"). Other CSS/JS changes are left to
// the tailwindcss --watch subprocess.
type devWatcher struct {
	fsw          *fsnotify.Watcher
	root         string
	debounce     time.Duration
	extraIgnores []string

	restarts   chan struct{}
	cssReloads chan struct{}
	stopCh     chan struct{}
	doneCh     chan struct{}

	mu           sync.Mutex
	stopped      bool
	templChanged bool
}

// ignoredDirNames are directory names whose whole subtree is skipped.
var ignoredDirNames = map[string]bool{
	"tmp":          true,
	"vendor":       true,
	".git":         true,
	"node_modules": true,
	"public":       true,
	"dist":         true,
	"bin":          true,
	".idea":        true,
	".vscode":      true,
	"coverage":     true,
}

// watchExtensions are the file extensions that trigger a rebuild. CSS and
// JS are deliberately absent: stylesheet rebuilds are handled by the
// tailwind subprocess and static assets are served from disk.
var watchExtensions = map[string]bool{
	".go":       true,
	".templ":    true,
	".jet":      true,
	".template": true,
	".yaml":     true,
	".yml":      true,
	".json":     true,
	".toml":     true,
}

func newDevWatcher(root string, debounce time.Duration, extraIgnores []string) (*devWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &devWatcher{
		fsw:          fsw,
		root:         root,
		debounce:     debounce,
		extraIgnores: extraIgnores,
		restarts:     make(chan struct{}, 1),
		cssReloads:   make(chan struct{}, 1),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}, nil
}

// start walks the project root (plus any extra watch paths) adding a watch
// on every non-ignored directory, then serves events in the background.
func (w *devWatcher) start(extraPaths []string) error {
	if err := w.addTree(w.root); err != nil {
		w.fsw.Close()
		return err
	}
	for _, p := range extraPaths {
		if fi, err := os.Stat(p); err == nil {
			if fi.IsDir() {
				if err := w.addTree(p); err != nil {
					continue
				}
			} else {
				_ = w.fsw.Add(p)
			}
		}
	}
	// public/ is ignored wholesale, but the compiled stylesheet lives at
	// public/css/output.css and its (re)creation must be observed so the
	// browser can be told to reload after a tailwind rebuild.
	_ = w.fsw.Add(filepath.Join(w.root, "public", "css"))
	go w.loop()
	return nil
}

func (w *devWatcher) stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	close(w.stopCh)
	w.mu.Unlock()
	_ = w.fsw.Close()
	<-w.doneCh
}

// templFilesChanged reports (and resets) whether any .templ file changed
// since the last call, so the runner only runs `templ generate` when needed.
func (w *devWatcher) templFilesChanged() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	changed := w.templChanged
	w.templChanged = false
	return changed
}

func (w *devWatcher) setTemplChanged() {
	w.mu.Lock()
	w.templChanged = true
	w.mu.Unlock()
}

// addTree adds a watch on every directory under path, skipping ignored
// subtrees. Errors on individual directories are tolerated: watching is
// best-effort.
func (w *devWatcher) addTree(path string) error {
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != path && w.ignoredPath(p) {
			return filepath.SkipDir
		}
		return w.fsw.Add(p)
	})
}

// ignoredPath reports whether a directory should not be watched. It matches
// both well-known directory names and user-supplied entries (compared as a
// path component or a relative-path prefix).
func (w *devWatcher) ignoredPath(dir string) bool {
	rel := w.relFromRoot(dir)
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}
		if ignoredDirNames[part] {
			return true
		}
	}
	for _, ig := range w.extraIgnores {
		ig = strings.TrimSuffix(filepath.ToSlash(ig), "/")
		if ig == "" {
			continue
		}
		if strings.EqualFold(filepath.ToSlash(rel), ig) {
			return true
		}
		for _, part := range parts {
			if strings.EqualFold(part, ig) {
				return true
			}
		}
	}
	return false
}

func (w *devWatcher) relFromRoot(p string) string {
	rel, err := filepath.Rel(w.root, p)
	if err != nil {
		return p
	}
	return rel
}

// qualifies reports whether a change to path should trigger a rebuild.
func (w *devWatcher) qualifies(path string) bool {
	name := filepath.Base(path)

	// generated templ code: regenerating it would trigger a second rebuild
	if strings.HasSuffix(name, "_templ.go") {
		return false
	}

	// .env and profile variants (.env.dev, .env.master, ...)
	if strings.HasPrefix(name, ".env") {
		return true
	}

	if !watchExtensions[filepath.Ext(name)] {
		return false
	}

	// events for files under ignored directories are dropped
	rel := w.relFromRoot(filepath.Dir(path))
	if !strings.HasPrefix(rel, "..") {
		parts := strings.Split(rel, string(filepath.Separator))
		for _, part := range parts {
			if part == "." || part == "" {
				continue
			}
			if ignoredDirNames[part] {
				return false
			}
			for _, ig := range w.extraIgnores {
				if strings.EqualFold(part, strings.TrimSuffix(filepath.ToSlash(ig), "/")) {
					return false
				}
			}
		}
	}
	return true
}

// devEventKind classifies what a file change means for the dev loop.
type devEventKind int

const (
	devEventNone devEventKind = iota
	devEventRestart
	devEventCSS
)

// classify returns what a change to path means: a restart-worthy change,
// a compiled-stylesheet change (browser reload only), or nothing.
func (w *devWatcher) classify(path string) devEventKind {
	if w.isCSSOutput(path) {
		return devEventCSS
	}
	if w.qualifies(path) {
		return devEventRestart
	}
	return devEventNone
}

// isCSSOutput reports whether path is the compiled stylesheet carved out
// of the otherwise-ignored public/ tree.
func (w *devWatcher) isCSSOutput(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	return filepath.ToSlash(rel) == "public/css/output.css"
}

func (w *devWatcher) loop() {
	defer close(w.doneCh)
	var timer *time.Timer
	var timerC <-chan time.Time
	var pendingRestart, pendingCSS bool

	for {
		select {
		case <-w.stopCh:
			if timer != nil {
				timer.Stop()
			}
			return

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			_ = err // watcher errors are non-fatal; keep serving

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}

			// New directories (e.g. from `regius make handler`) are added
			// to the watch set so scaffolding during dev is picked up.
			if event.Has(fsnotify.Create) {
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					_ = w.addTree(event.Name)
					continue
				}
			}

			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) &&
				!event.Has(fsnotify.Remove) && !event.Has(fsnotify.Rename) {
				continue
			}
			switch w.classify(event.Name) {
			case devEventRestart:
				pendingRestart = true
				if strings.HasSuffix(event.Name, ".templ") {
					w.setTemplChanged()
				}
			case devEventCSS:
				pendingCSS = true
			case devEventNone:
				continue
			}

			// Debounce: reset the window on every qualifying event. The
			// pendings are only read when the timer channel fires below,
			// on this same goroutine — no cross-goroutine state (an
			// AfterFunc callback could race a concurrently-arriving event
			// and wipe its pending flag).
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(w.debounce)
			timerC = timer.C

		case <-timerC:
			timerC = nil
			// Each channel is cap-1 and dropping is correct: a pending
			// signal of the same kind makes the dropped one redundant.
			if pendingRestart {
				select {
				case w.restarts <- struct{}{}:
				default:
				}
			}
			if pendingCSS {
				select {
				case w.cssReloads <- struct{}{}:
				default:
				}
			}
			pendingRestart, pendingCSS = false, false
		}
	}
}
