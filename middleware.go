package regius

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/justinas/nosurf"
)

func (r *Regius) SessionLoad(next http.Handler) http.Handler {
	r.InfoLog.Println("SessionLoad called")
	return r.Session.LoadAndSave(next)
}

func (r *Regius) NoSurf(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	secure, _ := strconv.ParseBool(r.config.cookie.secure)

	csrfHandler.ExemptRegexp("/api/.*")

	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Domain:   r.config.cookie.domain,
	})

	return csrfHandler
}

func (r *Regius) MaxRequestSize(maxBytes int64) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			if r.ContentLength > maxBytes {
				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func (r *Regius) CheckForMaintenanceMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if maintenanceMode {
			if !strings.Contains(req.URL.Path, "/public/maintenance.html") {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Header().Set("Retry-After", "300")
				w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0, post-check=0, pre-check=0")
				http.ServeFile(w, req, fmt.Sprintf("%s/public/maintenance.html", r.RootPath))
				return
			}
		}
		next.ServeHTTP(w, req)
	})
}

// Rate limiter is available as r.RateLimiter(config) - see ratelimiter.go for details
// API key auth is available as r.APIKeyAuth(config) - see apikeyauth.go for details

// clientIPAddress extracts the client IP from the request. When trustProxy is
// true, it reads the first IP from the X-Forwarded-For header (or X-Real-IP)
// — only enable this behind a trusted reverse proxy. Otherwise it uses
// net.SplitHostPort on RemoteAddr, which correctly handles IPv6 addresses of
// the form [::1]:1234 (returning ::1, not the bracketed form).
func clientIPAddress(req *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For may be a comma-separated list; the first entry
			// is the original client.
			if idx := strings.IndexByte(xff, ','); idx >= 0 {
				xff = xff[:idx]
			}
			return strings.TrimSpace(xff)
		}
		if xri := req.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
