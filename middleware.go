package regius

import (
	"fmt"
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

	csrfHandler.ExemptGlob("/api/*")

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
