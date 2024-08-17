package main

import (
	"errors"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"
)

func doMake(arg2, arg3, arg4 string) error {
	switch arg2 {
	case "migration":
		checkForDB()

		if arg3 == "" {
			exitGracefully(errors.New("you must give the migration a name"))
		}

		migrationType := "fizz"
		var up, down string

		if arg4 == "fizz" || arg4 == "" {
			upBytes, _ := templateFS.ReadFile("templates/migrations/migration_up.fizz")
			downBytes, _ := templateFS.ReadFile("templates/migrations/migration_down.fizz")

			up = string(upBytes)
			down = string(downBytes)
		} else {
			migrationType = "sql"
		}

		err := reg.CreatePopMigration([]byte(up), []byte(down), arg3, migrationType)
		if err != nil {
			exitGracefully(err)
		}

	case "auth":
		err := doAuth()
		if err != nil {
			exitGracefully(err)
		}

	case "handler":
		if arg3 == "" {
			exitGracefully(errors.New("you must give the handler a name"))
		}

		fileName := reg.RootPath + "/handlers/" + strings.ToLower(arg3) + ".go"
		if fileExists(fileName) {
			exitGracefully(errors.New(fileName + " already exists!"))
		}

		data, err := templateFS.ReadFile("templates/handlers/handler")
		if err != nil {
			exitGracefully(err)
		}

		handler := string(data)
		handler = strings.ReplaceAll(handler, "$HANDLER_NAME", strcase.ToCamel(arg3))

		err = os.WriteFile(fileName, []byte(handler), 0o644)
		if err != nil {
			exitGracefully(err)
		}

	case "model":
		if arg3 == "" {
			exitGracefully(errors.New("you must give the model a name"))
		}

		data, err := templateFS.ReadFile("templates/data/model")
		if err != nil {
			exitGracefully(err)
		}

		model := string(data)
		plur := pluralize.NewClient()

		modelName := arg3
		tableName := arg3

		if plur.IsPlural(arg3) {
			modelName = plur.Singular(arg3)
			tableName = strings.ToLower(tableName)
		} else {
			tableName = strings.ToLower(plur.Plural(arg3))
		}

		fileName := reg.RootPath + "/data/" + strings.ToLower(modelName) + ".go"
		if fileExists(fileName) {
			exitGracefully(errors.New(fileName + " already exists!"))
		}

		model = strings.ReplaceAll(model, "$MODEL_NAME", strcase.ToCamel(modelName))
		model = strings.ReplaceAll(model, "$TABLE_NAME", tableName)

		err = copyDataToFile([]byte(model), fileName)
		if err != nil {
			exitGracefully(err)
		}

	case "session":
		err := doSessionTable()
		if err != nil {
			exitGracefully(err)
		}

	case "key":
		rnd := reg.RandomString(32)
		color.Yellow("32 character encryption key: %s", rnd)

	case "mail":
		if arg3 == "" {
			exitGracefully(errors.New("you must give the mail template a name"))
		}

		htmlMail := reg.RootPath + "/mail/" + strings.ToLower(arg3) + ".html.tmpl"
		plainMail := reg.RootPath + "/mail/" + strings.ToLower(arg3) + ".plain.tmpl"

		err := copyFileFromTemplate("templates/mailer/mail.html.tmpl", htmlMail)
		if err != nil {
			exitGracefully(err)
		}

		err = copyFileFromTemplate("templates/mailer/mail.plain.tmpl", plainMail)
		if err != nil {
			exitGracefully(err)
		}

	}

	return nil
}
