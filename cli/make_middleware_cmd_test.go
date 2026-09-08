package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const routesFixture = `package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) routes() *chi.Mux {
	// middlewares
	// add any global middleware here
	// a.use(a.Middleware.CheckRemember)

	// routes
	a.get("/", a.Handlers.Home)

	// add any route here

	return a.App.Routes
}
`

const routesNoMarkerFixture = `package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) routes() *chi.Mux {
	// middlewares
	// a.use(a.Middleware.CheckRemember)

	a.get("/", a.Handlers.Home)

	return a.App.Routes
}
`

// withMiddlewareRoot points the CLI's global backend at a fresh temp dir
// seeded with a middleware struct file and a routes.go fixture.
func withMiddlewareRoot(t *testing.T, routesFixture string) string {
	t.Helper()

	old := b.RootPath
	dir := t.TempDir()
	b.RootPath = dir
	t.Cleanup(func() { b.RootPath = old })

	if err := os.MkdirAll(filepath.Join(dir, "middleware"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "middleware", "middleware.go"), []byte("package middleware\n\ntype Middleware struct{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "routes.go"), []byte(routesFixture), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestDoMakeMiddleware_CreatesStub(t *testing.T) {
	dir := withMiddlewareRoot(t, routesFixture)

	if err := doMakeMiddleware("request-logging", false); err != nil {
		t.Fatalf("doMakeMiddleware: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "middleware", "request_logging.go"))
	if err != nil {
		t.Fatalf("middleware file not created: %v", err)
	}
	mw := string(data)
	for _, want := range []string{
		"package middleware",
		"// RequestLogging TODO: describe what this middleware does.",
		"func (m *Middleware) RequestLogging(next http.Handler) http.Handler",
	} {
		if !strings.Contains(mw, want) {
			t.Errorf("generated middleware missing %q:\n%s", want, mw)
		}
	}
	if strings.Contains(mw, "$") {
		t.Errorf("generated middleware still contains unreplaced placeholder:\n%s", mw)
	}

	// without --global, routes.go is untouched
	routes, err := os.ReadFile(filepath.Join(dir, "routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(routes), "a.use(a.Middleware.RequestLogging)") {
		t.Error("routes.go was wired without --global")
	}
}

func TestDoMakeMiddleware_GlobalWiresRoutes(t *testing.T) {
	dir := withMiddlewareRoot(t, routesFixture)

	if err := doMakeMiddleware("request-logging", true); err != nil {
		t.Fatalf("doMakeMiddleware: %v", err)
	}

	routes, err := os.ReadFile(filepath.Join(dir, "routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(routes)
	if !strings.Contains(got, "\ta.use(a.Middleware.RequestLogging)\n") {
		t.Errorf("routes.go missing the a.use line:\n%s", got)
	}
	// must be wired before any route registration
	if strings.Index(got, "a.use(a.Middleware.RequestLogging)") > strings.Index(got, "a.get(\"/\"") {
		t.Errorf("a.use line inserted after route registrations:\n%s", got)
	}
}

func TestDoMakeMiddleware_GlobalFallbackWithoutMarker(t *testing.T) {
	dir := withMiddlewareRoot(t, routesNoMarkerFixture)

	if err := doMakeMiddleware("request-logging", true); err != nil {
		t.Fatalf("doMakeMiddleware: %v", err)
	}

	routes, err := os.ReadFile(filepath.Join(dir, "routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(routes)
	if !strings.Contains(got, "\ta.use(a.Middleware.RequestLogging)\n") {
		t.Errorf("routes.go missing the a.use line (fallback):\n%s", got)
	}
}

func TestDoMakeMiddleware_DuplicateName(t *testing.T) {
	withMiddlewareRoot(t, routesFixture)

	if err := doMakeMiddleware("logging", false); err != nil {
		t.Fatalf("first doMakeMiddleware: %v", err)
	}
	err := doMakeMiddleware("logging", false)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}

func TestWireGlobalMiddleware_Idempotent(t *testing.T) {
	dir := withMiddlewareRoot(t, routesFixture)
	routesPath := filepath.Join(dir, "routes.go")

	if err := wireGlobalMiddleware(routesPath, "RequestLogging"); err != nil {
		t.Fatalf("wireGlobalMiddleware: %v", err)
	}
	if err := wireGlobalMiddleware(routesPath, "RequestLogging"); err != nil {
		t.Fatalf("wireGlobalMiddleware (2nd): %v", err)
	}

	routes, err := os.ReadFile(routesPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(routes), "a.use(a.Middleware.RequestLogging)") != 1 {
		t.Errorf("expected exactly one a.use line, got:\n%s", routes)
	}
}

func TestWireGlobalMiddleware_NoAnchor(t *testing.T) {
	dir := t.TempDir()
	routesPath := filepath.Join(dir, "routes.go")
	if err := os.WriteFile(routesPath, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := wireGlobalMiddleware(routesPath, "RequestLogging")
	if err == nil || !strings.Contains(err.Error(), "no insertion point") {
		t.Fatalf("expected no-insertion-point error, got %v", err)
	}
}
