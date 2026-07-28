package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	GOOSLinux   = "linux"
	GOOSDarwin  = "darwin"
	GOOSWindows = "windows"
)

var (
	newAppDB       string
	newAppRenderer string
	newAppVerbose  bool
)

var validDBTypes = map[string]bool{
	"postgres":   true,
	"postgresql": true,
	"mysql":      true,
	"mariadb":    true,
	"sqlite":     true,
	"sqlite3":    true,
}

func init() {
	rootCmd.AddCommand(newCmd)
	newCmd.Flags().StringVar(&newAppDB, "db", "sqlite", "pre-fill DATABASE_TYPE in .env (postgres|postgresql|mysql|mariadb|sqlite|sqlite3)")
	newCmd.Flags().StringVar(&newAppRenderer, "renderer", defaultRenderer, "template engine to scaffold (templ|jet|go)")
	newCmd.Flags().BoolVarP(&newAppVerbose, "verbose", "v", false, "stream go command output live instead of capturing it")
}

var newCmd = &cobra.Command{
	Use:   "new [application-name]",
	Short: "Create a new Regius application",
	Long: `Create a new Regius application from the embedded starter skeleton
and set up the initial configuration.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		doNew(args[0])
	},
}

var appURL string

func doNew(appName string) {
	appName = strings.ToLower(appName)
	appURL = appName

	if strings.Contains(appName, "/") {
		exploded := strings.SplitAfter(appName, "/")
		appName = exploded[(len(exploded) - 1)]
	}

	db := strings.ToLower(newAppDB)
	renderer := strings.ToLower(newAppRenderer)

	if !validDBTypes[db] {
		exitWithError(fmt.Errorf("unsupported --db type %q (use postgres|postgresql|mysql|mariadb|sqlite|sqlite3)", newAppDB))
	}
	if !validRenderers[renderer] {
		exitWithError(fmt.Errorf("unsupported --renderer %q (use templ|jet|go)", newAppRenderer))
	}

	color.Green("Creating application '%s' (db=%s, renderer=%s)...", appName, db, renderer)

	if err := writeSkeleton("./" + appName); err != nil {
		exitGracefully(err)
	}
	color.Green("  ✓ Skeleton copied")

	// .env: substitute DATABASE_TYPE and RENDERER, plus app name + key.
	data, err := templateFS.ReadFile("templates/env")
	if err != nil {
		exitGracefully(err)
	}
	env := string(data)
	env = strings.Replace(env, "DATABASE_TYPE=sqlite", "DATABASE_TYPE="+db, 1)
	env = strings.Replace(env, "RENDERER=templ", "RENDERER="+renderer, 1)
	env = strings.ReplaceAll(env, "${APP_NAME}", appName)
	env = strings.ReplaceAll(env, "${KEY}", reg.RandomString(32))
	if err := copyDataToFile([]byte(env), fmt.Sprintf("./%s/.env", appName)); err != nil {
		exitGracefully(err)
	}
	color.Green("  ✓ .env written")

	// Makefile: the per-OS template, with templ targets + a templ generate step
	// prepended to `build` when the renderer is templ.
	mkOS := runtime.GOOS
	switch mkOS {
	case GOOSDarwin:
		mkOS = "mac"
	case GOOSWindows:
		mkOS = "windows"
	default:
		mkOS = "linux"
	}
	mkData, err := templateFS.ReadFile(fmt.Sprintf("templates/Makefile.%s", mkOS))
	if err != nil {
		exitGracefully(err)
	}
	mk := string(mkData)
	mk = strings.ReplaceAll(mk, "${NAME}", appName)
	binaryName := appName
	if mkOS == "windows" {
		binaryName = appName + ".exe"
	}
	mk = strings.ReplaceAll(mk, "${BINARY_APP_NAME}", binaryName)
	if renderer == "templ" {
		mk = strings.Replace(mk, "@go mod vendor\n", "@go mod vendor\n\t@templ generate\n", 1)
		mk += "\n" + templMakefileTargets()
	}
	if err := copyDataToFile([]byte(mk), fmt.Sprintf("./%s/Makefile", appName)); err != nil {
		exitGracefully(err)
	}
	color.Green("  ✓ Makefile written (%s)", mkOS)

	// go.mod: from the template, with the app module name substituted.
	_ = os.Remove(fmt.Sprintf("./%s/go.mod", appName))
	data, err = templateFS.ReadFile("templates/go_mod")
	if err != nil {
		exitGracefully(err)
	}
	mod := strings.ReplaceAll(string(data), "${APP_NAME}", appName)
	mod = strings.ReplaceAll(mod, "${REGIUS_VERSION}", regiusGoModVersion())
	if err := copyDataToFile([]byte(mod), fmt.Sprintf("./%s/go.mod", appName)); err != nil {
		exitGracefully(err)
	}
	color.Green("  ✓ go.mod written")

	if err := os.Chdir("./" + appName); err != nil {
		exitGracefully(err)
	}
	updateSource()
	color.Green("  ✓ Source files updated")

	// Non-templ renderers drop the templui/Tailwind stack and swap Home + auth.
	if err := pruneForRenderer(renderer); err != nil {
		exitWithError(fmt.Errorf("renderer setup failed: %w", err))
	}

	if renderer == "templ" {
		if err := runTemplGenerate(); err != nil {
			color.Yellow("  ! templ generate failed (build may need `templ generate`): %v", err)
		}
	}

	if err := runGoCmd(exec.Command("go", "get", "github.com/hbarral/regius"), "go get github.com/hbarral/regius"); err != nil {
		exitWithError(fmt.Errorf("go get failed: %w", err))
	}
	if err := runGoCmd(exec.Command("go", "mod", "tidy"), "go mod tidy"); err != nil {
		exitWithError(fmt.Errorf("go mod tidy failed: %w", err))
	}

	color.Green("  ✓ Done — %s is ready", appURL)
	color.Green("  Go build something real!")
}

// templMakefileTargets returns the templ/tailwind recipes appended to the
// scaffolded Makefile for templ-based apps.
func templMakefileTargets() string {
	return `templ:
	@templ generate

tailwind:
	@TEMPLUI_PATH="$$(go list -mod=mod -m -f '{{.Dir}}' github.com/templui/templui)" && \
	 printf '%s\n' \
	   '@source "./**/*.templ";' \
	   '@source "./**/*.js";' \
	   "@source \"$$TEMPLUI_PATH/components/**/*.templ\";" \
	   "@source \"$$TEMPLUI_PATH/components/**/*.js\";" \
	   > ./assets/css/sources.generated.css && \
	tailwindcss -i ./assets/css/input.css -o ./public/css/output.css
`
}

// runGoCmd executes a go subcommand. By default the command's output is
// captured and a spinner animates while it runs; the captured output is only
// shown on failure. With --verbose, the output streams live and no spinner is
// drawn.
func runGoCmd(cmd *exec.Cmd, label string) error {
	if newAppVerbose {
		color.Cyan("  • %s", label)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			color.Red("  ✗ %s", label)
			return err
		}
		color.Green("  ✓ %s", label)
		return nil
	}

	sp := newSpinner(label)
	sp.start()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	sp.stop()
	if err != nil {
		if buf.Len() > 0 {
			fmt.Fprint(os.Stderr, buf.String())
		}
		color.Red("  ✗ %s", label)
		return err
	}
	color.Green("  ✓ %s", label)
	return nil
}
