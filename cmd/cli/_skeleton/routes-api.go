package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) ApiRoutes() http.Handler {
	r := chi.NewRouter()

	r.Route("/api", func(_ chi.Router) {
		// add any API route here

		// example:
		// r.Get("/hello", func(w http.ResponseWriter, _ *http.Request) {
		// 	var payload struct {
		// 		Content string `json:"content"`
		// 	}
		//
		// 	payload.Content = "Hello World!"
		// 	a.App.WriteJSON(w, http.StatusOK, payload)
		// })
	})

	return r
}
