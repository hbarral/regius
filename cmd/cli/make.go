package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"
)

func doMake(arg2, arg3 string) error {
	switch arg2 {
	case "migration":
		dbType := reg.DB.DataType
		if arg3 == "" {
			exitGracefully(errors.New("you must give the migration a name"))
		}

		fileName := fmt.Sprintf("%d_%s", time.Now().UnixMicro(), arg3)

		upFile := reg.RootPath + "/migrations/" + fileName + "." + dbType + ".up.sql"
		downFile := reg.RootPath + "/migrations/" + fileName + "." + dbType + ".down.sql"

		err := copyFileFromTemplate("templates/migrations/migration."+dbType+".up.sql", upFile)
		if err != nil {
			exitGracefully(err)
		}

		err = copyFileFromTemplate("templates/migrations/migration."+dbType+".down.sql", downFile)
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

		err = ioutil.WriteFile(fileName, []byte(handler), 0644)
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

	}

	return nil
}
