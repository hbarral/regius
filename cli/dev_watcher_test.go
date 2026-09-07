package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestWatcher(t *testing.T, root string, debounce time.Duration, extraIgnores []string) *devWatcher {
	t.Helper()
	w, err := newDevWatcher(root, debounce, extraIgnores)
	if err != nil {
		t.Fatalf("newDevWatcher: %v", err)
	}
	return w
}

func TestWatcher_Qualifies(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root, 50*time.Millisecond, nil)

	cases := []struct {
		rel  string
		want bool
	}{
		{"main.go", true},
		{"handlers/handlers.go", true},
		{"views/home.templ", true},
		{"views/home.jet", true},
		{"views/home.page.template", true},
		{"views/layouts/base.layout.template", true},
		{"locales/en/en.yaml", true},
		{"config.json", true},
		{"config.toml", true},
		{".env", true},
		{".env.dev", true},
		{"mail/templates/mail.html.template", true},

		{"views/home_templ.go", false},        // generated
		{"views/home._templ.go", false},       // generated (canonical suffix)
		{"tmp/regius-dev", false},             // no watch extension
		{"vendor/foo/bar.go", false},          // ignored dir
		{".git/config", false},                // ignored dir
		{"public/css/output.css", false},      // ignored dir + css
		{"assets/css/input.css", false},       // css is tailwind's job
		{"public/js/app.js", false},           // js is not restart-worthy
		{"migrations/001_init.up.sql", false}, // sql not watched
		{"data/regius.db", false},             // no watch extension
	}

	for _, c := range cases {
		got := w.qualifies(filepath.Join(root, c.rel))
		if got != c.want {
			t.Errorf("qualifies(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

func TestWatcher_QualifiesExtraIgnores(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root, 50*time.Millisecond, []string{"generated", "third_party"})

	for _, rel := range []string{"generated/models.go", "third_party/lib/util.go"} {
		if w.qualifies(filepath.Join(root, rel)) {
			t.Errorf("qualifies(%q) = true, want false", rel)
		}
	}
	if !w.qualifies(filepath.Join(root, "handlers/handlers.go")) {
		t.Error("qualifies(handlers/handlers.go) = false, want true")
	}
}

func TestWatcher_Debounce(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root, 150*time.Millisecond, nil)
	if err := w.start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer w.stop()

	// Burst of writes: expect a single coalesced event.
	for i := 0; i < 5; i++ {
		name := filepath.Join(root, "file.go")
		if err := os.WriteFile(name, []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	select {
	case <-w.restarts:
		// first signal received
	case <-time.After(3 * time.Second):
		t.Fatal("no event received after writes")
	}

	// No additional events should arrive for the same burst.
	select {
	case <-w.restarts:
		t.Fatal("unexpected second event for a single burst")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestWatcher_NewDirectory(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root, 100*time.Millisecond, nil)
	if err := w.start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer w.stop()

	// Directory created after the watcher started: files added inside it
	// must still trigger events (e.g. `regius make handler` during dev).
	newDir := filepath.Join(root, "workers")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(newDir, "send_email.go"), []byte("package workers\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.restarts:
	case <-time.After(3 * time.Second):
		t.Fatal("no event for a file in a newly created directory")
	}
}

func TestWatcher_TemplFilesChanged(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root, 50*time.Millisecond, nil)

	if w.templFilesChanged() {
		t.Fatal("templFilesChanged should start false")
	}

	w.setTemplChanged()
	if !w.templFilesChanged() {
		t.Error("templFilesChanged = false after setTemplChanged")
	}
	if w.templFilesChanged() {
		t.Error("templFilesChanged should reset after being read")
	}
}

func TestWatcher_Stop(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root, 50*time.Millisecond, nil)
	if err := w.start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		w.stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not return")
	}

	// A second stop must be a safe no-op.
	w.stop()
}

func TestWatcher_CSSRebuildSignal(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, root, 100*time.Millisecond, nil)
	if err := os.MkdirAll(filepath.Join(root, "public", "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "public", "js"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer w.stop()

	// The compiled stylesheet is the carve-out from the ignored public/ tree.
	if err := os.WriteFile(filepath.Join(root, "public", "css", "output.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.cssReloads:
	case <-time.After(3 * time.Second):
		t.Fatal("no css reload signal for output.css")
	}
	select {
	case <-w.restarts:
		t.Fatal("stylesheet change must not trigger a restart")
	default:
	}

	// Other files under public/ stay ignored.
	if err := os.WriteFile(filepath.Join(root, "public", "js", "app.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.restarts:
		t.Fatal("unexpected restart signal for public/js/app.js")
	case <-w.cssReloads:
		t.Fatal("unexpected css signal for public/js/app.js")
	case <-time.After(700 * time.Millisecond):
	}

	// A mixed burst fires both channels after the debounce.
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "css", "output.css"), []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}
	gotRestart, gotCSS := false, false
	deadline := time.After(3 * time.Second)
	for !(gotRestart && gotCSS) {
		select {
		case <-w.restarts:
			gotRestart = true
		case <-w.cssReloads:
			gotCSS = true
		case <-deadline:
			t.Fatalf("mixed burst: restart=%v css=%v", gotRestart, gotCSS)
		}
	}
}
