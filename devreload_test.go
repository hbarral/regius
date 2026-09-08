package regius

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func devReloadHandler(t *testing.T, cfg DevReloadConfig, next http.Handler) http.Handler {
	t.Helper()
	r := &Regius{}
	return r.DevReload(cfg)(next)
}

func TestDevReload_DisabledPassthrough(t *testing.T) {
	body := "<html><body>hello</body></html>"
	handler := devReloadHandler(t, DevReloadConfig{Enabled: false}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))

	// The dev endpoints are not mounted when disabled.
	for _, path := range []string{"/__dev/reload.js", "/__dev/stream", "/__dev/notify"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Body.String() != body {
			t.Errorf("disabled middleware modified the %s response: %q", path, rr.Body.String())
		}
	}
}

func TestDevReload_Injection(t *testing.T) {
	scriptTag := `<script src="/__dev/reload.js" defer></script>`

	cases := []struct {
		name        string
		method      string
		contentType string
		body        string
		setLength   bool
		flushEarly  bool
		wantScript  bool
	}{
		{
			name:        "html page",
			contentType: "text/html",
			body:        "<html><head></head><body><p>hi</p></body></html>",
			wantScript:  true,
		},
		{
			name:        "html fragment without body tag",
			contentType: "text/html",
			body:        "<div>partial response</div>",
			wantScript:  false,
		},
		{
			name:        "json response",
			contentType: "application/json",
			body:        `{"ok":true}`,
			wantScript:  false,
		},
		{
			name:        "no content type, plain text",
			contentType: "",
			body:        "plain text",
			wantScript:  false,
		},
		{
			// handlers may leave Content-Type unset (net/http sniffs it
			// at the connection) — HTML pages must still be injected
			name:        "no content type, html body",
			contentType: "",
			body:        "<html><body>sniffed</body></html>",
			wantScript:  true,
		},
		{
			name:        "html with explicit content-length",
			contentType: "text/html",
			body:        "<html><body>x</body></html>",
			setLength:   true,
			wantScript:  true,
		},
		{
			name:        "uppercase body tag",
			contentType: "text/html; charset=utf-8",
			body:        "<HTML><BODY>x</BODY></HTML>",
			wantScript:  true,
		},
		{
			name:        "spaced body tag",
			contentType: "text/html",
			body:        "<html><body>x</body ></html>",
			wantScript:  true,
		},
		{
			name:        "head request",
			method:      http.MethodHead,
			contentType: "text/html",
			body:        "<html><body>x</body></html>",
			wantScript:  false,
		},
		{
			name:        "streaming response flushed early",
			contentType: "text/html",
			body:        "<html><body>x</body></html>",
			flushEarly:  true,
			wantScript:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			method := c.method
			if method == "" {
				method = http.MethodGet
			}
			handler := devReloadHandler(t, DevReloadConfig{Enabled: true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if c.contentType != "" {
					w.Header().Set("Content-Type", c.contentType)
				}
				if c.setLength {
					w.Header().Set("Content-Length", "999")
				}
				w.WriteHeader(http.StatusOK)
				// write in two passes to exercise chunked writes
				half := len(c.body) / 2
				_, _ = w.Write([]byte(c.body[:half]))
				if c.flushEarly {
					w.(http.Flusher).Flush()
				}
				_, _ = w.Write([]byte(c.body[half:]))
			}))

			req := httptest.NewRequest(method, "/page", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			got := rr.Body.String()
			if c.wantScript {
				if !strings.Contains(got, scriptTag) {
					t.Fatalf("script tag not injected:\n%s", got)
				}
				if !strings.Contains(got, scriptTag+"</body") && !strings.Contains(got, scriptTag+"</BODY") && !strings.Contains(got, scriptTag+"</body ") {
					t.Errorf("script tag not placed before the closing body tag:\n%s", got)
				}
				if c.setLength && rr.Header().Get("Content-Length") != "" {
					t.Errorf("Content-Length should be dropped when injecting, got %q", rr.Header().Get("Content-Length"))
				}
			} else {
				if strings.Contains(got, "reload.js") {
					t.Fatalf("unexpected injection:\n%s", got)
				}
				if got != c.body {
					t.Errorf("body modified without injection:\ngot  %q\nwant %q", got, c.body)
				}
			}
			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rr.Code)
			}
		})
	}
}

func TestDevReload_StatusPreservedWhileBuffering(t *testing.T) {
	handler := devReloadHandler(t, DevReloadConfig{Enabled: true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("<html><body>short and stout</body></html>"))
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/page", nil))
	if rr.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "reload.js") {
		t.Error("expected injection on error-status HTML page")
	}
}

func TestDevReload_ScriptAsset(t *testing.T) {
	handler := devReloadHandler(t, DevReloadConfig{Enabled: true, Path: "/__custom"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler must not run for dev endpoints")
	}))

	req := httptest.NewRequest(http.MethodGet, "/__custom/reload.js", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	// the client script must point at the configured base path
	if !strings.Contains(rr.Body.String(), `new EventSource("/__custom/stream")`) {
		t.Errorf("script does not use the configured path:\n%s", rr.Body.String())
	}
}

func TestDevReload_StreamBootEvent(t *testing.T) {
	handler := devReloadHandler(t, DevReloadConfig{Enabled: true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler must not run for dev endpoints")
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/__dev/stream", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rr, req)
		close(done)
	}()

	// poll until the boot event has been written
	deadline := time.Now().Add(3 * time.Second)
	for {
		if strings.Contains(rr.Body.String(), "event: boot") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no boot event within deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "retry: 500") {
		t.Errorf("boot event missing the fast reconnect hint:\n%s", body)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream handler did not stop after context cancellation")
	}
}

func TestDevReload_Notify(t *testing.T) {
	handler := devReloadHandler(t, DevReloadConfig{Enabled: true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	loopback := httptest.NewRequest(http.MethodPost, "/__dev/notify", nil)
	loopback.RemoteAddr = "127.0.0.1:55555"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, loopback)
	if rr.Code != http.StatusOK {
		t.Errorf("loopback notify status = %d, want 200", rr.Code)
	}

	remote := httptest.NewRequest(http.MethodPost, "/__dev/notify", nil)
	remote.RemoteAddr = "203.0.113.7:55555"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, remote)
	if rr.Code != http.StatusForbidden {
		t.Errorf("remote notify status = %d, want 403", rr.Code)
	}

	spoofed := httptest.NewRequest(http.MethodPost, "/__dev/notify", nil)
	spoofed.RemoteAddr = "127.0.0.1:55555"
	spoofed.Header.Set("X-Forwarded-For", "203.0.113.7")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, spoofed)
	if rr.Code != http.StatusForbidden {
		t.Errorf("spoofed XFF notify status = %d, want 403", rr.Code)
	}
}

func TestDevReloadHub_Broadcast(t *testing.T) {
	hub := newDevReloadHub()
	events, unsubscribe := hub.subscribe()
	defer unsubscribe()

	hub.broadcast(SSEEvent{Event: "reload", Data: []byte("css")})

	select {
	case ev := <-events:
		if ev.Event != "reload" || string(ev.Data) != "css" {
			t.Errorf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}

	// unsubscribe removes the client
	unsubscribe()
	hub.broadcast(SSEEvent{Event: "reload"})
	select {
	case ev := <-events:
		t.Errorf("received event after unsubscribe: %+v", ev)
	default:
	}
}

func TestDevReload_UnknownDevPath(t *testing.T) {
	handler := devReloadHandler(t, DevReloadConfig{Enabled: true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/__dev/nope", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestInjectBeforeBodyEnd(t *testing.T) {
	script := "<script src=\"s\"></script>"
	cases := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{"plain", "<html><body>a</body></html>", "<html><body>a" + script + "</body></html>", true},
		{"upper", "<HTML><BODY>a</BODY></HTML>", "<HTML><BODY>a" + script + "</BODY></HTML>", true},
		{"spaced", "<html><body>a</body ></html>", "<html><body>a" + script + "</body ></html>", true},
		{"last of two", "<body>a</body><body>b</body>", "<body>a</body><body>b" + script + "</body>", true},
		{"fragment", "<div>a</div>", "<div>a</div>", false},
		{"empty", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := injectBeforeBodyEnd([]byte(c.body), script)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if string(got) != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
