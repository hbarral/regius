package handlers

import (
	"net/http"
	"time"

	"github.com/hbarral/regius"

	"regius-app/data"
)

type Handlers struct {
	App    *regius.Regius
	Models data.Models
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	defer h.App.LoadTime(time.Now())
	err := h.render(w, r, "home", nil, nil)
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}
