package cli

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	devPort        int
	devBuildDelay  time.Duration
	devWatchPaths  string
	devIgnorePaths string
	devNoTailwind  bool
	devNoTempl     bool
	devExitOnFail  bool
	devBinary      string
	devVerbose     bool
)

func init() {
	rootCmd.AddCommand(devCmd)

	devCmd.Flags().IntVar(&devPort, "port", 0, "override the PORT the dev server listens on")
	devCmd.Flags().DurationVar(&devBuildDelay, "build-delay", 0, "debounce delay after the last file change before rebuilding (default 500ms)")
	devCmd.Flags().StringVar(&devWatchPaths, "watch", "", "comma-separated additional paths to watch")
	devCmd.Flags().StringVar(&devIgnorePaths, "ignore", "", "comma-separated additional paths to ignore (added to the defaults)")
	devCmd.Flags().BoolVar(&devNoTailwind, "no-tailwind", false, "disable the automatic Tailwind CSS watcher")
	devCmd.Flags().BoolVar(&devNoTempl, "no-templ", false, "disable automatic `templ generate` on .templ changes")
	devCmd.Flags().BoolVar(&devExitOnFail, "exit", false, "exit on build failure instead of keeping the old process alive")
	devCmd.Flags().StringVar(&devBinary, "binary", "", "output binary path for dev builds (default tmp/regius-dev)")
	devCmd.Flags().BoolVarP(&devVerbose, "verbose", "v", false, "stream build output live")
}

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the app with hot-reload in development",
	Long: `Start the application and watch the project for file changes.

When a Go source, template, or config file changes, the app is rebuilt and
restarted automatically. .templ changes run "templ generate" first, and a
"tailwindcss --watch" subprocess keeps the stylesheet fresh. If a rebuild
fails (compile error), the previously started process keeps serving until
the next successful build.

Run from the application root (the directory with .env and go.mod).`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDev(); err != nil {
			exitWithError(err)
		}
	},
}

// devConfig resolves the `regius dev` settings from flags, environment
// variables, and sensible defaults (flag > env > default).
type devConfig struct {
	port       int
	buildDelay time.Duration
	binaryPath string
	noTailwind bool
	noTempl    bool
	exitOnFail bool
	verbose    bool
	rootPath   string
	renderer   string
}

func runDev() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}

	if !fileExists("go.mod") {
		return errors.New("no go.mod found — run `regius dev` from the application root")
	}
	if !fileExists("main.go") && !fileExists("init.regius.go") {
		return errors.New("this does not look like a Regius application (no main.go or init.regius.go)")
	}

	cfg := resolveDevConfig(root)

	runner := newDevRunner(cfg)

	if err := os.MkdirAll(filepath.Dir(cfg.binaryPath), 0o755); err != nil {
		return err
	}

	if err := runner.build(); err != nil {
		return err
	}
	if err := runner.start(); err != nil {
		return err
	}

	// Tailwind watcher subprocess (best effort: skipped with a warning when
	// the tailwindcss CLI is not installed).
	var tw *tailwindProc
	if !cfg.noTailwind {
		tw = startTailwindWatcher(cfg)
	}

	extraIgnores := splitCommaList(devIgnorePaths)
	w, err := newDevWatcher(cfg.rootPath, cfg.buildDelay, extraIgnores)
	if err != nil {
		return err
	}
	runner.templDirty = w.templFilesChanged
	if err := w.start(splitCommaList(devWatchPaths)); err != nil {
		return err
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	color.Green("[dev] watching for changes (renderer=%s, delay=%s) — press Ctrl+C to stop", cfg.renderer, cfg.buildDelay)

	for {
		select {
		case <-sigs:
			color.Yellow("\n[dev] shutting down...")
			w.stop()
			if tw != nil {
				tw.stop()
			}
			if err := runner.stop(); err != nil {
				color.Red("[dev] error stopping app: %v", err)
			}
			color.Green("[dev] stopped")
			return nil

		case <-w.events:
			color.Cyan("[dev] change detected — rebuilding")
			if err := rebuildAndRestart(runner, cfg); err != nil {
				return err
			}

		case <-runner.exits:
			// The app process died on its own (crash, bad config, port in
			// use). Its output already streamed to the terminal.
			if runner.childUptime() < 3*time.Second {
				color.Red("[dev] app exited shortly after start; fix the issue and save a file to rebuild")
				continue
			}
			color.Yellow("[dev] app exited unexpectedly — rebuilding")
			if err := rebuildAndRestart(runner, cfg); err != nil {
				return err
			}
		}
	}
}

// rebuildAndRestart runs the build (templ generate when needed + go build)
// and swaps the child process. On build failure the old process keeps
// serving unless --exit was given.
func rebuildAndRestart(runner *devRunner, cfg devConfig) error {
	if err := runner.build(); err != nil {
		color.Red("[dev] build failed (the old process is still serving)")
		if cfg.exitOnFail {
			if serr := runner.stop(); serr != nil {
				color.Red("[dev] error stopping app: %v", serr)
			}
			return err
		}
		return nil
	}
	if err := runner.stop(); err != nil {
		color.Red("[dev] error stopping old process: %v", err)
	}
	if err := runner.start(); err != nil {
		color.Red("[dev] failed to start app: %v", err)
		if cfg.exitOnFail {
			return err
		}
		return nil
	}
	color.Green("[dev] restarted at %s", time.Now().Format("15:04:05"))
	return nil
}

func resolveDevConfig(root string) devConfig {
	cfg := devConfig{
		port:       devPort,
		noTailwind: devNoTailwind,
		noTempl:    devNoTempl,
		exitOnFail: devExitOnFail,
		verbose:    devVerbose,
		rootPath:   root,
		renderer:   detectRenderer(),
	}

	cfg.buildDelay = devBuildDelay
	if cfg.buildDelay == 0 {
		if v := os.Getenv("DEV_BUILD_DELAY"); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				cfg.buildDelay = d
			}
		}
	}
	if cfg.buildDelay == 0 {
		cfg.buildDelay = 500 * time.Millisecond
	}

	cfg.binaryPath = devBinary
	if cfg.binaryPath == "" {
		cfg.binaryPath = os.Getenv("DEV_BINARY")
	}
	if cfg.binaryPath == "" {
		cfg.binaryPath = filepath.Join("tmp", "regius-dev")
	}
	if runtime.GOOS == GOOSWindows && !strings.HasSuffix(cfg.binaryPath, ".exe") {
		cfg.binaryPath += ".exe"
	}

	cfg.exitOnFail = cfg.exitOnFail || isTruthyEnv(os.Getenv("DEV_EXIT_ON_FAILURE"))

	return cfg
}

func isTruthyEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// detectRenderer returns the app's template engine: the RENDERER env var
// (set from .env by the root command's PersistentPreRun) wins, falling back
// to the presence of templ/jet sources.
func detectRenderer() string {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("RENDERER"))); v != "" {
		return v
	}
	if hasFiles(".", ".templ") {
		return "templ"
	}
	if hasFiles("views", ".jet") {
		return "jet"
	}
	return "go"
}

func hasFiles(dir, ext string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ext) {
			found = true
		}
		return nil
	})
	return found
}

func splitCommaList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
