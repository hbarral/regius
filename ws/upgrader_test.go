package ws

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCheckSameOrigin(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		host    string
		wantErr bool
	}{
		{"empty origin allowed", "", "example.com", false},
		{"same host allowed", "https://example.com", "example.com", false},
		{"same host different scheme allowed", "http://example.com", "example.com", false},
		{"host with port matches", "http://example.com:4000", "example.com:4000", false},
		{"different host rejected", "https://evil.example", "example.com", true},
		{"different port rejected", "http://example.com:8080", "example.com:4000", true},
		{"subdomain rejected", "https://sub.example.com", "example.com", true},
		{"malformed origin rejected", "://not-a-url", "example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/ws", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			r.Host = tt.host

			err := CheckSameOrigin(r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckSameOrigin() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAllowOrigins(t *testing.T) {
	checker := AllowOrigins("example.com", "https://app.example.org")

	tests := []struct {
		name    string
		origin  string
		host    string
		wantErr bool
	}{
		{"listed bare host allowed", "https://example.com", "other.example", false},
		{"listed url allowed with scheme dropped", "https://app.example.org", "other.example", false},
		{"empty origin allowed", "", "other.example", false},
		{"unlisted host rejected", "https://evil.example", "other.example", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/ws", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			r.Host = tt.host

			err := checker(r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("AllowOrigins()() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAllowOrigins_Wildcard(t *testing.T) {
	checker := AllowOrigins("*")
	for _, origin := range []string{"https://anything.example", "http://other.example:4000"} {
		r := httptest.NewRequest(http.MethodGet, "http://server.example/ws", nil)
		r.Header.Set("Origin", origin)
		r.Host = "server.example"
		if err := checker(r); err != nil {
			t.Fatalf("origin %q rejected by wildcard, want allowed", origin)
		}
	}
}

func TestRequireOrigin(t *testing.T) {
	checker := RequireOrigin(CheckSameOrigin)

	r := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	r.Host = "example.com"
	if err := checker(r); err == nil {
		t.Fatal("empty origin admitted by RequireOrigin, want rejection")
	}

	r.Header.Set("Origin", "https://example.com")
	if err := checker(r); err != nil {
		t.Fatalf("same-origin request rejected: %v", err)
	}

	r.Header.Set("Origin", "https://evil.example")
	if err := checker(r); err == nil {
		t.Fatal("cross-origin request admitted, want rejection")
	}
}

func TestAllowOrigins_ExplicitListOnly(t *testing.T) {
	checker := AllowOrigins("good.example")

	tests := []struct {
		name    string
		origin  string
		wantErr bool
	}{
		{"listed allowed", "https://good.example", false},
		{"empty allowed", "", false},
		{"same-host-but-unlisted rejected", "https://server.example", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://server.example/ws", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			r.Host = "server.example"

			err := checker(r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpgrade_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&Upgrader{}).Upgrade(w, r)
		if err != nil {
			t.Errorf("Upgrade() error = %v", err)
			return
		}
		defer conn.Close("done")

		if err := conn.WriteEvent(Event{Event: "hello", Data: []byte(`"world"`)}); err != nil {
			t.Errorf("WriteEvent() error = %v", err)
		}
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial error = %v", err)
	}
	defer conn.Close()

	var ev Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if ev.Event != "hello" || string(ev.Data) != `"world"` {
		t.Fatalf("received %+v, want event=hello data=\"world\"", ev)
	}
}

func TestUpgrade_NotWebSocket(t *testing.T) {
	upgrader := &Upgrader{}

	tests := []struct {
		name   string
		method string
		header map[string]string
	}{
		{"post request", http.MethodPost, wsHeaders()},
		{"get without upgrade headers", http.MethodGet, nil},
		{"get with connection only", http.MethodGet, map[string]string{"Connection": "upgrade"}},
		{"get with upgrade only", http.MethodGet, map[string]string{"Upgrade": "websocket"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "/ws", nil)
			for k, v := range tt.header {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()

			_, err := upgrader.Upgrade(w, r)
			if !errors.Is(err, ErrNotWebSocket) {
				t.Fatalf("Upgrade() error = %v, want ErrNotWebSocket", err)
			}
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestUpgrade_OriginRejected(t *testing.T) {
	server := httptest.NewServer((&Upgrader{}).UpgradeHandler())
	defer server.Close()

	header := http.Header{"Origin": []string{"http://evil.example"}}
	resp, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), header)
	if err == nil {
		conn := resp
		conn.Close()
		t.Fatal("dial succeeded with cross-origin header, want rejection")
	}
}

func TestUpgrade_OriginRejected_TypedError(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	for k, v := range wsHeaders() {
		r.Header.Set(k, v)
	}
	r.Header.Set("Origin", "http://evil.example")
	r.Host = "good.example"
	w := httptest.NewRecorder()

	_, err := (&Upgrader{}).Upgrade(w, r)
	if !errors.Is(err, ErrOriginRejected) {
		t.Fatalf("Upgrade() error = %v, want ErrOriginRejected", err)
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// hijackableRecorder lets recorder-based tests pass the upgrader's
// hijackability walk; the handshake-failure paths under test return
// before any hijack happens, so the stub never runs.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
}

func (hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not really hijackable")
}

func TestUpgrade_UpgradeFailed(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	for k, v := range wsHeaders() {
		r.Header.Set(k, v)
	}
	r.Header.Set("Sec-WebSocket-Version", "12") // unsupported version
	w := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}

	_, err := (&Upgrader{}).Upgrade(w, r)
	if !errors.Is(err, ErrUpgradeFailed) {
		t.Fatalf("Upgrade() error = %v, want ErrUpgradeFailed", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpgrade_AllowedOriginSucceeds(t *testing.T) {
	upgrader := &Upgrader{CheckOrigin: AllowOrigins("trusted.example")}
	server := httptest.NewServer(upgrader.UpgradeHandler())
	defer server.Close()

	header := http.Header{"Origin": []string{"https://trusted.example"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), header)
	if err != nil {
		t.Fatalf("dial error = %v", err)
	}
	conn.Close()
}

// UpgradeHandler adapts Upgrade to an http.HandlerFunc for tests that
// exercise the HTTP layer end-to-end.
func (u *Upgrader) UpgradeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := u.Upgrade(w, r)
		if err != nil {
			return
		}
		conn.Close("done")
	})
}

func TestConn_CloseHandshake(t *testing.T) {
	upgrader := &Upgrader{WriteTimeout: time.Second}
	server := httptest.NewServer(upgrader.UpgradeHandler())
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial error = %v", err)
	}
	defer conn.Close()

	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("ReadMessage() error = %v, want close error", err)
	}
	if closeErr.Code != websocket.CloseNormalClosure {
		t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.CloseNormalClosure)
	}
	if closeErr.Text != "done" {
		t.Fatalf("close text = %q, want %q", closeErr.Text, "done")
	}
}

// unwrappingWrapper mimics middleware like the scs session writer: it
// wraps the ResponseWriter without promoting http.Hijacker, but exposes
// Unwrap so the underlying writer is reachable.
type unwrappingWrapper struct {
	http.ResponseWriter
	wroteHeader bool
}

func (uw *unwrappingWrapper) WriteHeader(code int) {
	uw.wroteHeader = true
	uw.ResponseWriter.WriteHeader(code)
}

func (uw *unwrappingWrapper) Write(b []byte) (int, error) {
	uw.wroteHeader = true
	return uw.ResponseWriter.Write(b)
}

func (uw *unwrappingWrapper) Unwrap() http.ResponseWriter {
	return uw.ResponseWriter
}

// TestUpgrade_ThroughWrappingMiddleware is the regression test for the
// scs case: an upgrade must succeed through a middleware writer that
// implements Unwrap but not http.Hijacker. Without the chain walk,
// gorilla's direct assertion fails and the handshake 500s.
func TestUpgrade_ThroughWrappingMiddleware(t *testing.T) {
	upgrader := &Upgrader{WriteTimeout: time.Second}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uw := &unwrappingWrapper{ResponseWriter: w}
		conn, err := upgrader.Upgrade(uw, r)
		if err != nil {
			t.Errorf("Upgrade() through wrapping middleware error = %v", err)
			return
		}
		defer conn.Close("done")
		if err := conn.WriteEvent(Event{Event: "hello", Data: []byte(`"wrapped"`)}); err != nil {
			t.Errorf("WriteEvent() error = %v", err)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial through wrapping middleware error = %v", err)
	}
	defer conn.Close()

	var ev Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if ev.Event != "hello" || string(ev.Data) != `"wrapped"` {
		t.Fatalf("received %+v, want hello/wrapped", ev)
	}
}

// TestUpgrade_UnhijackableWriter verifies a clear typed error (and a 500
// to the client) when the wrapper chain bottoms out without a hijackable
// writer.
func TestUpgrade_UnhijackableWriter(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// strip hijackability: a wrapper with no Unwrap and no Hijacker
		deaf := &struct{ http.ResponseWriter }{w}
		_, err := (&Upgrader{}).Upgrade(deaf, r)
		if !errors.Is(err, ErrUpgradeFailed) {
			t.Errorf("Upgrade() error = %v, want ErrUpgradeFailed", err)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	_, resp, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err == nil {
		t.Fatal("dial succeeded through unhijackable writer, want 500")
	}
	if resp == nil {
		t.Fatalf("dial error without response: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

func wsHeaders() map[string]string {
	return map[string]string{
		"Connection":            "Upgrade",
		"Upgrade":               "websocket",
		"Sec-WebSocket-Version": "13",
		"Sec-WebSocket-Key":     "dGhlIHNhbXBsZSBub25jZQ==",
	}
}

func wsURL(httpURL string) string {
	if len(httpURL) >= 8 && httpURL[:8] == "https://" {
		return "ws://" + httpURL[8:]
	}
	if len(httpURL) >= 7 && httpURL[:7] == "http://" {
		return "ws://" + httpURL[7:]
	}
	return "ws://" + httpURL
}
