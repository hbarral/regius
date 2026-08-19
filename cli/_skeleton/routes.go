package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/templui/templui/utils"
)

func (a *application) routes() *chi.Mux {
	// middlewares
	// a.use(a.Middleware.CheckRemember)

	// routes
	a.get("/", a.Handlers.Home)
	a.get("/set-language/{lang}", a.Handlers.SetLanguage)
	a.get("/sse/ping", a.Handlers.SSEPing)

	// static routes
	fileServer := http.FileServer(http.Dir("./public"))
	a.App.Routes.Handle("/public/*", http.StripPrefix("/public", fileServer))

	// templui component scripts
	templuiMux := http.NewServeMux()
	utils.SetupScriptRoutes(templuiMux, a.App.Debug)
	a.App.Routes.Handle("/templui/js/*", templuiMux)

	// api
	a.App.Routes.Mount("/api", a.ApiRoutes())

	return a.App.Routes
}
