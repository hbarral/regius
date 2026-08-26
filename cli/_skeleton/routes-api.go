package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) ApiRoutes() http.Handler {
	r := chi.NewRouter()

	r.Route("/", func(r chi.Router) {
		// add any API route here
		//
		// To enable the Scalar API reference UI:
		// 1. Set SCALAR_ENABLED=true in .env
		// 2. Build an OpenAPI document and assign it:
		//    a.App.Scalar.Spec = api.NewDocument("My API", "1.0.0")
		//    (or set SCALAR_SPEC_FILE=openapi.yaml for a static spec)
		// 3. Visit /docs

		// example:
		// r.Get("/hello", func(w http.ResponseWriter, _ *http.Request) {
		// 	var payload struct {
		// 		Content string `json:"content"`
		// 	}
		//
		// 	payload.Content = "Hello World!"
		// 	a.App.WriteAPIResponse(w, http.StatusOK, payload)
		// })
	})

	return r
}
