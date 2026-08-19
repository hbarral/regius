package cli

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

const defaultRenderer = "templ"

var validRenderers = map[string]bool{
	"templ": true,
	"jet":   true,
	"go":    true,
}

// resolveRenderer returns the renderer to use for a `make` subcommand:
// the --renderer flag wins, then the RENDERER env var, then the default.
func resolveRenderer() string {
	if makeRenderer != "" {
		return makeRenderer
	}
	if v := os.Getenv("RENDERER"); v != "" {
		return v
	}
	return defaultRenderer
}

// handlerTemplateSuffix returns the embedded-template filename suffix for the
// handler/auth-handlers stub of the given renderer. The "go" engine uses
// "gotpl" (not "go") so the stub is not compiled as real Go by the CLI's own
// build (only files ending in .go are compiled).
func handlerTemplateSuffix(renderer string) string {
	if strings.ToLower(renderer) == "go" {
		return "gotpl"
	}
	return strings.ToLower(renderer)
}

// handlerViewStub returns the embedded view-stub template path and the file
// extension to use for the written view file, for the given renderer.
func handlerViewStub(renderer string) (templatePath, fileExt string) {
	switch strings.ToLower(renderer) {
	case "jet":
		return "templates/views/handler.jet", ".jet"
	case "go":
		return "templates/views/handler.page.template", ".page.template"
	default:
		return "templates/views/handler.templ", ".templ"
	}
}

// templVersion pins the templ CLI version used by `go run` fallback so it
// matches the version pinned in templates/go_mod.
const templVersion = "v0.3.1020"

// runTemplGenerate runs `templ generate` (the installed CLI when available,
// otherwise `go run github.com/a-h/templ/cmd/templ@<version> generate`) in the
// current working directory. It is a no-op (returns nil) when there are no
// .templ files.
func runTemplGenerate() error {
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
	return runGoCmd(cmd, "templ generate")
}

// patchHomeHandler rewrites the scaffolded Home handler so it uses the given
// renderer. templ keeps the skeleton default; jet/go swap the render call and
// drop the now-unused `views` package import.
func patchHomeHandler(renderer string) error {
	handlersFile := "handlers/handlers.go"
	data, err := os.ReadFile(handlersFile)
	if err != nil {
		return err
	}
	src := string(data)

	viewsImport := fmt.Sprintf("\t\"%s/views\"\n", appURL)
	// match both the lean (nil) and auth-aware (TemplateData) forms
	templRenderRe := `err := h.App.Render.Page(w, r, views.Home(), nil)`

	switch strings.ToLower(renderer) {
	case "jet":
		src = strings.Replace(src, templRenderRe,
			"err := h.App.Render.Page(w, r, h.App.Render.Jet(\"home\", nil), nil)", 1)
		src = strings.Replace(src, viewsImport, "", 1)
	case "go":
		src = strings.Replace(src, templRenderRe,
			"err := h.App.Render.Page(w, r, h.App.Render.GoLayout(\"home\", \"base\"), nil)", 1)
		src = strings.Replace(src, viewsImport, "", 1)
	}

	return os.WriteFile(handlersFile, []byte(src), 0o644)
}

// patchRoutesNoTemplUI strips the templui script route and its import from
// routes.go. Used for non-templ renderers where the templui components (and
// their JS bundles) are not shipped.
func patchRoutesNoTemplUI() error {
	routesFile := "routes.go"
	data, err := os.ReadFile(routesFile)
	if err != nil {
		return err
	}
	src := string(data)

	src = strings.Replace(src, "\t\"github.com/templui/templui/utils\"\n", "", 1)
	src = strings.Replace(src, `
	// templui component scripts
	templuiMux := http.NewServeMux()
	utils.SetupScriptRoutes(templuiMux, a.App.Debug)
	a.App.Routes.Handle("/templui/js/*", templuiMux)
`, "", 1)

	return os.WriteFile(routesFile, []byte(src), 0o644)
}

// goTailwindSources returns the @source directives used by the go renderer's
// Tailwind workflow. It scans Go template files and JS instead of templ sources.
func goTailwindSources() string {
	return `@source "./**/*.page.template";
@source "./**/*.layout.template";
@source "./**/*.js";
`
}

// jetTailwindSources returns the @source directives used by the jet renderer's
// Tailwind workflow. It scans Jet template files and JS instead of templ sources.
func jetTailwindSources() string {
	return `@source "./**/*.jet";
@source "./**/*.js";
`
}

// patchTaskfileForJet rewrites the scaffolded Taskfile for the jet renderer so
// the Tailwind watcher scans Jet templates instead of templ/templui sources.
func patchTaskfileForJet() error {
	// Write the source file up-front so the dev workflow is ready immediately.
	if err := copyDataToFile([]byte(jetTailwindSources()), "assets/css/sources.generated.css"); err != nil {
		return err
	}

	taskfile := `version: "3"

tasks:
  tailwind:
    desc: Watch and rebuild Tailwind CSS stylesheet automatically
    cmds:
      - |
        printf '%s\n' \
          '@source "./**/*.jet";' \
          '@source "./**/*.js";' \
          > ./assets/css/sources.generated.css && \
        NODE_OPTIONS="" tailwindcss -i ./assets/css/input.css -o ./public/css/output.css --watch

  build-css:
    desc: Build the Tailwind CSS stylesheet once
    cmds:
      - |
        printf '%s\n' \
          '@source "./**/*.jet";' \
          '@source "./**/*.js";' \
          > ./assets/css/sources.generated.css && \
        NODE_OPTIONS="" tailwindcss -i ./assets/css/input.css -o ./public/css/output.css

  dev:
    desc: Run the Tailwind CSS watcher for development
    cmds:
      - go-task tailwind
`
	return os.WriteFile("Taskfile.yml", []byte(taskfile), 0o644)
}

// patchTaskfileForGo rewrites the scaffolded Taskfile for the go renderer so
// the Tailwind watcher scans Go templates instead of templ/templui sources.
func patchTaskfileForGo() error {
	// Write the source file up-front so the dev workflow is ready immediately.
	if err := copyDataToFile([]byte(goTailwindSources()), "assets/css/sources.generated.css"); err != nil {
		return err
	}

	taskfile := `version: "3"

tasks:
  tailwind:
    desc: Watch and rebuild Tailwind CSS stylesheet automatically
    cmds:
      - |
        printf '%s\n' \
          '@source "./**/*.page.template";' \
          '@source "./**/*.layout.template";' \
          '@source "./**/*.js";' \
          > ./assets/css/sources.generated.css && \
        NODE_OPTIONS="" tailwindcss -i ./assets/css/input.css -o ./public/css/output.css --watch

  build-css:
    desc: Build the Tailwind CSS stylesheet once
    cmds:
      - |
        printf '%s\n' \
          '@source "./**/*.page.template";' \
          '@source "./**/*.layout.template";' \
          '@source "./**/*.js";' \
          > ./assets/css/sources.generated.css && \
        NODE_OPTIONS="" tailwindcss -i ./assets/css/input.css -o ./public/css/output.css

  dev:
    desc: Run the Tailwind CSS watcher for development
    cmds:
      - go-task tailwind
`
	return os.WriteFile("Taskfile.yml", []byte(taskfile), 0o644)
}

// pruneForRenderer removes templui/Tailwind-only files from the scaffold for
// non-templ renderers and swaps the Home handler to the chosen engine. It does
// not install auth (that is opt-in via `regius make auth`).
func pruneForRenderer(renderer string) error {
	renderer = strings.ToLower(renderer)
	if renderer == "templ" {
		return nil
	}

	// strip the templui script route now that the components are gone.
	if err := patchRoutesNoTemplUI(); err != nil {
		return err
	}

	// swap the Home handler to the chosen renderer.
	if err := patchHomeHandler(renderer); err != nil {
		return err
	}

	// Drop all templ sources + generated code.
	_ = filepath.Walk("views", func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".templ") || strings.HasSuffix(path, "_templ.go") {
			_ = os.Remove(path)
		}
		return nil
	})

	switch renderer {
	case "go":
		// The go renderer keeps the Tailwind CSS workflow and ships a
		// pre-built public/css/output.css, so assets/ and the Taskfile stay.
		// Remove the jet views that are shipped for the jet renderer.
		_ = os.Remove("views/home.jet")
		_ = os.Remove("views/layouts/base.jet")
		_ = os.RemoveAll("views/components")

		// Install the Go-template layout system and styled pages.
		if err := copyFileFromTemplate("templates/views/home.page.template", "views/home.page.template"); err != nil {
			return err
		}
		if err := copyFileFromTemplate("templates/views/layouts/base.layout.template", "views/layouts/base.layout.template"); err != nil {
			return err
		}
		if err := copyFileFromTemplate("templates/views/layouts/main.layout.template", "views/layouts/main.layout.template"); err != nil {
			return err
		}
		components, err := fs.Glob(templateFS, "templates/views/components/*.page.template")
		if err != nil {
			return err
		}
		for _, src := range components {
			dst := strings.TrimPrefix(src, "templates/")
			if err := copyFileFromTemplate(src, dst); err != nil {
				return err
			}
		}

		// Rewrite the Tailwind watcher to scan Go templates instead of templ.
		if err := patchTaskfileForGo(); err != nil {
			return err
		}

	case "jet":
		// The jet renderer now also ships the modern Tailwind CSS workflow.
		// Remove the lean jet views shipped from the skeleton and install the
		// styled layouts, components, and home page.
		_ = os.RemoveAll("views/components")
		_ = os.Remove("views/home.jet")
		_ = os.Remove("views/layouts/base.jet")

		if err := copyFileFromTemplate("templates/views/home.jet", "views/home.jet"); err != nil {
			return err
		}
		if err := copyFileFromTemplate("templates/views/layouts/base.jet", "views/layouts/base.jet"); err != nil {
			return err
		}
		if err := copyFileFromTemplate("templates/views/layouts/main.jet", "views/layouts/main.jet"); err != nil {
			return err
		}
		components, err := fs.Glob(templateFS, "templates/views/components/*.jet")
		if err != nil {
			return err
		}
		for _, src := range components {
			dst := strings.TrimPrefix(src, "templates/")
			if err := copyFileFromTemplate(src, dst); err != nil {
				return err
			}
		}

		// Rewrite the Tailwind watcher to scan Jet templates instead of templ.
		if err := patchTaskfileForJet(); err != nil {
			return err
		}
	}

	color.Green("  ✓ Configured for %s renderer", renderer)
	return nil
}

// installAuthViews copies the four auth views for the given renderer from the
// embedded templates into the app's views/ directory. templ views contain
// ${APP_NAME} import placeholders that are substituted to the app module name.
// For templ it also swaps the lean navbar + mobile-menu components for the
// auth-aware ones (Sign In / Sign Out / Welcome).
func installAuthViews(renderer string) error {
	names := []string{"signin", "signup", "forgot", "reset-password"}
	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = appURL
	}
	switch strings.ToLower(renderer) {
	case "templ":
		for _, n := range names {
			src := fmt.Sprintf("templates/views/%s.templ", n)
			dst := fmt.Sprintf("views/%s.templ", n)
			if fileExists(dst) {
				continue
			}
			data, err := templateFS.ReadFile(src)
			if err != nil {
				return err
			}
			content := strings.ReplaceAll(string(data), "${APP_NAME}", appName)
			if err := copyDataToFile([]byte(content), dst); err != nil {
				return err
			}
		}
		// overwrite the lean navbar + menu with the auth-aware variants
		for _, c := range []string{"navbar", "menu_mobile"} {
			src := fmt.Sprintf("templates/views/components/%s.templ", c)
			dst := fmt.Sprintf("views/components/%s.templ", c)
			data, err := templateFS.ReadFile(src)
			if err != nil {
				return err
			}
			if err := copyDataToFile([]byte(data), dst); err != nil {
				return err
			}
		}
	case "jet":
		for _, n := range names {
			src := fmt.Sprintf("templates/views/%s.jet", n)
			dst := fmt.Sprintf("views/%s.jet", n)
			if err := copyFileFromTemplate(src, dst); err != nil {
				return err
			}
		}
		// Swap the lean navbar + mobile menu for auth-aware variants.
		for _, c := range []string{"navbar", "menu_mobile"} {
			src := fmt.Sprintf("templates/views/components/auth/%s.jet", c)
			dst := fmt.Sprintf("views/components/%s.jet", c)
			data, err := templateFS.ReadFile(src)
			if err != nil {
				return err
			}
			if err := copyDataToFile([]byte(data), dst); err != nil {
				return err
			}
		}
	case "go":
		for _, n := range names {
			src := fmt.Sprintf("templates/views/%s.page.template", n)
			dst := fmt.Sprintf("views/%s.page.template", n)
			if err := copyFileFromTemplate(src, dst); err != nil {
				return err
			}
		}
		// Swap the lean navbar + mobile menu for auth-aware variants.
		for _, c := range []string{"navbar", "menu_mobile"} {
			src := fmt.Sprintf("templates/views/components/auth/%s.page.template", c)
			dst := fmt.Sprintf("views/components/%s.page.template", c)
			data, err := templateFS.ReadFile(src)
			if err != nil {
				return err
			}
			if err := copyDataToFile([]byte(data), dst); err != nil {
				return err
			}
		}
	}
	return nil
}
