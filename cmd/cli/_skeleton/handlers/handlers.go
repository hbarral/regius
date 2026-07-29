package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hbarral/regius"

	"regius-app/data"
	"regius-app/views"
)

type Handlers struct {
	App    *regius.Regius
	Models data.Models
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	defer h.App.LoadTime(time.Now())
	err := h.App.Render.Page(w, r, views.Home(), nil)
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}

func (h *Handlers) SetLanguage(w http.ResponseWriter, r *http.Request) {
	lang := chi.URLParam(r, "lang")
	if !h.App.I18n.IsSupported(lang) {
		lang = h.App.I18n.DefaultLocale
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.App.I18n.CookieName,
		Value:    lang,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.App.Server.Secure,
	})

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
