package regius

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func wsDialURL(serverURL, path string) string {
	return "ws://" + strings.TrimPrefix(strings.TrimPrefix(serverURL, "https://"), "http://") + path
}

// TestWS_DisabledByDefault verifies the /ws route is not mounted unless
// WS_ENABLED is set, while the hub itself is always constructed.
func TestWS_DisabledByDefault(t *testing.T) {
	r := newTestApp(t, nil)

	if r.WS == nil {
		t.Fatal("r.WS is nil, want the hub always constructed")
	}

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ws")
	if err != nil {
		t.Fatalf("GET /ws error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /ws status = %d, want %d (route must be opt-in)", resp.StatusCode, http.StatusNotFound)
	}

	// Broadcasting without a mounted route is a harmless no-op, mirroring
	// the SSE broker.
	if err := r.WSBroadcastJSON("ping", "pong"); err != nil {
		t.Fatalf("WSBroadcastJSON() on disabled app error = %v", err)
	}
}

func TestWS_EnabledRoundTrip(t *testing.T) {
	r := newTestApp(t, map[string]string{"WS_ENABLED": "true"})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsDialURL(ts.URL, "/ws"), nil)
	if err != nil {
		t.Fatalf("dial /ws error = %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	if err := r.WSBroadcastJSON("ping", map[string]string{"message": "pong"}); err != nil {
		t.Fatalf("WSBroadcastJSON() error = %v", err)
	}

	var ev struct {
		Event string `json:"event"`
		Data  struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if ev.Event != "ping" || ev.Data.Message != "pong" {
		t.Fatalf("received %+v, want event=ping message=pong", ev)
	}
}

// TestWS_CustomPath verifies WS_PATH relocates the mount.
func TestWS_CustomPath(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"WS_ENABLED": "true",
		"WS_PATH":    "/socket",
	})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsDialURL(ts.URL, "/socket"), nil)
	if err != nil {
		t.Fatalf("dial /socket error = %v", err)
	}
	conn.Close()

	resp, err := http.Get(ts.URL + "/ws")
	if err != nil {
		t.Fatalf("GET /ws error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /ws status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// TestWS_OriginRejected verifies the default mount enforces the origin
// policy: with WS_ALLOW_EMPTY_ORIGIN=false, the gorilla dialer (which sends
// no Origin header) must be rejected.
func TestWS_OriginRejected(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"WS_ENABLED":            "true",
		"WS_ALLOW_EMPTY_ORIGIN": "false",
	})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	if _, _, err := websocket.DefaultDialer.Dial(wsDialURL(ts.URL, "/ws"), nil); err == nil {
		t.Fatal("dial without Origin header succeeded, want 403 rejection")
	}
}

// TestWS_AllowedOrigins verifies WS_ALLOWED_ORIGINS extends the policy for
// cross-origin browser clients while unknown origins stay rejected.
func TestWS_AllowedOrigins(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"WS_ENABLED":         "true",
		"WS_ALLOWED_ORIGINS": "trusted.example",
	})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsDialURL(ts.URL, "/ws"), http.Header{
		"Origin": []string{"https://trusted.example"},
	})
	if err != nil {
		t.Fatalf("dial with allowed origin error = %v", err)
	}
	conn.Close()

	if _, _, err := websocket.DefaultDialer.Dial(wsDialURL(ts.URL, "/ws"), http.Header{
		"Origin": []string{"https://evil.example"},
	}); err == nil {
		t.Fatal("dial with unlisted origin succeeded, want rejection")
	}
}

// TestWS_MaintenanceBypass documents and asserts the outer-mux trade-off:
// while maintenance mode blocks app routes, the default WebSocket mount
// (like the SSE stream and jobs dashboard) keeps serving handshakes.
// Authenticated sockets that must respect maintenance should mount
// r.WS.Handler under r.Routes instead.
func TestWS_MaintenanceBypass(t *testing.T) {
	r := newTestApp(t, map[string]string{"WS_ENABLED": "true"})
	r.Routes.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	maintenanceMode = true
	defer func() { maintenanceMode = false }()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET / status = %d, want %d (maintenance)", resp.StatusCode, http.StatusServiceUnavailable)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsDialURL(ts.URL, "/ws"), nil)
	if err != nil {
		t.Fatalf("dial /ws during maintenance error = %v, want outer-mux bypass", err)
	}
	conn.Close()
}

func TestWS_ConfigDefaults(t *testing.T) {
	r := &Regius{}
	cfg := r.createWSConfig()

	if cfg.enabled {
		t.Error("enabled = true, want false (opt-in)")
	}
	if cfg.path != "/ws" {
		t.Errorf("path = %q, want /ws", cfg.path)
	}
	if len(cfg.allowedOrigins) != 0 {
		t.Errorf("allowedOrigins = %v, want empty", cfg.allowedOrigins)
	}
	if !cfg.allowEmptyOrigin {
		t.Error("allowEmptyOrigin = false, want true (non-browser clients)")
	}
	if cfg.heartbeat != 30*time.Second {
		t.Errorf("heartbeat = %v, want 30s", cfg.heartbeat)
	}
	if cfg.writeTimeout != 10*time.Second {
		t.Errorf("writeTimeout = %v, want 10s", cfg.writeTimeout)
	}
	if cfg.pongTimeout != 60*time.Second {
		t.Errorf("pongTimeout = %v, want 60s", cfg.pongTimeout)
	}
	if cfg.maxMessageSize != 32*1024 {
		t.Errorf("maxMessageSize = %d, want 32768", cfg.maxMessageSize)
	}
	if cfg.clientBuffer != 16 {
		t.Errorf("clientBuffer = %d, want 16", cfg.clientBuffer)
	}
	if cfg.maxClients != 0 {
		t.Errorf("maxClients = %d, want 0 (unlimited)", cfg.maxClients)
	}
}

func TestWS_ConfigOverrides(t *testing.T) {
	overrides := map[string]string{
		"WS_ENABLED":            "true",
		"WS_PATH":               "/socket",
		"WS_ALLOWED_ORIGINS":    "one.example, two.example ,",
		"WS_ALLOW_EMPTY_ORIGIN": "false",
		"WS_HEARTBEAT":          "5s",
		"WS_WRITE_TIMEOUT":      "2s",
		"WS_PONG_TIMEOUT":       "15s",
		"WS_MAX_MESSAGE_SIZE":   "1024",
		"WS_CLIENT_BUFFER":      "8",
		"WS_MAX_CLIENTS":        "64",
	}
	for k, v := range overrides {
		t.Setenv(k, v)
	}

	r := &Regius{}
	cfg := r.createWSConfig()

	if !cfg.enabled || cfg.path != "/socket" {
		t.Errorf("enabled/path = %v/%q, want true//socket", cfg.enabled, cfg.path)
	}
	if len(cfg.allowedOrigins) != 2 || cfg.allowedOrigins[0] != "one.example" || cfg.allowedOrigins[1] != "two.example" {
		t.Errorf("allowedOrigins = %v, want [one.example two.example]", cfg.allowedOrigins)
	}
	if cfg.allowEmptyOrigin {
		t.Error("allowEmptyOrigin = true, want false")
	}
	if cfg.heartbeat != 5*time.Second || cfg.writeTimeout != 2*time.Second || cfg.pongTimeout != 15*time.Second {
		t.Errorf("durations = %v/%v/%v, want 5s/2s/15s", cfg.heartbeat, cfg.writeTimeout, cfg.pongTimeout)
	}
	if cfg.maxMessageSize != 1024 || cfg.clientBuffer != 8 || cfg.maxClients != 64 {
		t.Errorf("limits = %d/%d/%d, want 1024/8/64", cfg.maxMessageSize, cfg.clientBuffer, cfg.maxClients)
	}
}

// TestWS_ConfigInvalidValues verifies malformed values fall back to the
// defaults instead of breaking startup.
func TestWS_ConfigInvalidValues(t *testing.T) {
	for k, v := range map[string]string{
		"WS_ENABLED":            "maybe",
		"WS_PATH":               "",
		"WS_HEARTBEAT":          "not-a-duration",
		"WS_MAX_MESSAGE_SIZE":   "-5",
		"WS_CLIENT_BUFFER":      "zero",
		"WS_MAX_CLIENTS":        "-10",
		"WS_ALLOW_EMPTY_ORIGIN": "maybe",
	} {
		t.Setenv(k, v)
	}

	r := &Regius{}
	cfg := r.createWSConfig()

	if cfg.enabled {
		t.Error("enabled parsed true from \"maybe\", want false")
	}
	if cfg.path != "/ws" {
		t.Errorf("path = %q, want /ws fallback", cfg.path)
	}
	if cfg.heartbeat != 30*time.Second {
		t.Errorf("heartbeat = %v, want 30s fallback", cfg.heartbeat)
	}
	if cfg.maxMessageSize != 32*1024 {
		t.Errorf("maxMessageSize = %d, want 32768 fallback", cfg.maxMessageSize)
	}
	if cfg.clientBuffer != 16 {
		t.Errorf("clientBuffer = %d, want 16 fallback", cfg.clientBuffer)
	}
	if cfg.maxClients != 0 {
		t.Errorf("maxClients = %d, want 0 fallback", cfg.maxClients)
	}
	if !cfg.allowEmptyOrigin {
		t.Error("allowEmptyOrigin = false, want true fallback")
	}
}

func TestWSBroadcastJSON_UninitializedHub(t *testing.T) {
	r := &Regius{}
	if err := r.WSBroadcastJSON("ping", "pong"); err == nil {
		t.Fatal("WSBroadcastJSON() on zero Regius succeeded, want error")
	}
}
