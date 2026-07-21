package main

import (
	"log"
	"os"

	"github.com/hbarral/regius"

	"regius-app/data"
	"regius-app/handlers"
	"regius-app/middleware"
)

func initApplication() *application {
	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	reg := &regius.Regius{}
	err = reg.New(path)

	if err != nil {
		log.Fatal(err)
	}

	reg.AppName = "regius-app"

	myMiddleware := &middleware.Middleware{
		App: reg,
	}

	myHandlers := &handlers.Handlers{
		App: reg,
	}

	app := &application{
		App:        reg,
		Handlers:   myHandlers,
		Middleware: myMiddleware,
	}

	app.App.Routes = app.routes()

	app.Models = data.New(app.App.DB.Pool)
	myHandlers.Models = app.Models
	app.Middleware.Models = app.Models

	return app
}
