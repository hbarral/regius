package regius

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const xssPayload = `<script>alert('xss')</script>hello`

func newSanitizeRegius() *Regius {
	return &Regius{}
}

func TestRequestSanitizer_Disabled(t *testing.T) {
	r := newSanitizeRegius()
	handler := r.RequestSanitizer(RequestSanitizerConfig{Enabled: false})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Query must be untouched when disabled.
			if got := req.URL.Query().Get("q"); got != xssPayload {
				t.Errorf("expected query untouched when disabled, got %q", got)
			}
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest("GET", "/?q="+xssPayload, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequestSanitizer_Query(t *testing.T) {
	r := newSanitizeRegius()
	handler := r.RequestSanitizer(RequestSanitizerConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if got := req.URL.Query().Get("q"); got != "hello" {
				t.Errorf("expected sanitized query %q, got %q", "hello", got)
			}
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest("GET", "/?q="+xssPayload, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequestSanitizer_QueryCleanUnchanged(t *testing.T) {
	r := newSanitizeRegius()
	original := "name=hello&n=2"
	handler := r.RequestSanitizer(RequestSanitizerConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// A clean query must be left byte-for-byte intact (no reordering).
			if req.URL.RawQuery != original {
				t.Errorf("expected clean query %q, got %q", original, req.URL.RawQuery)
			}
		}),
	)

	req := httptest.NewRequest("GET", "/?"+original, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRequestSanitizer_Form(t *testing.T) {
	r := newSanitizeRegius()
	handler := r.RequestSanitizer(RequestSanitizerConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if got := req.Form.Get("q"); got != "hello" {
				t.Errorf("expected sanitized form value %q, got %q", "hello", got)
			}
		}),
	)

	body := "q=" + xssPayload
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRequestSanitizer_JSONBodyUntouched(t *testing.T) {
	r := newSanitizeRegius()
	jsonBody := `{"q":"` + xssPayload + `"}`
	handler := r.RequestSanitizer(RequestSanitizerConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			got, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if string(got) != jsonBody {
				t.Errorf("JSON body must be untouched, got %q", got)
			}
		}),
	)

	req := httptest.NewRequest("POST", "/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRequestSanitizer_Multipart(t *testing.T) {
	r := newSanitizeRegius()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("comment", xssPayload)
	fw, _ := mw.CreateFormFile("upload", "x.txt")
	_, _ = fw.Write([]byte("file-contents"))
	_ = mw.Close()

	handler := r.RequestSanitizer(RequestSanitizerConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if err := req.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			if got := req.FormValue("comment"); got != "hello" {
				t.Errorf("expected sanitized multipart field %q, got %q", "hello", got)
			}
			if _, ok := req.MultipartForm.File["upload"]; !ok {
				t.Error("expected file upload to be preserved")
			}
		}),
	)

	req := httptest.NewRequest("POST", "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRequestSanitizer_Headers(t *testing.T) {
	r := newSanitizeRegius()
	handler := r.RequestSanitizer(RequestSanitizerConfig{
		Enabled: true,
		Query:   BoolPtr(false),
		Form:    BoolPtr(false),
		Headers: []string{"Referer"},
	})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Referer"); got != "hello" {
			t.Errorf("expected sanitized Referer %q, got %q", "hello", got)
		}
		// Headers not in the allowlist must be left intact.
		if got := req.Header.Get("Authorization"); got != "Bearer "+xssPayload {
			t.Errorf("Authorization must be untouched, got %q", got)
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Referer", xssPayload)
	req.Header.Set("Authorization", "Bearer "+xssPayload)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRequestSanitizer_Exempt(t *testing.T) {
	r := newSanitizeRegius()
	// Empty Exempt falls back to the default "/api/.*".
	handler := r.RequestSanitizer(RequestSanitizerConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if got := req.URL.Query().Get("q"); got != xssPayload {
				t.Errorf("exempt /api path must not be sanitized, got %q", got)
			}
		}),
	)

	req := httptest.NewRequest("GET", "/api/foo?q="+xssPayload, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRequestSanitizer_PolicyUGC(t *testing.T) {
	r := newSanitizeRegius()
	handler := r.RequestSanitizer(RequestSanitizerConfig{
		Enabled: true,
		Policy:  SanitizePolicyUGC,
	})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.URL.Query().Get("q"); got != "<b>bold</b>" {
			t.Errorf("ugc policy should keep <b> and drop <script>, got %q", got)
		}
	}))

	req := httptest.NewRequest("GET", "/?q=<b>bold</b><script>alert(1)</script>", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestRequestSanitizer_DownstreamRuns(t *testing.T) {
	r := newSanitizeRegius()
	called := false
	handler := r.RequestSanitizer(RequestSanitizerConfig{Enabled: true})(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			called = true
			w.WriteHeader(http.StatusTeapot)
		}),
	)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected downstream handler to be invoked")
	}
	if rr.Code != http.StatusTeapot {
		t.Errorf("expected status %d, got %d", http.StatusTeapot, rr.Code)
	}
}

func TestSanitize_Helper(t *testing.T) {
	r := &Regius{}
	if got := r.Sanitize(xssPayload); got != "hello" {
		t.Errorf("r.Sanitize: expected %q, got %q", "hello", got)
	}
	// Package-level helper uses the strict policy.
	if got := Sanitize(xssPayload); got != "hello" {
		t.Errorf("Sanitize: expected %q, got %q", "hello", got)
	}
	// Plain text passes through.
	if got := Sanitize("plain text"); got != "plain text" {
		t.Errorf("Sanitize plain: expected %q, got %q", "plain text", got)
	}
}
