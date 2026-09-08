package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const crudRoutesFixture = `package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) routes() *chi.Mux {
	// middlewares
	// add any global middleware here

	// routes
	a.get("/", a.Handlers.Home)

	// add any route here

	return a.App.Routes
}
`

const crudModelsFixture = `package data

import (
	"database/sql"
	"os"
)

type Models struct {
	// any models inserted here (and in the New function)
	// are easily accessible throughout the entire application
}

func New(databasePool *sql.DB) Models {
	_ = os.Getenv("DATABASE_TYPE")

	return Models{}
}
`

// withCrudRoot points the CLI's global backend at a fresh temp dir seeded
// with routes.go and data/models.go fixtures, using the given renderer and
// database type.
func withCrudRoot(t *testing.T, routesFixture, renderer, dbType string) string {
	t.Helper()

	old := b.RootPath
	oldDBType := b.DBType
	oldRenderer := makeRenderer
	dir := t.TempDir()
	b.RootPath = dir
	b.DBType = dbType
	makeRenderer = ""
	t.Setenv("APP_NAME", "testapp")
	t.Setenv("RENDERER", renderer)
	if dbType != "" {
		t.Setenv("DATABASE_TYPE", dbType)
	} else {
		t.Setenv("DATABASE_TYPE", "")
	}
	t.Cleanup(func() {
		b.RootPath = old
		b.DBType = oldDBType
		makeRenderer = oldRenderer
	})

	if err := os.WriteFile(filepath.Join(dir, "routes.go"), []byte(routesFixture), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "models.go"), []byte(crudModelsFixture), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestDoMakeCRUD_TemplRenderer(t *testing.T) {
	dir := withCrudRoot(t, crudRoutesFixture, "templ", "sqlite")

	if err := doMakeCRUD("post"); err != nil {
		t.Fatalf("doMakeCRUD: %v", err)
	}

	// the model file
	model := readJobFile(t, filepath.Join(dir, "data", "post.go"))
	if !strings.Contains(model, "type Post struct {") {
		t.Errorf("model file missing the Post struct:\n%s", model)
	}
	if !strings.Contains(model, `return "posts"`) {
		t.Errorf("model file missing the posts table name:\n%s", model)
	}

	// the model is wired into the Models struct
	models := readJobFile(t, filepath.Join(dir, "data", "models.go"))
	if !strings.Contains(models, "Post Post") {
		t.Errorf("models.go missing the Post field:\n%s", models)
	}
	if !strings.Contains(models, "Post: Post{},") {
		t.Errorf("models.go missing the Post constructor entry:\n%s", models)
	}

	// the migration (sqlite dialect)
	matches, err := filepath.Glob(filepath.Join(dir, "migrations", "*_create_posts_table.up.sql"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one up migration, got %v (%v)", matches, err)
	}
	up := readJobFile(t, matches[0])
	if !strings.Contains(up, "CREATE TABLE IF NOT EXISTS posts") {
		t.Errorf("up migration missing the posts table:\n%s", up)
	}
	if strings.Contains(up, "$") {
		t.Error("up migration still contains an unreplaced placeholder")
	}

	// the handler (templ variant)
	handler := readJobFile(t, filepath.Join(dir, "handlers", "posts_crud.go"))
	for _, want := range []string{
		"\"testapp/views/posts\"",
		"func (h *Handlers) PostList(w http.ResponseWriter, r *http.Request)",
		"func (h *Handlers) PostCreate(w http.ResponseWriter, r *http.Request)",
		"$TABLE.Index()",
		"h.App.Render.Page(w, r, posts.Index(),",
	} {
		if want == "$TABLE.Index()" {
			continue
		}
		if !strings.Contains(handler, want) {
			t.Errorf("generated handler missing %q:\n%s", want, handler)
		}
	}
	if strings.Contains(handler, "$") {
		t.Errorf("generated handler still contains unreplaced placeholder:\n%s", handler)
	}

	// the views
	for _, view := range []string{"index", "show", "form"} {
		p := filepath.Join(dir, "views", "posts", view+".templ")
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("view %s not created: %v", view, err)
		}
		v := readJobFile(t, p)
		if !strings.Contains(v, "package posts") {
			t.Errorf("view %s missing the posts package:\n%s", view, v)
		}
		if strings.Contains(v, "$") {
			t.Errorf("view %s still contains unreplaced placeholder:\n%s", view, v)
		}
	}

	// the routes
	routes := readJobFile(t, filepath.Join(dir, "routes.go"))
	routeBlock := `a.App.Routes.Route("/posts", func(r chi.Router) {`
	if !strings.Contains(routes, routeBlock) {
		t.Errorf("routes.go missing the CRUD route block:\n%s", routes)
	}
	for _, want := range []string{
		"a.Handlers.PostList",
		"a.Handlers.PostShow",
		"a.Handlers.PostNew",
		"a.Handlers.PostCreate",
		"a.Handlers.PostEdit",
		"a.Handlers.PostUpdate",
		"a.Handlers.PostDelete",
	} {
		if !strings.Contains(routes, want) {
			t.Errorf("routes.go missing %q:\n%s", want, routes)
		}
	}
	// the block sits before the marker, which stays last
	if strings.Index(routes, routeBlock) > strings.Index(routes, "// add any route here") {
		t.Errorf("route block should sit above the marker:\n%s", routes)
	}
}

func TestDoMakeCRUD_RendererVariants(t *testing.T) {
	tests := []struct {
		renderer   string
		handlerArg string
		viewExt    string
	}{
		{"jet", `h.App.Render.Jet("posts/index", nil)`, ".jet"},
		{"go", `h.App.Render.GoLayout("posts/index", "main")`, ".page.template"},
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.renderer, func(t *testing.T) {
			dir := withCrudRoot(t, crudRoutesFixture, tt.renderer, "sqlite")

			if err := doMakeCRUD("post"); err != nil {
				t.Fatalf("doMakeCRUD: %v", err)
			}

			handler := readJobFile(t, filepath.Join(dir, "handlers", "posts_crud.go"))
			if !strings.Contains(handler, tt.handlerArg) {
				t.Errorf("generated handler missing %q:\n%s", tt.handlerArg, handler)
			}

			view := filepath.Join(dir, "views", "posts", "index"+tt.viewExt)
			if _, err := os.Stat(view); err != nil {
				t.Fatalf("index view not created with ext %s: %v", tt.viewExt, err)
			}
		})
	}
}

func TestDoMakeCRUD_NameNormalization(t *testing.T) {
	tests := []struct {
		name  string
		model string
		table string
	}{
		{"singular", "post", "posts"},
		{"plural", "category", "categories"},
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.name, func(t *testing.T) {
			dir := withCrudRoot(t, crudRoutesFixture, "jet", "sqlite")

			input := tt.model
			if tt.name == "plural" {
				input = tt.table
			}

			if err := doMakeCRUD(input); err != nil {
				t.Fatalf("doMakeCRUD(%q): %v", input, err)
			}

			if _, err := os.Stat(filepath.Join(dir, "data", tt.model+".go")); err != nil {
				t.Errorf("model file %s.go not created: %v", tt.model, err)
			}
			matches, _ := filepath.Glob(filepath.Join(dir, "migrations", "*_create_"+tt.table+"_table.up.sql"))
			if len(matches) != 1 {
				t.Errorf("expected one %s migration, got %v", tt.table, matches)
			}
			if _, err := os.Stat(filepath.Join(dir, "handlers", tt.table+"_crud.go")); err != nil {
				t.Errorf("handler %s_crud.go not created: %v", tt.table, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "views", tt.table, "index.jet")); err != nil {
				t.Errorf("view views/%s/index.jet not created: %v", tt.table, err)
			}
		})
	}
}

func TestDoMakeCRUD_MigrationDialects(t *testing.T) {
	tests := []struct {
		dbType string
		marker string
	}{
		{"postgres", "SERIAL PRIMARY KEY"},
		{"postgresql", "SERIAL PRIMARY KEY"},
		{"mysql", "AUTO_INCREMENT"},
		{"mariadb", "AUTO_INCREMENT"},
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.dbType, func(t *testing.T) {
			dir := withCrudRoot(t, crudRoutesFixture, "jet", tt.dbType)

			if err := doMakeCRUD("post"); err != nil {
				t.Fatalf("doMakeCRUD: %v", err)
			}

			matches, err := filepath.Glob(filepath.Join(dir, "migrations", "*_create_posts_table.up.sql"))
			if err != nil || len(matches) != 1 {
				t.Fatalf("expected one up migration, got %v (%v)", matches, err)
			}
			up := readJobFile(t, matches[0])
			if !strings.Contains(up, tt.marker) {
				t.Errorf("up migration missing %q:\n%s", tt.marker, up)
			}
		})
	}
}

func TestDoMakeCRUD_DuplicateHandlerRefused(t *testing.T) {
	withCrudRoot(t, crudRoutesFixture, "jet", "sqlite")

	if err := doMakeCRUD("post"); err != nil {
		t.Fatalf("first doMakeCRUD: %v", err)
	}
	err := doMakeCRUD("post")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error for the handler, got %v", err)
	}
}

func TestDoMakeCRUD_ExistingModelSoftSkipped(t *testing.T) {
	dir := withCrudRoot(t, crudRoutesFixture, "jet", "sqlite")

	custom := "package data\n\ntype Post struct{ Custom bool }\n"
	if err := os.WriteFile(filepath.Join(dir, "data", "post.go"), []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}

	if err := doMakeCRUD("post"); err != nil {
		t.Fatalf("doMakeCRUD with an existing model: %v", err)
	}

	model := readJobFile(t, filepath.Join(dir, "data", "post.go"))
	if model != custom {
		t.Errorf("existing model was overwritten:\n%s", model)
	}

	// the custom model is still wired into Models
	models := readJobFile(t, filepath.Join(dir, "data", "models.go"))
	if !strings.Contains(models, "Post Post") {
		t.Errorf("models.go missing the Post field:\n%s", models)
	}
}

func TestDoMakeCRUD_RoutesFallbackWithoutMarker(t *testing.T) {
	noMarker := strings.Replace(crudRoutesFixture, "\t// add any route here\n\n", "", 1)
	dir := withCrudRoot(t, noMarker, "jet", "sqlite")

	if err := doMakeCRUD("post"); err != nil {
		t.Fatalf("doMakeCRUD: %v", err)
	}

	routes := readJobFile(t, filepath.Join(dir, "routes.go"))
	if !strings.Contains(routes, `a.App.Routes.Route("/posts", func(r chi.Router) {`) {
		t.Errorf("routes.go missing the route block (fallback):\n%s", routes)
	}
	// inserted right after the home route
	if strings.Index(routes, `a.App.Routes.Route("/posts"`) < strings.Index(routes, `a.get("/", a.Handlers.Home)`) {
		t.Errorf("route block should sit after the home route:\n%s", routes)
	}
}

func TestDoMakeCRUD_RoutesIdempotent(t *testing.T) {
	dir := withCrudRoot(t, crudRoutesFixture, "jet", "sqlite")
	routesPath := filepath.Join(dir, "routes.go")

	if err := wireCrudRoutes(routesPath, "Post", "posts"); err != nil {
		t.Fatalf("wireCrudRoutes: %v", err)
	}
	if err := wireCrudRoutes(routesPath, "Post", "posts"); err != nil {
		t.Fatalf("wireCrudRoutes (2nd): %v", err)
	}

	routes := readJobFile(t, routesPath)
	if strings.Count(routes, `a.App.Routes.Route("/posts"`) != 1 {
		t.Errorf("route block duplicated:\n%s", routes)
	}
}
