package regius

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestApp creates a fully initialized Regius app rooted in a temp directory,
// suitable for integration tests that exercise the real middleware stack.
func newTestApp(t *testing.T, envOverrides map[string]string) *Regius {
	t.Helper()

	root := t.TempDir()

	r := &Regius{}
	for _, dir := range []string{
		"handlers", "migrations", "views", "mail", "data",
		"public", "tmp", "logs", "middleware", "screenshots",
	} {
		require.NoError(t, r.CreateDirIfNotExist(filepath.Join(root, dir)))
	}

	// godotenv.Load requires a .env file to exist.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte{}, 0644))
	// Maintenance mode serves this file.
	require.NoError(t, os.WriteFile(filepath.Join(root, "public", "maintenance.html"), []byte("<html>maintenance</html>"), 0644))

	baseEnv := map[string]string{
		"APP_NAME":    "TestApp",
		"APP_URL":     "http://localhost:4000",
		"DEBUG":       "true",
		"PORT":        "4000",
		"SERVER_NAME": "localhost",
		"SECURE":      "false",
	}
	for k, v := range baseEnv {
		t.Setenv(k, v)
	}
	for k, v := range envOverrides {
		t.Setenv(k, v)
	}

	require.NoError(t, r.New(root))
	return r
}

func TestIntegration_RequestID(t *testing.T) {
	r := newTestApp(t, nil)
	r.Routes.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))
}

func TestIntegration_SecurityHeaders(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"SECURITY_HEADERS_ENABLED": "true",
	})
	r.Routes.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.NotEmpty(t, resp.Header.Get("X-Frame-Options"))
	assert.NotEmpty(t, resp.Header.Get("Referrer-Policy"))
}

func TestIntegration_MaintenanceMode(t *testing.T) {
	original := maintenanceMode
	maintenanceMode = false
	t.Cleanup(func() { maintenanceMode = original })

	r := newTestApp(t, nil)
	r.Routes.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	maintenanceMode = true

	resp, err = http.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "300", resp.Header.Get("Retry-After"))
}

func TestIntegration_IPFilter_Allow(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"IP_FILTER_ENABLED": "true",
		"IP_FILTER_ALLOW":   "127.0.0.1/32",
	})
	r.Routes.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestIntegration_IPFilter_Deny(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"IP_FILTER_ENABLED": "true",
		"IP_FILTER_DENY":    "127.0.0.1/32",
	})
	r.Routes.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestIntegration_RequestSanitizer(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"REQUEST_SANITIZATION_ENABLED": "true",
	})

	var sanitized string
	r.Routes.Get("/", func(w http.ResponseWriter, req *http.Request) {
		sanitized = req.URL.Query().Get("input")
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/?input=<script>alert(1)</script>hello")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "hello", sanitized)
}

func TestIntegration_CORS(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"CORS_ENABLED": "true",
	})
	r.Routes.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Methods"))
}

func TestIntegration_RateLimiter(t *testing.T) {
	r := newTestApp(t, nil)
	r.Routes.With(r.RateLimiter(RateLimiterConfig{
		Enabled:   true,
		Algorithm: RateLimiterAlgorithmTokenBucket,
		Requests:  1,
		Window:    time.Minute,
		Storage:   "memory",
	})).Get("/api", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "1", resp.Header.Get("X-RateLimit-Limit"))
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, "1", resp.Header.Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
}

func TestIntegration_APIKeyAuth(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"API_KEY_AUTH_ENABLED": "true",
		"API_KEYS":             "secret-key",
	})

	r.Routes.Route("/api", func(api chi.Router) {
		api.Use(r.APIKeyAuth(r.APIKeyAuthCfg()))
		api.Get("/", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	ts := httptest.NewServer(r.Routes)
	defer ts.Close()

	// Missing key.
	resp, err := http.Get(ts.URL + "/api/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.True(t, strings.Contains(resp.Header.Get("WWW-Authenticate"), "Bearer"))
	resp.Body.Close()

	// Invalid key.
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "wrong-key")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Valid key via X-API-Key.
	req, err = http.NewRequest(http.MethodGet, ts.URL+"/api/", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "secret-key")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
