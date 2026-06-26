package regius

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxRequestSize_UnderLimit(t *testing.T) {
	r := &Regius{}

	called := false
	handler := r.MaxRequestSize(100)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 50)))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, called, "downstream handler must run when body is under the limit")
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestMaxRequestSize_ExactLimit(t *testing.T) {
	r := &Regius{}

	called := false
	handler := r.MaxRequestSize(100)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 100)))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, called, "a body equal to the limit is allowed (strictly greater is rejected)")
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestMaxRequestSize_OverLimit(t *testing.T) {
	r := &Regius{}

	called := false
	handler := r.MaxRequestSize(100)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 200)))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.False(t, called, "downstream handler must not run when body exceeds the limit")
	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	assert.Contains(t, rr.Body.String(), "Request body too large")
}

func TestClientIPAddress(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xRealIP    string
		trustProxy bool
		want       string
	}{
		{"plain remote addr", "1.2.3.4:5678", "", "", false, "1.2.3.4"},
		{"trust proxy xff", "1.2.3.4:5678", "9.9.9.9", "", true, "9.9.9.9"},
		{"xff list takes first", "1.2.3.4:5678", "9.9.9.9, 8.8.8.8", "", true, "9.9.9.9"},
		{"trust proxy xrealip", "1.2.3.4:5678", "", "7.7.7.7", true, "7.7.7.7"},
		{"xff preferred over xrealip", "1.2.3.4:5678", "9.9.9.9", "7.7.7.7", true, "9.9.9.9"},
		{"ignore headers when not trusting proxy", "1.2.3.4:5678", "9.9.9.9", "7.7.7.7", false, "1.2.3.4"},
		{"ipv6 remote addr", "[::1]:1234", "", "", false, "::1"},
		{"trust proxy but no headers falls back to remote addr", "5.5.5.5:1", "", "", true, "5.5.5.5"},
		{"empty remote addr returns as-is", "", "", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			got := clientIPAddress(req, tt.trustProxy)
			assert.Equal(t, tt.want, got)
		})
	}
}

// withMaintenanceMode sets the package-global and restores it on cleanup.
func withMaintenanceMode(t *testing.T, on bool) {
	t.Helper()
	original := maintenanceMode
	maintenanceMode = on
	t.Cleanup(func() { maintenanceMode = original })
}

func TestCheckForMaintenanceMode_Off(t *testing.T) {
	withMaintenanceMode(t, false)

	r := &Regius{RootPath: t.TempDir()}
	called := false
	handler := r.CheckForMaintenanceMode(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestCheckForMaintenanceMode_On_ServesMaintenancePage(t *testing.T) {
	withMaintenanceMode(t, true)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "public"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "public", "maintenance.html"), []byte("<h1>Maintenance</h1>"), 0644))

	r := &Regius{RootPath: root}
	called := false
	handler := r.CheckForMaintenanceMode(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.False(t, called, "downstream must not run in maintenance mode")
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Equal(t, "300", rr.Header().Get("Retry-After"))
	assert.Contains(t, rr.Header().Get("Cache-Control"), "no-store")
	assert.Contains(t, rr.Body.String(), "Maintenance")
}

func TestCheckForMaintenanceMode_ExemptsMaintenanceAsset(t *testing.T) {
	withMaintenanceMode(t, true)

	r := &Regius{RootPath: t.TempDir()}
	called := false
	handler := r.CheckForMaintenanceMode(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/public/maintenance.html", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, called, "the maintenance asset path must bypass the block")
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestNoSurf_ReturnsHandler(t *testing.T) {
	r := &Regius{}
	r.config.cookie = cookieConfig{secure: "false", domain: ""}

	handler := r.NoSurf(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	assert.NotNil(t, handler)
}

func TestNoSurf_GetRequestAllowed(t *testing.T) {
	r := &Regius{}
	r.config.cookie = cookieConfig{secure: "false", domain: ""}

	called := false
	handler := r.NoSurf(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestNoSurf_PostWithoutTokenForbidden(t *testing.T) {
	r := &Regius{}
	r.config.cookie = cookieConfig{secure: "false", domain: ""}

	called := false
	handler := r.NoSurf(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.False(t, called, "POST without CSRF token must be rejected")
	assert.Equal(t, http.StatusBadRequest, rr.Code, "nosurf default FailureCode is 400")
}

func TestNoSurf_ExemptAPIPath(t *testing.T) {
	r := &Regius{}
	r.config.cookie = cookieConfig{secure: "false", domain: ""}

	called := false
	handler := r.NoSurf(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, called, "exempted /api/.* paths must bypass CSRF")
	assert.Equal(t, http.StatusOK, rr.Code)
}
