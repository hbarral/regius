package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fatih/color"
)

func doAuth() error {
	checkForDB()
	appName := os.Getenv("APP_NAME")
	appName = strings.ToLower(appName)
	log.Println("APP NAME IS:", appName)
	dbType := reg.DB.DataType

	tx, err := reg.PopConnect()
	if err != nil {
		exitGracefully(err)
	}
	defer tx.Close()

	upBytes, err := templateFS.ReadFile(fmt.Sprintf("templates/migrations/auth_tables.%s.sql", dbType))
	if err != nil {
		exitGracefully(err)
	}

	downBytes := []byte(
		"DROP TABLE IF EXISTS users CASCADE; DROP TABLE IF EXISTS tokens CASCADE; DROP TABLE IF EXISTS remember_tokens;",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = reg.CreatePopMigration(upBytes, downBytes, "auth", "sql")
	if err != nil {
		exitGracefully(err)
	}

	err = reg.RunPopMigrations(tx)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/data/user", reg.RootPath+"/data/user.go")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/data/token", reg.RootPath+"/data/token.go")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/data/remember_token",
		reg.RootPath+"/data/remember_token.go",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/middleware/auth", reg.RootPath+"/middleware/auth.go")
	if err != nil {
		exitGracefully(err)
	}

	data, err := templateFS.ReadFile("templates/middleware/remember")
	if err != nil {
		exitGracefully(err)
	}

	rememberTokenFileContent := string(data)
	rememberTokenFileContent = strings.ReplaceAll(rememberTokenFileContent, "${APP_NAME}", appName)

	err = copyDataToFile([]byte(rememberTokenFileContent), "./middleware/remember.go")
	if err != nil {
		exitGracefully(err)
	}

	data, err = templateFS.ReadFile("templates/handlers/auth-handlers")
	if err != nil {
		exitGracefully(err)
	}

	authHandlerFileContent := string(data)
	authHandlerFileContent = strings.ReplaceAll(authHandlerFileContent, "${APP_NAME}", appName)

	err = copyDataToFile([]byte(authHandlerFileContent), "./handlers/auth-handlers.go")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/middleware/auth-token",
		reg.RootPath+"/middleware/auth-token.go",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/mailer/password-reset.html.template",
		reg.RootPath+"/mail/password-reset.html.template",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/mailer/password-reset.plain.template",
		reg.RootPath+"/mail/password-reset.plain.template",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/views/login.jet", reg.RootPath+"/views/login.jet")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/views/forgot.jet", reg.RootPath+"/views/forgot.jet")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/views/reset-password.jet",
		reg.RootPath+"/views/reset-password.jet",
	)
	if err != nil {
		exitGracefully(err)
	}

	color.Yellow(" - users, tokens, and remember_tokens migrations created and executed")
	color.Yellow(" - users and tokens models created")
	color.Yellow(" - auth middleware created")
	color.Yellow("")
	color.Yellow(
		"Don't forget to add user and token models in data/models.go, and to add appropriate middleware to your routes!",
	)

	return nil
}
