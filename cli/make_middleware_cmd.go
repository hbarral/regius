package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	makeMiddlewareCmd.Flags().BoolVar(&makeMiddlewareGlobal, "global", false, "also wire the middleware into the global chain in routes.go")
}

var makeMiddlewareCmd = &cobra.Command{
	Use:   "middleware [name]",
	Short: "Create a custom middleware stub",
	Long: `Creates a custom middleware in the middleware directory, as a method on
the app's Middleware struct (so it can use App and Models).

By default the middleware is only scaffolded: add a.use(a.Middleware.<Name>)
wherever it should apply. Pass --global to also wire it into the global
middleware chain in routes.go (before any route is registered, so chi
applies it to every route).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeMiddleware(args[0], makeMiddlewareGlobal); err != nil {
			exitGracefully(err)
		}
		color.Green("Middleware created!")
	},
}

func doMakeMiddleware(name string, global bool) error {
	if name == "" {
		return errors.New("you must give the middleware a name")
	}

	title := pascalIdent(name)
	fileName := b.RootPath + "/middleware/" + snakeIdent(name) + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	data, err := templateFS.ReadFile("templates/middleware/custom.go.tmpl")
	if err != nil {
		return err
	}

	mw := strings.ReplaceAll(string(data), "$MIDDLEWARE_NAME", title)
	if err := copyDataToFile([]byte(mw), fileName); err != nil {
		return err
	}
	color.Yellow("  - Created middleware/%s.go", snakeIdent(name))

	if global {
		if err := wireGlobalMiddleware(b.RootPath+"/routes.go", title); err != nil {
			return err
		}
		color.Yellow("  - Wired a.use(a.Middleware.%s) into the global chain in routes.go", title)
	} else {
		color.Yellow("  - Next: add a.use(a.Middleware.%s) where it should apply, or re-run with --global", title)
	}

	return nil
}

// wireGlobalMiddleware inserts a.use(a.Middleware.<Name>) into routes.go's
// global middleware section, which must sit before any route is registered
// (chi only applies a mux's middleware to routes registered afterwards).
// Idempotent.
func wireGlobalMiddleware(routesPath, title string) error {
	useLine := fmt.Sprintf("\ta.use(a.Middleware.%s)\n", title)
	return insertAtMarker(routesPath, "// add any global middleware here", useLine,
		fmt.Sprintf("a.use(a.Middleware.%s)", title), func(content string) (string, bool) {
			// fallback for apps generated before the marker: right after the
			// "// middlewares" section comment
			if strings.Contains(content, "\n\t// middlewares\n") {
				return strings.Replace(content, "\n\t// middlewares\n", "\n\t// middlewares\n"+useLine, 1), true
			}
			// older skeletons: before the commented CheckRemember example
			if strings.Contains(content, "\t// a.use(a.Middleware.CheckRemember)") {
				return strings.Replace(content, "\t// a.use(a.Middleware.CheckRemember)", useLine+"\n\t// a.use(a.Middleware.CheckRemember)", 1), true
			}
			return "", false
		})
}
