package regius

import (
	_ "embed"
	"fmt"
	"net/http"
	"text/template"

	"github.com/go-chi/chi/v5"

	"github.com/hbarral/regius/api"
)

//go:embed scalar.html.tmpl
var scalarHTMLTemplate string

// ScalarConfig configures the Scalar API reference middleware.
type ScalarConfig struct {
	// Enabled controls whether the Scalar routes are registered. When
	// false, no routes are added. Defaults to false (opt-in via
	// SCALAR_ENABLED env var).
	Enabled bool

	// DocsPath is the URL path where the Scalar API reference UI is
	// served. Defaults to "/docs".
	DocsPath string

	// SpecPath is the URL path where the OpenAPI spec is served.
	// Defaults to "/openapi.json".
	SpecPath string

	// Title is the page title shown in the browser. Defaults to
	// "API Reference".
	Title string

	// CDNURL is the URL to the Scalar JavaScript bundle. Defaults to
	// the jsDelivr CDN. Set to a local URL for air-gapped/offline use.
	CDNURL string

	// SpecFile is an optional path to a static openapi.yaml or
	// openapi.json file on disk. When set, the file is served as-is
	// and takes precedence over the programmatic Spec.
	SpecFile string

	// Theme is the Scalar UI theme. Defaults to "default". See
	// https://github.com/scalar/scalar for available themes.
	Theme string

	// ShowClients controls which client library code examples are
	// shown in the Scalar UI. It is injected as a raw JS expression
	// and converted to Scalar's hiddenClients option at runtime.
	// Examples: `["fetch","curl"]` (show only those), `{"js":true,"shell":["curl"]}`
	// (show all JS + only curl from shell). When empty, all clients are shown.
	ShowClients string

	// Spec is the programmatic OpenAPI document. When SpecFile is
	// empty, this document is marshaled to JSON and served at
	// SpecPath.
	Spec *api.Document
}

// scalarPageData holds the template data for the Scalar HTML page.
type scalarPageData struct {
	Title       string
	CDNURL      string
	SpecURL     string
	Theme       string
	ShowClients string
}

// SetAPIDocument sets the programmatic OpenAPI document for the Scalar
// middleware. This should be called after New() and before the server
// starts listening.
//
// Example:
//
//	app.SetAPIDocument(myAPI.Document)
func (r *Regius) SetAPIDocument(doc *api.Document) {
	r.Scalar.Spec = doc
}

// registerScalarRoutes registers the Scalar API reference routes on the
// given chi.Mux. When ScalarConfig is disabled, this is a no-op.
func (r *Regius) registerScalarRoutes(mux *chi.Mux) {
	cfg := r.Scalar
	if !cfg.Enabled {
		return
	}

	mux.Get(cfg.SpecPath, r.serveOpenAPISpec)
	mux.Get(cfg.DocsPath, r.serveScalarPage)
}

// serveOpenAPISpec serves the OpenAPI spec. If SpecFile is set, the file
// is served as-is with the correct content type. Otherwise, the
// programmatic Spec is marshaled to JSON.
func (r *Regius) serveOpenAPISpec(w http.ResponseWriter, req *http.Request) {
	cfg := r.Scalar

	if cfg.SpecFile != "" {
		content, contentType, err := api.LoadSpecFile(cfg.SpecFile)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to load spec file: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(content)
		return
	}

	if cfg.Spec == nil {
		http.Error(w, "OpenAPI spec not configured", http.StatusNotFound)
		return
	}

	data, err := cfg.Spec.MarshalJSON()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to marshal spec: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// serveScalarPage renders the embedded HTML template that loads the
// Scalar JavaScript bundle and points it at the spec URL.
func (r *Regius) serveScalarPage(w http.ResponseWriter, req *http.Request) {
	cfg := r.Scalar

	tmpl, err := template.New("scalar").Parse(scalarHTMLTemplate)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to parse template: %v", err), http.StatusInternalServerError)
		return
	}

	data := scalarPageData{
		Title:       cfg.Title,
		CDNURL:      cfg.CDNURL,
		SpecURL:     cfg.SpecPath,
		Theme:       cfg.Theme,
		ShowClients: cfg.ShowClients,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("failed to render template: %v", err), http.StatusInternalServerError)
		return
	}
}
