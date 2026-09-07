package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// devRunner owns the application's build/restart lifecycle: it builds the
// dev binary (running `templ generate` first when templ sources changed),
// stops the previous child process gracefully, and starts the new one with
// its output streamed to the terminal.
type devRunner struct {
	cfg devConfig

	// templDirty is wired to the watcher's templFilesChanged by runDev so
	// `templ generate` only runs when .templ sources actually changed.
	templDirty func() bool

	mu        sync.Mutex
	cmd       *exec.Cmd
	done      chan struct{}
	stopping  bool
	startedAt time.Time

	exits chan struct{}
}

func newDevRunner(cfg devConfig) *devRunner {
	return &devRunner{
		cfg:   cfg,
		exits: make(chan struct{}, 1),
	}
}

// build produces the dev binary. templ sources are regenerated first when
// the watcher reported .templ changes (and the renderer is templ).
func (r *devRunner) build() error {
	if r.cfg.renderer == "templ" && !r.cfg.noTempl && r.templDirty != nil && r.templDirty() {
		if err := r.runTemplGenerate(); err != nil {
			return fmt.Errorf("templ generate failed: %w", err)
		}
	}
	return r.runGoBuild()
}

func (r *devRunner) runGoBuild() error {
	cmd := exec.Command("go", "build", "-o", r.cfg.binaryPath, ".")
	cmd.Dir = r.cfg.rootPath

	if r.cfg.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go build failed: %w", err)
		}
		return nil
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		if buf.Len() > 0 {
			fmt.Fprintln(os.Stderr, buf.String())
		}
		return fmt.Errorf("go build failed: %w", err)
	}
	return nil
}

// runTemplGenerate regenerates templ sources using the installed templ CLI
// (or the pinned `go run` fallback), with output captured and shown on
// failure.
func (r *devRunner) runTemplGenerate() error {
	matches, _ := filepath.Glob("views/**/*.templ")
	top, _ := filepath.Glob("views/*.templ")
	if len(matches) == 0 && len(top) == 0 {
		return nil
	}

	var cmd *exec.Cmd
	if _, err := exec.LookPath("templ"); err == nil {
		cmd = exec.Command("templ", "generate")
	} else {
		cmd = exec.Command("go", "run", fmt.Sprintf("github.com/a-h/templ/cmd/templ@%s", templVersion), "generate")
	}
	cmd.Dir = r.cfg.rootPath

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		if buf.Len() > 0 {
			fmt.Fprintln(os.Stderr, buf.String())
		}
		return err
	}
	return nil
}

// start launches the dev binary with the app's environment (plus the
// --port override) and streams its output. A monitor goroutine reaps the
// process and reports unexpected exits on the exits channel.
func (r *devRunner) start() error {
	absBinary, err := filepath.Abs(r.cfg.binaryPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(absBinary)
	cmd.Dir = r.cfg.rootPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = r.childEnv()
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan struct{})
	r.mu.Lock()
	r.cmd = cmd
	r.done = done
	r.stopping = false
	r.startedAt = time.Now()
	r.mu.Unlock()

	go func() {
		err := cmd.Wait()
		close(done)

		r.mu.Lock()
		wasCurrent := r.cmd == cmd
		stopping := r.stopping
		r.mu.Unlock()
		if wasCurrent && !stopping && err != nil {
			select {
			case r.exits <- struct{}{}:
			default:
			}
		}
	}()
	return nil
}

// childEnv returns the app environment: the current environment (already
// loaded from .env by the root command) with PORT overridden when set and
// DEV_RELOAD_ENABLED set explicitly so the DevReload middleware state
// matches the resolved config (flag > env > default on).
func (r *devRunner) childEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+2)
	for _, e := range env {
		if strings.HasPrefix(e, "PORT=") || strings.HasPrefix(e, "DEV_RELOAD_ENABLED=") {
			continue
		}
		out = append(out, e)
	}
	if r.cfg.port > 0 {
		out = append(out, fmt.Sprintf("PORT=%d", r.cfg.port))
	}
	out = append(out, fmt.Sprintf("DEV_RELOAD_ENABLED=%t", r.cfg.browserReload))
	return out
}

// stop gracefully terminates the running child (SIGTERM to the process
// group on Unix, taskkill on Windows), escalating to a hard kill after the
// grace period.
func (r *devRunner) stop() error {
	r.mu.Lock()
	cmd := r.cmd
	done := r.done
	if cmd == nil {
		r.mu.Unlock()
		return nil
	}
	r.stopping = true
	r.mu.Unlock()

	_ = terminateProcess(cmd)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		color.Red("[dev] app did not exit within 10s — killing")
		_ = killProcess(cmd)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}

	r.mu.Lock()
	if r.cmd == cmd {
		r.cmd = nil
	}
	r.mu.Unlock()
	return nil
}

// childUptime reports how long the current child has been running (zero
// when none is running). Used to guard against crash/restart loops.
func (r *devRunner) childUptime() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil {
		return 0
	}
	return time.Since(r.startedAt)
}

// tailwindProc manages the `tailwindcss --watch` subprocess.
type tailwindProc struct {
	cmd     *exec.Cmd
	done    chan struct{}
	stopped bool
	mu      sync.Mutex
}

// startTailwindWatcher starts the CSS watcher when the tailwindcss CLI is
// available and the project has a Tailwind input stylesheet. Returns nil
// (with a warning printed) otherwise.
func startTailwindWatcher(cfg devConfig) *tailwindProc {
	if _, err := exec.LookPath("tailwindcss"); err != nil {
		color.Yellow("[dev] tailwindcss CLI not found — CSS changes will not rebuild automatically")
		return nil
	}
	if !fileExists(filepath.Join("assets", "css", "input.css")) {
		return nil
	}

	if err := writeTailwindSources(cfg.renderer); err != nil {
		color.Yellow("[dev] could not write tailwind sources: %v", err)
	}

	// --watch=always: plain --watch stops when stdin closes, and this
	// subprocess has no terminal attached.
	cmd := exec.Command("tailwindcss",
		"-i", filepath.Join("assets", "css", "input.css"),
		"-o", filepath.Join("public", "css", "output.css"),
		"--watch=always")
	cmd.Dir = cfg.rootPath
	cmd.Stdout = newPrefixWriter("[tailwind] ", os.Stdout)
	cmd.Stderr = newPrefixWriter("[tailwind] ", os.Stderr)
	// Clear NODE_OPTIONS: it can carry flags that break the tailwind CLI
	// (e.g. --disallow-code-generation-from-strings).
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "NODE_OPTIONS=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, "NODE_OPTIONS=")
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		color.Yellow("[dev] could not start tailwind watcher: %v", err)
		return nil
	}

	t := &tailwindProc{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(t.done)
	}()
	return t
}

func (t *tailwindProc) stop() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	t.mu.Unlock()

	_ = terminateProcess(t.cmd)
	select {
	case <-t.done:
	case <-time.After(5 * time.Second):
		_ = killProcess(t.cmd)
		select {
		case <-t.done:
		case <-time.After(3 * time.Second):
		}
	}
}

// writeTailwindSources regenerates assets/css/sources.generated.css for the
// given renderer. The templ variant resolves the templui module path with
// `go list` (like the scaffolded Makefile target); jet/go use static globs.
func writeTailwindSources(renderer string) error {
	var lines []string
	switch strings.ToLower(renderer) {
	case "templ":
		templuiPath := ""
		out, err := exec.Command("go", "list", "-mod=mod", "-m", "-f", "{{.Dir}}", "github.com/templui/templui").Output()
		if err == nil {
			templuiPath = strings.TrimSpace(string(out))
		}
		lines = []string{
			`@source "./**/*.templ";`,
			`@source "./**/*.js";`,
		}
		if templuiPath != "" {
			lines = append(lines,
				fmt.Sprintf("@source %q;", filepath.ToSlash(filepath.Join(templuiPath, "components/**/*.templ"))),
				fmt.Sprintf("@source %q;", filepath.ToSlash(filepath.Join(templuiPath, "components/**/*.js"))),
			)
		}
	case "jet":
		lines = []string{
			`@source "./**/*.jet";`,
			`@source "./**/*.js";`,
		}
	default:
		lines = []string{
			`@source "./**/*.page.template";`,
			`@source "./**/*.layout.template";`,
			`@source "./**/*.js";`,
		}
	}
	target := filepath.Join("assets", "css", "sources.generated.css")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// prefixWriter prefixes every written line with a label so subprocess
// output (e.g. tailwind) is distinguishable from app output.
type prefixWriter struct {
	prefix      string
	w           io.Writer
	atLineStart bool
}

func newPrefixWriter(prefix string, w io.Writer) *prefixWriter {
	return &prefixWriter{prefix: prefix, w: w, atLineStart: true}
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	var out bytes.Buffer
	for _, c := range b {
		if p.atLineStart {
			out.WriteString(p.prefix)
			p.atLineStart = false
		}
		out.WriteByte(c)
		if c == '\n' {
			p.atLineStart = true
		}
	}
	_, err := p.w.Write(out.Bytes())
	return len(b), err
}
