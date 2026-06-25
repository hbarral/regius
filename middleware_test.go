package regius

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
