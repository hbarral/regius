package regius

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
