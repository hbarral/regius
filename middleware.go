package regius

import (
	"net/http"
	"strconv"

	"github.com/justinas/nosurf"
)

func (r *Regius) SessionLoad(next http.Handler) http.Handler {
	r.InfoLog.Println("SessionLoad called")
	return r.Session.LoadAndSave(next)
}

func (r *Regius) NoSurf(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	secure, _ := strconv.ParseBool(r.config.cookie.secure)

	// csrfHandler.ExemptGlob("/someapi/*")

	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Domain:   r.config.cookie.domain,
	})

	return csrfHandler
}
