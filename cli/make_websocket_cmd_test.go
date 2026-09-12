package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const wsRoutesFixture = `package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) routes() *chi.Mux {
	// routes
	a.get("/", a.Handlers.Home)
	a.get("/sse/ping", a.Handlers.SSEPing)

	// add any route here

	// static routes
	fileServer := http.FileServer(http.Dir("./public"))
	a.App.Routes.Handle("/public/*", http.StripPrefix("/public", fileServer))

	return a.App.Routes
}
`

// withWebsocketRoot points the CLI's global backend at a fresh temp dir
// with a routes.go fixture, the process env a PersistentPreRun would have
// set, and b.DBType loaded from it.
func withWebsocketRoot(t *testing.T, routes string) string {
	t.Helper()

	old := b.RootPath
	oldDBType := b.DBType
	dir := t.TempDir()
	b.RootPath = dir
	b.DBType = "sqlite"
	t.Setenv("APP_NAME", "testapp")
	t.Cleanup(func() {
		b.RootPath = old
		b.DBType = oldDBType
	})

	if err := os.WriteFile(filepath.Join(dir, "routes.go"), []byte(routes), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_NAME=testapp\nDATABASE_TYPE=sqlite\n"), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func readWsFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}

func TestDoMakeWebsocket_FirstRun(t *testing.T) {
	dir := withWebsocketRoot(t, wsRoutesFixture)

	if err := doMakeWebsocket("chat"); err != nil {
		t.Fatalf("doMakeWebsocket: %v", err)
	}

	// the handler file, with every placeholder replaced
	handler := readWsFile(t, filepath.Join(dir, "handlers", "ws_chat.go"))
	for _, want := range []string{
		"func (h *Handlers) WSChat(w http.ResponseWriter, r *http.Request)",
		"func WSBroadcastChat(app *regius.Regius, event string, payload any) error",
		"upgrader := &ws.Upgrader{",
		"conn.WriteEvent(ev)",
		"case \"chat.send\":", // normalized in the TODO example
	} {
		if !strings.Contains(handler, want) {
			t.Errorf("generated handler missing %q", want)
		}
	}
	if strings.Contains(handler, "$") {
		t.Errorf("generated handler still contains unreplaced placeholder:\n%s", handler)
	}

	// the route is mounted before the marker, which stays in place
	routes := readWsFile(t, filepath.Join(dir, "routes.go"))
	if !strings.Contains(routes, "a.get(\"/ws/chat\", a.Handlers.WSChat)") {
		t.Errorf("routes.go missing the mount:\n%s", routes)
	}
	if wsIdx, markerIdx := strings.Index(routes, "a.get(\"/ws/chat\""), strings.Index(routes, "// add any route here"); wsIdx < 0 || markerIdx < 0 || wsIdx > markerIdx {
		t.Errorf("route not inserted before the marker:\n%s", routes)
	}
}

func TestDoMakeWebsocket_SecondEndpointAppends(t *testing.T) {
	dir := withWebsocketRoot(t, wsRoutesFixture)

	if err := doMakeWebsocket("chat"); err != nil {
		t.Fatalf("first doMakeWebsocket: %v", err)
	}
	if err := doMakeWebsocket("live-scores"); err != nil {
		t.Fatalf("second doMakeWebsocket: %v", err)
	}

	routes := readWsFile(t, filepath.Join(dir, "routes.go"))
	for _, want := range []string{
		"a.get(\"/ws/chat\", a.Handlers.WSChat)",
		"a.get(\"/ws/live_scores\", a.Handlers.WSLiveScores)",
	} {
		if !strings.Contains(routes, want) {
			t.Errorf("routes.go missing %q:\n%s", want, routes)
		}
	}
	if strings.Count(routes, "// add any route here") != 1 {
		t.Errorf("marker duplicated:\n%s", routes)
	}
	if wsIdx, markerIdx := strings.Index(routes, "a.get(\"/ws/live_scores\""), strings.Index(routes, "// add any route here"); wsIdx < 0 || markerIdx < 0 || wsIdx > markerIdx {
		t.Errorf("second mount not before the marker:\n%s", routes)
	}

	if _, err := os.Stat(filepath.Join(dir, "handlers", "ws_live_scores.go")); err != nil {
		t.Errorf("second handler file missing: %v", err)
	}
}

func TestDoMakeWebsocket_DuplicateRefused(t *testing.T) {
	dir := withWebsocketRoot(t, wsRoutesFixture)

	if err := doMakeWebsocket("chat"); err != nil {
		t.Fatalf("first doMakeWebsocket: %v", err)
	}
	err := doMakeWebsocket("chat")
	if err == nil {
		t.Fatal("duplicate doMakeWebsocket succeeded, want refusal")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("duplicate error = %v, want already-exists refusal", err)
	}

	// the duplicate attempt must not double-wire the route either
	routes := readWsFile(t, filepath.Join(dir, "routes.go"))
	if strings.Count(routes, "a.get(\"/ws/chat\"") != 1 {
		t.Errorf("route duplicated by refused run:\n%s", routes)
	}
}

func TestDoMakeWebsocket_NameNormalization(t *testing.T) {
	tests := []struct {
		in, handler, wsName string
	}{
		{"chat", "WSChat", "chat"},
		{"live-scores", "WSLiveScores", "live_scores"},
		{"live_scores", "WSLiveScores", "live_scores"},
		// camelCase boundaries are not preserved: the name is lowercased
		// before normalization (job/webhook naming precedent)
		{"LiveScores", "WSLivescores", "livescores"},
		{"Presence-Channel", "WSPresenceChannel", "presence_channel"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			dir := withWebsocketRoot(t, wsRoutesFixture)

			if err := doMakeWebsocket(tt.in); err != nil {
				t.Fatalf("doMakeWebsocket(%q): %v", tt.in, err)
			}

			handler := readWsFile(t, filepath.Join(dir, "handlers", "ws_"+tt.wsName+".go"))
			if !strings.Contains(handler, "func (h *Handlers) "+tt.handler+"(") {
				t.Errorf("handler missing %q:\n%s", tt.handler, handler)
			}

			routes := readWsFile(t, filepath.Join(dir, "routes.go"))
			mount := "a.get(\"/ws/" + tt.wsName + "\", a.Handlers." + tt.handler + ")"
			if !strings.Contains(routes, mount) {
				t.Errorf("routes.go missing %q:\n%s", mount, routes)
			}
		})
	}
}

func TestDoMakeWebsocket_RouteMarkerFallback(t *testing.T) {
	// apps generated before the routes marker: fall back to right after
	// the home route
	noMarker := strings.Replace(wsRoutesFixture, "\t// add any route here\n\n", "", 1)
	dir := withWebsocketRoot(t, noMarker)

	if err := doMakeWebsocket("chat"); err != nil {
		t.Fatalf("doMakeWebsocket: %v", err)
	}

	routes := readWsFile(t, filepath.Join(dir, "routes.go"))
	homeIdx := strings.Index(routes, `a.get("/", a.Handlers.Home)`)
	wsIdx := strings.Index(routes, `a.get("/ws/chat", a.Handlers.WSChat)`)
	if homeIdx < 0 || wsIdx < 0 || wsIdx < homeIdx {
		t.Errorf("route not inserted after the home route:\n%s", routes)
	}
}

func TestWireWebsocketRoute_Idempotent(t *testing.T) {
	dir := withWebsocketRoot(t, wsRoutesFixture)
	routesPath := filepath.Join(dir, "routes.go")

	for i := 0; i < 2; i++ {
		if err := wireWebsocketRoute(routesPath, "Chat", "chat"); err != nil {
			t.Fatalf("wireWebsocketRoute #%d: %v", i+1, err)
		}
	}

	routes := readWsFile(t, routesPath)
	if got := strings.Count(routes, "a.get(\"/ws/chat\""); got != 1 {
		t.Errorf("route mounted %d times, want 1:\n%s", got, routes)
	}
}

func TestDoMakeWebsocket_MissingRoutesFile(t *testing.T) {
	dir := t.TempDir()

	old := b.RootPath
	b.RootPath = dir
	t.Cleanup(func() { b.RootPath = old })

	if err := doMakeWebsocket("chat"); err == nil {
		t.Fatal("doMakeWebsocket without routes.go succeeded, want error")
	}
}
