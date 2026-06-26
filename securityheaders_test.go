package regius

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_Disabled(t *testing.T) {
	r := &Regius{}
	handler := r.SecurityHeaders(SecurityHeadersConfig{Enabled: false})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	securityHeaderKeys := []string{
		"Content-Security-Policy",
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
	}
	for _, h := range securityHeaderKeys {
		if got := rr.Header().Get(h); got != "" {
			t.Errorf("expected no %s header when disabled, got %q", h, got)
		}
	}
}

func TestSecurityHeaders_Defaults(t *testing.T) {
	r := &Regius{}
	handler := r.SecurityHeaders(SecurityHeadersConfig{Enabled: true})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	cases := []struct {
		header string
		want   string
	}{
		{"Content-Security-Policy", "default-src 'self'"},
		{"X-Content-Type-Options", "nosniff"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"X-Frame-Options", "SAMEORIGIN"},
		{"X-Permitted-Cross-Domain-Policies", "none"},
		{"Cross-Origin-Opener-Policy", "same-origin"},
		{"Cross-Origin-Resource-Policy", "same-origin"},
		{"X-DNS-Prefetch-Control", "off"},
	}
	for _, c := range cases {
		if got := rr.Header().Get(c.header); got != c.want {
			t.Errorf("expected %s to be %q, got %q", c.header, c.want, got)
		}
	}

	// HSTS must not be emitted when the server is not secure.
	if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("expected no Strict-Transport-Security when not secure, got %q", got)
	}
}

func TestSecurityHeaders_HSTS_OnlyWhenSecure(t *testing.T) {
	tests := []struct {
		name        string
		secure      bool
		wantHSTS    bool
		wantHSTSVal string
	}{
		{"secure_off_no_hsts", false, false, ""},
		{"secure_on_emits_hsts", true, true, "max-age=31536000"},
	}

	for _, e := range tests {
		t.Run(e.name, func(t *testing.T) {
			r := &Regius{Server: Server{Secure: e.secure}}
			handler := r.SecurityHeaders(SecurityHeadersConfig{
				Enabled: true,
			})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			got := rr.Header().Get("Strict-Transport-Security")
			if e.wantHSTS {
				if got != e.wantHSTSVal {
					t.Errorf("expected Strict-Transport-Security %q, got %q", e.wantHSTSVal, got)
				}
			} else {
				if got != "" {
					t.Errorf("expected no Strict-Transport-Security, got %q", got)
				}
			}
		})
	}
}

func TestSecurityHeaders_HSTSDirectives(t *testing.T) {
	r := &Regius{Server: Server{Secure: true}}
	handler := r.SecurityHeaders(SecurityHeadersConfig{
		Enabled:               true,
		HSTSMaxAge:            86400,
		HSTSIncludeSubDomains: true,
		HSTSPreload:           true,
	})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	want := "max-age=86400; includeSubDomains; preload"
	if got := rr.Header().Get("Strict-Transport-Security"); got != want {
		t.Errorf("expected Strict-Transport-Security %q, got %q", want, got)
	}
}

func TestSecurityHeaders_Overrides(t *testing.T) {
	r := &Regius{Server: Server{Secure: true}}
	cfg := SecurityHeadersConfig{
		Enabled:                       true,
		ContentSecurityPolicy:         "default-src 'self'; script-src 'self' cdn.example.com",
		ReferrerPolicy:                "no-referrer",
		XFrameOptions:                 "DENY",
		XPermittedCrossDomainPolicies: "master-only",
		CrossOriginOpenerPolicy:       "unsafe-none",
		CrossOriginResourcePolicy:     "cross-origin",
		XDNSPrefetchControl:           "on",
		HSTSMaxAge:                    3600,
	}
	handler := r.SecurityHeaders(cfg)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	cases := []struct {
		header string
		want   string
	}{
		{"Content-Security-Policy", "default-src 'self'; script-src 'self' cdn.example.com"},
		{"Referrer-Policy", "no-referrer"},
		{"X-Frame-Options", "DENY"},
		{"X-Permitted-Cross-Domain-Policies", "master-only"},
		{"Cross-Origin-Opener-Policy", "unsafe-none"},
		{"Cross-Origin-Resource-Policy", "cross-origin"},
		{"X-DNS-Prefetch-Control", "on"},
		{"Strict-Transport-Security", "max-age=3600"},
	}
	for _, c := range cases {
		if got := rr.Header().Get(c.header); got != c.want {
			t.Errorf("expected %s to be %q, got %q", c.header, c.want, got)
		}
	}
}

func TestSecurityHeaders_DownstreamHandlerRuns(t *testing.T) {
	r := &Regius{}
	called := false
	handler := r.SecurityHeaders(SecurityHeadersConfig{Enabled: true})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

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

func TestSecurityHeaders_DownstreamCanOverride(t *testing.T) {
	r := &Regius{}
	handler := r.SecurityHeaders(SecurityHeadersConfig{Enabled: true})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Content-Security-Policy"); got != "default-src 'none'" {
		t.Errorf("expected downstream CSP override to win, got %q", got)
	}
}
