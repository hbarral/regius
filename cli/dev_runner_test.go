package cli

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writeGoApp creates a minimal long-running Go app in dir (a module with a
// main that prints a marker and sleeps) and returns it.
func writeGoApp(t *testing.T, dir, marker string) {
	t.Helper()
	files := map[string]string{
		"go.mod": "module devtest\n\ngo 1.25\n",
		"main.go": "package main\n" +
			"import (\n\t\"fmt\"\n\t\"time\"\n)\n" +
			"func main() {\n\tfmt.Println(\"" + marker + "\")\n\tfor { time.Sleep(time.Hour) }\n}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newTestRunner(t *testing.T, dir string) *devRunner {
	t.Helper()
	binary := filepath.Join(dir, "tmp", "devtest-app")
	return newDevRunner(devConfig{
		binaryPath: binary,
		rootPath:   dir,
		renderer:   "go",
		buildDelay: 50 * time.Millisecond,
	})
}

func TestRunner_Build(t *testing.T) {
	dir := t.TempDir()
	writeGoApp(t, dir, "hello-dev")
	r := newTestRunner(t, dir)

	if err := r.build(); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(r.cfg.binaryPath); err != nil {
		t.Fatalf("binary not produced: %v", err)
	}
}

func TestRunner_BuildFailure(t *testing.T) {
	dir := t.TempDir()
	writeGoApp(t, dir, "hello-dev")
	// introduce a syntax error
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package main\nfunc broken({\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := newTestRunner(t, dir)

	if err := r.build(); err == nil {
		t.Fatal("expected build error for broken source")
	}
}

func TestRunner_StopStart(t *testing.T) {
	dir := t.TempDir()
	writeGoApp(t, dir, "hello-dev")
	r := newTestRunner(t, dir)

	if err := r.build(); err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	r.mu.Lock()
	pid1 := r.cmd.Process.Pid
	r.mu.Unlock()

	if r.childUptime() <= 0 {
		t.Fatal("childUptime should be positive while running")
	}

	if err := r.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// the old process should no longer be alive
	if processAlive(t, pid1) {
		t.Fatal("child still running after stop")
	}

	// start again: a fresh process
	if err := r.start(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer r.stop()

	r.mu.Lock()
	pid2 := r.cmd.Process.Pid
	r.mu.Unlock()
	if pid1 == pid2 {
		t.Fatalf("expected a new pid after restart (%d)", pid1)
	}
}

func processAlive(t *testing.T, pid int) bool {
	t.Helper()
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func TestRunner_PortOverride(t *testing.T) {
	r := newDevRunner(devConfig{port: 3999})
	env := r.childEnv()

	found := false
	for _, e := range env {
		if e == "PORT=3999" {
			found = true
		}
		if strings.HasPrefix(e, "PORT=") && e != "PORT=3999" {
			t.Fatalf("duplicate PORT entry: %q", e)
		}
	}
	if !found {
		t.Fatal("childEnv missing PORT=3999 override")
	}
}

func TestRunner_PortZeroKeepsEnv(t *testing.T) {
	r := newDevRunner(devConfig{port: 0})
	if len(r.childEnv()) != len(os.Environ()) {
		t.Fatal("childEnv should be a pass-through when no port override is set")
	}
}

func TestPrefixWriter(t *testing.T) {
	var sb strings.Builder
	w := newPrefixWriter("[tw] ", &sb)
	if _, err := w.Write([]byte("line1\nline2\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("line3")); err != nil {
		t.Fatal(err)
	}
	want := "[tw] line1\n[tw] line2\n[tw] line3"
	if sb.String() != want {
		t.Fatalf("got %q, want %q", sb.String(), want)
	}
}

func TestWriteTailwindSources(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir("/") })

	for _, renderer := range []string{"go", "jet"} {
		if err := writeTailwindSources(renderer); err != nil {
			t.Fatalf("writeTailwindSources(%s): %v", renderer, err)
		}
		data, err := os.ReadFile(filepath.Join("assets", "css", "sources.generated.css"))
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		switch renderer {
		case "go":
			if !strings.Contains(content, "*.page.template") {
				t.Error("go sources missing *.page.template")
			}
		case "jet":
			if !strings.Contains(content, "*.jet") {
				t.Error("jet sources missing *.jet")
			}
		}
	}
}
