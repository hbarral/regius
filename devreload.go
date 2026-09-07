package regius

import (
	"bytes"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DevReloadConfig configures the development browser live-reload middleware.
//
// When enabled, the middleware mounts three dev-only endpoints under Path
// and injects a <script> tag into text/html responses so open browser tabs
// reload automatically after `regius dev` restarts the app (detected via a
// per-process boot ID announced on the SSE stream) or after a CSS-only
// rebuild (an explicit reload event pushed to the notify endpoint).
//
// It is disabled by default and must never be enabled in production: the
// notify endpoint can force connected tabs to reload. `regius dev` turns it
// on automatically via the DEV_RELOAD_ENABLED environment variable.
type DevReloadConfig struct {
	Enabled bool

	// Path is the base path for the dev endpoints. Defaults to "/__dev".
	// The endpoints are Path+"/reload.js" (script asset), Path+"/stream"
	// (SSE), and Path+"/notify" (loopback-only POST trigger).
	Path string
}

const defaultDevReloadPath = "/__dev"

// devReloadScript is the browser client. The browser's native EventSource
// reconnect (with the 500ms retry hint sent by the stream) handles the
// server-down window; the boot event carries a per-process ID so a reload
// only happens when the process behind the endpoint actually changed, and
// the reload event covers CSS-only rebuilds that do not restart the app.
const devReloadScript = `(function () {
	"use strict";
	var lastBoot = null;
	var es = new EventSource("__PATH__/stream");
	es.addEventListener("boot", function (e) {
		if (lastBoot !== null && lastBoot !== e.data) { location.reload(); }
		lastBoot = e.data;
	});
	es.addEventListener("reload", function () { location.reload(); });
})();
`

// devReloadHub is a minimal per-middleware SSE client registry. It mirrors
// the SSEBroker pattern (buffered channels, non-blocking broadcast) but
// stays private: this transport is internal to the dev middleware.
type devReloadHub struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

func newDevReloadHub() *devReloadHub {
	return &devReloadHub{clients: make(map[chan SSEEvent]struct{})}
}

func (h *devReloadHub) subscribe() (<-chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 4)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}
}

func (h *devReloadHub) broadcast(ev SSEEvent) {
	h.mu.RLock()
	clients := make([]chan SSEEvent, 0, len(h.clients))
	for ch := range h.clients {
		clients = append(clients, ch)
	}
	h.mu.RUnlock()

	for _, ch := range clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

// DevReload returns the browser live-reload middleware. When Enabled is
// false it returns a no-op passthrough and mounts nothing.
//
// The middleware is wired on the outer mux (before app-level session/CSRF/
// sanitizer middleware) so the dev endpoints work regardless of app routes,
// and so it runs upstream of any response-compressing middleware. It must
// be wired before any middleware that modifies response bodies.
func (r *Regius) DevReload(cfg DevReloadConfig) func(next http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	path := cfg.Path
	if path == "" {
		path = defaultDevReloadPath
	}
	path = strings.TrimSuffix(path, "/")

	hub := newDevReloadHub()
	script := strings.ReplaceAll(devReloadScript, "__PATH__", path)
	scriptTag := `<script src="` + path + `/reload.js" defer></script>`

	// The boot ID identifies the process serving the stream: middleware
	// construction happens once per process during New(), so a restart
	// always yields a new ID and a reconnected tab knows to reload.
	bootID := strconv.FormatInt(time.Now().UnixMilli(), 10)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Dev endpoints are served here, before the injection wrapper.
			if strings.HasPrefix(req.URL.Path, path+"/") {
				switch {
				case req.Method == http.MethodGet && req.URL.Path == path+"/reload.js":
					serveDevReloadScript(w, script)
				case req.Method == http.MethodGet && req.URL.Path == path+"/stream":
					serveDevReloadStream(w, req, hub, bootID)
				case req.Method == http.MethodPost && req.URL.Path == path+"/notify":
					serveDevReloadNotify(w, req, hub)
				default:
					http.NotFound(w, req)
				}
				return
			}

			if req.Method == http.MethodHead {
				next.ServeHTTP(w, req)
				return
			}

			iw := &injectingWriter{ResponseWriter: w, scriptTag: scriptTag}
			defer iw.finish()
			next.ServeHTTP(iw, req)
		})
	}
}

func serveDevReloadScript(w http.ResponseWriter, script string) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

func serveDevReloadStream(w http.ResponseWriter, req *http.Request, hub *devReloadHub, bootID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Announce this process immediately, and hint a fast reconnect so tabs
	// pick up a restarted app quickly.
	if err := writeEvent(w, SSEEvent{Event: "boot", Data: []byte(bootID), Retry: 500}); err != nil {
		return
	}
	flusher.Flush()

	events, unsubscribe := hub.subscribe()
	defer unsubscribe()

	for {
		select {
		case <-req.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := writeEvent(w, ev); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// serveDevReloadNotify broadcasts a reload event to every connected tab.
// It is only reachable from the local machine: the request must come from
// a loopback address and must not carry proxy headers (which chi's RealIP
// middleware would otherwise use to rewrite RemoteAddr).
func serveDevReloadNotify(w http.ResponseWriter, req *http.Request, hub *devReloadHub) {
	if !isLoopbackRequest(req) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"notify is local-only"}`))
		return
	}

	hub.broadcast(SSEEvent{Event: "reload", Data: []byte("css")})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func isLoopbackRequest(req *http.Request) bool {
	if req.Header.Get("X-Forwarded-For") != "" || req.Header.Get("X-Real-IP") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// injectingWriter buffers text/html responses so a <script> tag can be
// inserted before the closing </body> tag once the handler returns. Any
// other content type passes straight through, and a mid-stream Flush gives
// up injection for that response (writing the buffered bytes through
// unmodified) so streaming handlers keep working.
//
// This is the framework's first response-body-modifying middleware: any
// future body-modifying middleware (e.g. compression) must be wired after
// DevReload on the chain.
type injectingWriter struct {
	http.ResponseWriter
	scriptTag string

	headerWritten bool
	status        int
	buffering     bool
	// sniff is set when the handler left Content-Type unset: net/http
	// would sniff the body at the connection level, so the decision is
	// deferred to finish() using the same http.DetectContentType.
	sniff bool
	buf   bytes.Buffer
}

func (w *injectingWriter) decide() {
	ct := strings.ToLower(strings.TrimSpace(w.Header().Get("Content-Type")))
	switch {
	case ct == "":
		w.buffering = true
		w.sniff = true
	case strings.HasPrefix(ct, "text/html"):
		w.buffering = true
	}
}

func (w *injectingWriter) WriteHeader(code int) {
	if w.headerWritten {
		return
	}
	w.headerWritten = true
	w.status = code

	w.decide()
	if !w.buffering {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *injectingWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	if w.buffering {
		return w.buf.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap supports http.ResponseController and middleware tooling that needs
// to reach the underlying writer.
func (w *injectingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush streams what has been buffered so far and disables injection for
// the rest of the response: once bytes have left the process un-injected,
// it is too late to add the script tag.
func (w *injectingWriter) Flush() {
	if !w.headerWritten {
		w.headerWritten = true
		w.status = http.StatusOK
		w.decide()
	}
	if w.buffering {
		w.buffering = false
		w.ResponseWriter.WriteHeader(w.status)
		if w.buf.Len() > 0 {
			_, _ = w.ResponseWriter.Write(w.buf.Bytes())
			w.buf.Reset()
		}
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// finish writes out a buffered response, injecting the script tag before
// the last </body> when the response is a full HTML document. Fragments
// (no closing body tag) are written through untouched: appending to
// partial responses would corrupt them. When the handler left
// Content-Type unset, the buffered body is sniffed with
// http.DetectContentType — the same rule net/http applies at the
// connection — so HTML pages render identically with and without the
// middleware.
func (w *injectingWriter) finish() {
	if !w.buffering {
		return
	}
	w.buffering = false

	body := w.buf.Bytes()
	inject := !w.sniff // explicit text/html: always a candidate
	if w.sniff {
		if ct := http.DetectContentType(body); strings.HasPrefix(ct, "text/html") {
			w.Header().Set("Content-Type", ct)
			inject = true
		}
	}
	if inject {
		if injected, ok := injectBeforeBodyEnd(body, w.scriptTag); ok {
			body = injected
			// The length changed; let net/http use chunked encoding.
			w.Header().Del("Content-Length")
		}
	}

	w.ResponseWriter.WriteHeader(w.status)
	if len(body) > 0 {
		_, _ = w.ResponseWriter.Write(body)
	}
}

// injectBeforeBodyEnd inserts script immediately before the last closing
// body tag (case-insensitive, tolerant of "</body >" spacing). It reports
// false when no closing tag exists.
func injectBeforeBodyEnd(body []byte, script string) ([]byte, bool) {
	lowered := bytes.ToLower(body)
	idx := bytes.LastIndex(lowered, []byte("</body"))
	if idx < 0 {
		return body, false
	}

	out := make([]byte, 0, len(body)+len(script))
	out = append(out, body[:idx]...)
	out = append(out, script...)
	out = append(out, body[idx:]...)
	return out, true
}
