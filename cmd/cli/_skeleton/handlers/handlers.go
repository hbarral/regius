package handlers

import (
	"net/http"
	"time"

	"regius-app/data"

	"github.com/hbarral/regius"
)

type Handlers struct {
	App    *regius.Regius
	Models data.Models
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	defer h.App.LoadTime(time.Now())
	err := h.App.Render.Page(w, r, h.App.Render.Jet("home", nil), nil)
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}
