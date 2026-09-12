package regius

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/hbarral/regius/api"
)

func newScalarTestApp(t *testing.T, cfg ScalarConfig) *Regius {
	t.Helper()
	r := &Regius{}
	r.Scalar = cfg
	return r
}

func TestRegisterScalarRoutes_Disabled(t *testing.T) {
	r := newScalarTestApp(t, ScalarConfig{Enabled: false})

	mux := chi.NewRouter()
	r.registerScalarRoutes(mux)

	req := httptest.NewRequest("GET", "/docs", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 when disabled, got %d", rr.Code)
	}
}

func TestServeScalarPage(t *testing.T) {
	r := newScalarTestApp(t, ScalarConfig{
		Enabled:  true,
		DocsPath: "/docs",
		SpecPath: "/openapi.json",
		Title:    "My API Docs",
		CDNURL:   "https://cdn.jsdelivr.net/npm/@scalar/api-reference",
		Theme:    "default",
	})

	req := httptest.NewRequest("GET", "/docs", nil)
	rr := httptest.NewRecorder()
	r.serveScalarPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "My API Docs") {
		t.Error("expected title in HTML output")
	}
	if !strings.Contains(body, "https://cdn.jsdelivr.net/npm/@scalar/api-reference") {
		t.Error("expected CDN URL in HTML output")
	}
	if !strings.Contains(body, "/openapi.json") {
		t.Error("expected spec URL in HTML output")
	}
	if !strings.Contains(body, "Scalar.createApiReference") {
		t.Error("expected Scalar.createApiReference call in HTML output")
	}
}

func TestServeOpenAPISpec_Programmatic(t *testing.T) {
	doc := api.NewDocument("Test API", "1.0.0")
	doc.Path("/users", api.NewPathItem().WithGet(
		api.NewOperation("Users", "List users").
			JSONResponse(200, "Success", api.ArraySchema(api.StringSchema())),
	))

	r := newScalarTestApp(t, ScalarConfig{
		Enabled: true,
		Spec:    doc,
	})

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rr := httptest.NewRecorder()
	r.serveOpenAPISpec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json content type, got %s", ct)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["openapi"] != "3.1.0" {
		t.Errorf("expected openapi 3.1.0, got %v", parsed["openapi"])
	}

	info := parsed["info"].(map[string]interface{})
	if info["title"] != "Test API" {
		t.Errorf("expected title 'Test API', got %v", info["title"])
	}
}

func TestServeOpenAPISpec_StaticFile(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.json")
	specContent := `{"openapi":"3.1.0","info":{"title":"Static","version":"2.0.0"}}`
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write spec file: %v", err)
	}

	r := newScalarTestApp(t, ScalarConfig{
		Enabled:  true,
		SpecFile: specPath,
	})

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rr := httptest.NewRecorder()
	r.serveOpenAPISpec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json content type, got %s", ct)
	}

	if rr.Body.String() != specContent {
		t.Error("expected static file content to be served as-is")
	}
}

func TestServeOpenAPISpec_StaticYAMLFile(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.yaml")
	specContent := "openapi: 3.1.0\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write spec file: %v", err)
	}

	r := newScalarTestApp(t, ScalarConfig{
		Enabled:  true,
		SpecFile: specPath,
	})

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rr := httptest.NewRecorder()
	r.serveOpenAPISpec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("expected application/yaml content type, got %s", ct)
	}
}

func TestServeOpenAPISpec_NoSpecConfigured(t *testing.T) {
	r := newScalarTestApp(t, ScalarConfig{
		Enabled: true,
	})

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rr := httptest.NewRecorder()
	r.serveOpenAPISpec(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 when no spec configured, got %d", rr.Code)
	}
}

func TestServeOpenAPISpec_StaticFileNotFound(t *testing.T) {
	r := newScalarTestApp(t, ScalarConfig{
		Enabled:  true,
		SpecFile: "/nonexistent/openapi.json",
	})

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rr := httptest.NewRecorder()
	r.serveOpenAPISpec(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for missing spec file, got %d", rr.Code)
	}
}

func TestSetAPIDocument(t *testing.T) {
	r := &Regius{}
	r.Scalar = ScalarConfig{Enabled: true}

	doc := api.NewDocument("Test", "1.0.0")
	r.SetAPIDocument(doc)

	if r.Scalar.Spec != doc {
		t.Error("expected Spec to be set after SetAPIDocument")
	}
}

func TestScalarRoutesRegistered(t *testing.T) {
	r := newScalarTestApp(t, ScalarConfig{
		Enabled:  true,
		DocsPath: "/docs",
		SpecPath: "/openapi.json",
		Title:    "Test",
		CDNURL:   "https://cdn.jsdelivr.net/npm/@scalar/api-reference",
		Theme:    "default",
		Spec:     api.NewDocument("Test API", "1.0.0"),
	})

	mux := chi.NewRouter()
	r.registerScalarRoutes(mux)

	req := httptest.NewRequest("GET", "/docs", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected /docs to return 200, got %d", rr.Code)
	}

	req = httptest.NewRequest("GET", "/openapi.json", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected /openapi.json to return 200, got %d", rr.Code)
	}
}
