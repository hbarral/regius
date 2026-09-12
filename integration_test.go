package regius

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"
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
		"APP_NAME":     "TestApp",
		"APP_URL":      "http://localhost:4000",
		"DEBUG":        "true",
		"PORT":         "4000",
		"SERVER_NAME":  "localhost",
		"SECURE":       "false",
		"MAX_FILESIZE": "10485760",
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

	ts := httptest.NewServer(r.Handler())
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

	ts := httptest.NewServer(r.Handler())
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

	ts := httptest.NewServer(r.Handler())
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

	ts := httptest.NewServer(r.Handler())
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

	ts := httptest.NewServer(r.Handler())
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

	ts := httptest.NewServer(r.Handler())
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

	ts := httptest.NewServer(r.Handler())
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

	ts := httptest.NewServer(r.Handler())
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

	ts := httptest.NewServer(r.Handler())
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

func TestIntegration_SSE(t *testing.T) {
	r := newTestApp(t, nil)

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/sse/stream", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	go func() {
		time.Sleep(50 * time.Millisecond)
		r.SSE.Broadcast(SSEEvent{
			Event: "test",
			Data:  []byte(`{"ok":true}`),
		})
	}()

	buf := make([]byte, 256)
	n, err := resp.Body.Read(buf)
	require.NoError(t, err)

	body := string(buf[:n])
	assert.Contains(t, body, "event: test")
	assert.Contains(t, body, "data: {\"ok\":true}")
}

func TestIntegration_CSRFProtection(t *testing.T) {
	r := newTestApp(t, nil)

	r.Routes.Get("/form", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<form method="post" action="/form"><input type="hidden" name="csrf_token" value="%s"></form>`, nosurf.Token(req))
	})
	r.Routes.Post("/form", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar}

	resp, err := client.Get(ts.URL + "/form")
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	re := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)
	matches := re.FindStringSubmatch(string(body))
	require.Len(t, matches, 2, "csrf token not found in form")

	form := strings.NewReader("csrf_token=" + url.QueryEscape(matches[1]))
	resp, err = client.Post(ts.URL+"/form", "application/x-www-form-urlencoded", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Post(ts.URL+"/form", "application/x-www-form-urlencoded", strings.NewReader(""))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestIntegration_AuthFlow(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"DATABASE_TYPE": "sqlite",
		"DATABASE_NAME": ":memory:",
		"HASH_COST":     "4",
	})

	_, err := r.DB.Pool.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE,
			password TEXT,
			first_name TEXT,
			last_name TEXT,
			user_active INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)
	`)
	require.NoError(t, err)

	r.Routes.Get("/auth/signup", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<form method="post" action="/auth/signup"><input type="hidden" name="csrf_token" value="%s"><input name="email"><input name="password"></form>`, nosurf.Token(req))
	})
	r.Routes.Post("/auth/signup", func(w http.ResponseWriter, req *http.Request) {
		email := req.FormValue("email")
		password := req.FormValue("password")

		hashed, hashErr := r.Hash.Generate(password)
		if hashErr != nil {
			http.Error(w, hashErr.Error(), http.StatusInternalServerError)
			return
		}

		_, hashErr = r.DB.Pool.Exec(
			"INSERT INTO users (email, password, first_name, last_name, user_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			email, hashed, "Test", "User", 1, time.Now(), time.Now(),
		)
		if hashErr != nil {
			http.Error(w, hashErr.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, req, "/auth/signin", http.StatusSeeOther)
	})
	r.Routes.Get("/auth/signin", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<form method="post" action="/auth/signin"><input type="hidden" name="csrf_token" value="%s"><input name="email"><input name="password"></form>`, nosurf.Token(req))
	})
	r.Routes.Post("/auth/signin", func(w http.ResponseWriter, req *http.Request) {
		email := req.FormValue("email")
		password := req.FormValue("password")

		var hashed string
		err := r.DB.Pool.QueryRow("SELECT password FROM users WHERE email = ?", email).Scan(&hashed)
		if err != nil {
			http.Redirect(w, req, "/auth/signin", http.StatusSeeOther)
			return
		}

		matches, err := r.Hash.Compare(hashed, password)
		if err != nil || !matches {
			http.Redirect(w, req, "/auth/signin", http.StatusSeeOther)
			return
		}

		r.Session.Put(req.Context(), "userID", 1)
		http.Redirect(w, req, "/", http.StatusSeeOther)
	})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	csrfToken := func(path string) string {
		resp, getErr := client.Get(ts.URL + path)
		require.NoError(t, getErr)
		body, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)
		resp.Body.Close()

		re := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)
		matches := re.FindStringSubmatch(string(body))
		require.Len(t, matches, 2, "csrf token not found in %s", path)
		return matches[1]
	}

	signupValues := url.Values{}
	signupValues.Set("csrf_token", csrfToken("/auth/signup"))
	signupValues.Set("email", "alice@example.com")
	signupValues.Set("password", "secret123")
	resp, err := client.Post(ts.URL+"/auth/signup", "application/x-www-form-urlencoded", strings.NewReader(signupValues.Encode()))
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/auth/signin", resp.Header.Get("Location"))
	resp.Body.Close()

	signinValues := url.Values{}
	signinValues.Set("csrf_token", csrfToken("/auth/signin"))
	signinValues.Set("email", "alice@example.com")
	signinValues.Set("password", "secret123")
	resp, err = client.Post(ts.URL+"/auth/signin", "application/x-www-form-urlencoded", strings.NewReader(signinValues.Encode()))
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("Location"))
	resp.Body.Close()
}
