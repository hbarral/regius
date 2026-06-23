package regius

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

func TestRequestID_Disabled(t *testing.T) {
	r := &Regius{}
	handler := r.RequestID(RequestIDConfig{Enabled: false})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Even when the middleware is disabled, chi's RequestIDKey should
		// not have been populated by this middleware.
		if id, ok := RequestIDFromContext(req.Context()); ok {
			t.Errorf("expected no request id when disabled, got %q", id)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Request-ID"); got != "" {
		t.Errorf("expected no X-Request-ID response header when disabled, got %q", got)
	}
}

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	r := &Regius{}
	var got string
	handler := r.RequestID(RequestIDConfig{Enabled: true, Format: RequestIDFormatUUID})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, ok := RequestIDFromContext(req.Context())
			if !ok || id == "" {
				t.Errorf("expected a generated request id in context, got %q (ok=%v)", id, ok)
			}
			got = id
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got == "" {
		t.Fatal("expected a non-empty generated request id")
	}
	if respID := rr.Header().Get("X-Request-ID"); respID != got {
		t.Errorf("response header %q does not match context id %q", respID, got)
	}
}

func TestRequestID_ReusesIncoming(t *testing.T) {
	r := &Regius{}
	const incoming = "abc-123-correlation"
	handler := r.RequestID(RequestIDConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, ok := RequestIDFromContext(req.Context())
			if !ok || id != incoming {
				t.Errorf("expected incoming id %q to be reused, got %q (ok=%v)", incoming, id, ok)
			}
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", incoming)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Request-ID"); got != incoming {
		t.Errorf("expected response to echo incoming id %q, got %q", incoming, got)
	}
}

func TestRequestID_Formats(t *testing.T) {
	tests := []struct {
		name   string
		format string
		check  func(t *testing.T, id string)
	}{
		{
			"uuid_default",
			"",
			func(t *testing.T, id string) {
				if _, err := uuid.Parse(id); err != nil {
					t.Errorf("expected a valid uuid, got %q: %v", id, err)
				}
			},
		},
		{
			"uuid_explicit",
			RequestIDFormatUUID,
			func(t *testing.T, id string) {
				if _, err := uuid.Parse(id); err != nil {
					t.Errorf("expected a valid uuid, got %q: %v", id, err)
				}
			},
		},
		{
			"xid",
			RequestIDFormatXID,
			func(t *testing.T, id string) {
				if len(id) != 20 {
					t.Errorf("expected xid of length 20, got %d (%q)", len(id), id)
				}
			},
		},
		{
			"short",
			RequestIDFormatShort,
			func(t *testing.T, id string) {
				if len(id) != 12 {
					t.Errorf("expected short id of length 12, got %d (%q)", len(id), id)
				}
			},
		},
		{
			"default_chi_style",
			RequestIDFormatDefault,
			func(t *testing.T, id string) {
				if !strings.Contains(id, "/") || !strings.Contains(id, "-") {
					t.Errorf("expected chi-style id containing '/' and '-', got %q", id)
				}
			},
		},
	}

	for _, e := range tests {
		t.Run(e.name, func(t *testing.T) {
			r := &Regius{}
			var id string
			handler := r.RequestID(RequestIDConfig{Enabled: true, Format: e.format})(
				http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					id, _ = RequestIDFromContext(req.Context())
					w.WriteHeader(http.StatusOK)
				}),
			)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if id == "" {
				t.Fatal("expected a non-empty generated id")
			}
			e.check(t, id)
			if got := rr.Header().Get("X-Request-ID"); got != id {
				t.Errorf("response header %q does not match context id %q", got, id)
			}
		})
	}
}

func TestRequestID_CustomGenerator(t *testing.T) {
	r := &Regius{}
	const want = "tenant-42-xyz"
	handler := r.RequestID(RequestIDConfig{
		Enabled:   true,
		Generator: func() string { return want },
	})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id, _ := RequestIDFromContext(req.Context())
		if id != want {
			t.Errorf("expected generator-produced id %q, got %q", want, id)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Request-ID"); got != want {
		t.Errorf("expected response header %q, got %q", want, got)
	}
}

func TestRequestID_ResponseHeaderConfigurable(t *testing.T) {
	r := &Regius{}
	handler := r.RequestID(RequestIDConfig{
		Enabled:        true,
		Format:         RequestIDFormatUUID,
		ResponseHeader: "X-Correlation-ID",
	})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Correlation-ID"); got == "" {
		t.Error("expected a non-empty X-Correlation-ID response header")
	}
	if got := rr.Header().Get("X-Request-ID"); got != "" {
		t.Errorf("expected no X-Request-ID header when a custom response header is set, got %q", got)
	}
}

func TestRequestID_ResponseHeaderEmpty(t *testing.T) {
	r := &Regius{}
	handler := r.RequestID(RequestIDConfig{
		Enabled:        true,
		Format:         RequestIDFormatUUID,
		ResponseHeader: "-",
	})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// ResponseHeader of "-" is non-empty, so the id is echoed under that
	// literal header name; the default X-Request-ID header must not be set.
	if got := rr.Header().Get("X-Request-ID"); got != "" {
		t.Errorf("expected no default X-Request-ID header, got %q", got)
	}
}

func TestRequestID_DownstreamHandlerRuns(t *testing.T) {
	r := &Regius{}
	called := false
	handler := r.RequestID(RequestIDConfig{Enabled: true, Format: RequestIDFormatUUID})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			called = true
			w.WriteHeader(http.StatusTeapot)
		}),
	)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected downstream handler to be invoked")
	}
	if rr.Code != http.StatusTeapot {
		t.Errorf("expected status %d, got %d", http.StatusTeapot, rr.Code)
	}
}

func TestRequestID_DownstreamCanOverride(t *testing.T) {
	r := &Regius{}
	const override = "overridden-id"
	handler := r.RequestID(RequestIDConfig{Enabled: true, Format: RequestIDFormatUUID})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Request-ID", override)
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Request-ID"); got != override {
		t.Errorf("expected downstream override %q to win, got %q", override, got)
	}
}

func TestRequestID_InteroperableWithChiGetReqID(t *testing.T) {
	r := &Regius{}
	const incoming = "chi-compatible"
	handler := r.RequestID(RequestIDConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// chi's own accessor must see the same id.
			if got := chimw.GetReqID(req.Context()); got != incoming {
				t.Errorf("expected chimw.GetReqID to return %q, got %q", incoming, got)
			}
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", incoming)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRequestID_TruncatesOversizedIncoming(t *testing.T) {
	r := &Regius{}
	oversized := make([]byte, maxRequestIDLength+10)
	for i := range oversized {
		oversized[i] = 'a'
	}
	handler := r.RequestID(RequestIDConfig{Enabled: true, Format: RequestIDFormatUUID})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, _ := RequestIDFromContext(req.Context())
			if id == string(oversized) {
				t.Error("expected oversized incoming id to be discarded, not reused")
			}
			if _, err := uuid.Parse(id); err != nil {
				t.Errorf("expected a freshly generated uuid, got %q: %v", id, err)
			}
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", string(oversized))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}
