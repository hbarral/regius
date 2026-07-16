package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) routes() *chi.Mux {
	// middlewares
	// a.use(a.Middleware.CheckRemember)

	// routes
	a.get("/", a.Handlers.Home)

	// static routes
	fileServer := http.FileServer(http.Dir("./public"))
	a.App.Routes.Handle("/public/*", http.StripPrefix("/public", fileServer))

	// api
	a.App.Routes.Mount("/api", a.ApiRoutes())

	return a.App.Routes
}
