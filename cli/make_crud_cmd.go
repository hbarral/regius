package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/gertd/go-pluralize"
	"github.com/spf13/cobra"
)

func init() {
	makeCrudCmd.Flags().StringVar(&makeRenderer, "renderer", "", "template engine for the handlers + views (templ|jet|go); defaults to RENDERER env or templ")
}

var makeCrudCmd = &cobra.Command{
	Use:   "crud [name]",
	Short: "Create a full-stack CRUD slice",
	Long: `Creates a web CRUD slice for one resource:

  - a data model (data/<name>.go, upper/db accessors)
  - a create-table migration for the app's DATABASE_TYPE dialect
  - the model wired into the Models struct (data/models.go)
  - a resource controller (handlers/<table>_crud.go) with
    list/show/new/create/edit/update/delete
  - renderer-aware views (views/<table>/) for templ, jet or go
  - routes mounted at /<table> in routes.go

The name is singularized for the model and handlers and pluralized for the
table, URL and views: "regius make crud post" yields the Post model, the
posts table and /posts routes.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeCRUD(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("CRUD scaffolding created!")
	},
}

func doMakeCRUD(name string) error {
	if name == "" {
		return errors.New("you must give the CRUD resource a name")
	}

	renderer := strings.ToLower(resolveRenderer())
	if !validRenderers[renderer] {
		return errors.New("invalid renderer " + renderer + " (use templ|jet|go)")
	}

	appName := os.Getenv("APP_NAME")

	plur := pluralize.NewClient()
	modelName := name
	table := strings.ToLower(name)
	if plur.IsPlural(name) {
		modelName = plur.Singular(name)
	} else {
		table = strings.ToLower(plur.Plural(name))
	}
	title := pascalIdent(modelName)

	// 1. the model; soft-skip when it already exists so re-running after
	// customizing the model is not blocked
	modelFile := b.RootPath + "/data/" + strings.ToLower(modelName) + ".go"
	if !fileExists(modelFile) {
		if err := doMakeModel(modelName); err != nil {
			return err
		}
		gofmtFile(modelFile)
		color.Yellow("  - Created data/%s.go (upper/db model for the %s table)", strings.ToLower(modelName), table)
	}

	if err := wireModelInModelsFile(title); err != nil {
		return err
	}
	color.Yellow("  - Wired %s into the Models struct (data/models.go)", title)

	// 2. the create-table migration
	dialect, err := createCrudMigration(table)
	if err != nil {
		return err
	}
	color.Yellow("  - Created migrations/..._create_%s_table.up/down.sql (%s dialect); add columns before migrating", table, dialect)

	// 3. the resource controller; refuse duplicates
	handlerFile := b.RootPath + "/handlers/" + table + "_crud.go"
	if fileExists(handlerFile) {
		return errors.New(handlerFile + " already exists!")
	}

	handlerData, err := templateFS.ReadFile("templates/crud/handler." + handlerTemplateSuffix(renderer))
	if err != nil {
		return err
	}
	handler := string(handlerData)
	handler = strings.ReplaceAll(handler, "$TITLE", title)
	handler = strings.ReplaceAll(handler, "$TABLE", table)
	handler = strings.ReplaceAll(handler, "${APP_NAME}", appName)
	if err := copyDataToFile([]byte(handler), handlerFile); err != nil {
		return err
	}
	color.Yellow("  - Created handlers/%s_crud.go (list/show/new/create/edit/update/delete)", table)

	// 4. the views (index/show/form); skip individual duplicates
	viewExt := crudViewExt(renderer)
	for _, view := range []string{"index", "show", "form"} {
		viewData, err := templateFS.ReadFile(fmt.Sprintf("templates/crud/views/%s.%s", view, viewExt))
		if err != nil {
			return err
		}
		viewSrc := string(viewData)
		viewSrc = strings.ReplaceAll(viewSrc, "$TITLE", title)
		viewSrc = strings.ReplaceAll(viewSrc, "$TABLE", table)
		viewSrc = strings.ReplaceAll(viewSrc, "${APP_NAME}", appName)
		viewFile := fmt.Sprintf("%s/views/%s/%s.%s", b.RootPath, table, view, viewExt)
		if fileExists(viewFile) {
			continue
		}
		if err := copyDataToFile([]byte(viewSrc), viewFile); err != nil {
			return err
		}
	}
	color.Yellow("  - Created views/%s/{index,show,form} for the %s renderer", table, renderer)

	// regen templ sources if needed
	if renderer == "templ" {
		if err := runTemplGenerate(); err != nil {
			color.Yellow("  ! templ generate failed; run `templ generate` manually: %v", err)
		}
	}

	// 5. the routes, mounted at /<table>
	if err := wireCrudRoutes(b.RootPath+"/routes.go", title, table); err != nil {
		return err
	}
	color.Yellow("  - Mounted /%s CRUD routes in routes.go", table)

	color.Yellow("  - Next: add columns to the migration, fields to data/%s.go, and the TODOs in handlers/%s_crud.go", strings.ToLower(modelName), table)

	return nil
}

// crudViewExt maps the renderer to the view template + file extension.
func crudViewExt(renderer string) string {
	switch strings.ToLower(renderer) {
	case "jet":
		return "jet"
	case "go":
		return "page.template"
	default:
		return "templ"
	}
}

// createCrudMigration writes the create-<table>_table migration for the
// app's DATABASE_TYPE dialect (defaulting to sqlite when unset).
func createCrudMigration(table string) (string, error) {
	dialect := normalizeDBType(b.DBType)
	if dialect == "" {
		dialect = "sqlite"
	}
	switch dialect {
	case "postgres", "mysql", "sqlite":
	default:
		return "", fmt.Errorf("unsupported DATABASE_TYPE %q for the CRUD table migration", b.DBType)
	}

	up, err := templateFS.ReadFile("templates/migrations/crud_table." + dialect + ".up.sql")
	if err != nil {
		return "", err
	}
	down, err := templateFS.ReadFile("templates/migrations/crud_table." + dialect + ".down.sql")
	if err != nil {
		return "", err
	}

	up = []byte(strings.ReplaceAll(string(up), "$TABLE_NAME", table))
	down = []byte(strings.ReplaceAll(string(down), "$TABLE_NAME", table))

	return dialect, b.CreateMigration(up, down, "create_"+table+"_table", "sql")
}

// wireModelInModelsFile registers the model in the Models struct and its
// constructor. Idempotent.
func wireModelInModelsFile(title string) error {
	path := b.RootPath + "/data/models.go"
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read data/models.go: %w", err)
	}
	str := string(data)
	if strings.Contains(str, "\t"+title+" "+title) {
		return nil // already wired
	}

	newStr, ok := insertAfterLine(str, "type Models struct {", "\t"+title+" "+title+"\n")
	if !ok {
		return fmt.Errorf("could not add %s to the Models struct in data/models.go; add it manually:\n\n\t%s %s\n", title, title, title)
	}
	str = newStr

	newStr, ok = insertAfterLine(str, "return Models{", "\t\t"+title+": "+title+"{},\n")
	if !ok {
		return fmt.Errorf("could not initialize %s in data/models.go New(); add it manually:\n\n\t%s: %s{},\n", title, title, title)
	}

	return writeFormatted(path, []byte(newStr))
}

// wireCrudRoutes mounts the CRUD route group at /<table> in routes.go, at
// the routes marker (fallback: right after the home route). Idempotent.
func wireCrudRoutes(routesPath, title, table string) error {
	block := fmt.Sprintf(`	// %[2]s CRUD (generated by `+"`regius make crud`"+`)
	a.App.Routes.Route("/%[2]s", func(r chi.Router) {
		r.Get("/", a.Handlers.%[1]sList)
		r.Get("/new", a.Handlers.%[1]sNew)
		r.Post("/", a.Handlers.%[1]sCreate)
		r.Get("/{id}", a.Handlers.%[1]sShow)
		r.Get("/{id}/edit", a.Handlers.%[1]sEdit)
		r.Post("/{id}", a.Handlers.%[1]sUpdate)
		r.Post("/{id}/delete", a.Handlers.%[1]sDelete)
	})
`, title, table)

	return insertAtMarker(routesPath, "// add any route here", block,
		fmt.Sprintf("a.App.Routes.Route(\"/%s\"", table), func(content string) (string, bool) {
			// fallback for apps generated before the routes marker: right
			// after the home route
			return insertAfterLine(content, `a.get("/", a.Handlers.Home)`, block)
		})
}
