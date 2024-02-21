package main

import (
	"fmt"
	"time"

	"github.com/fatih/color"
)

func doAuth() error {
	dbType := reg.DB.DataType
	fileName := fmt.Sprintf("%d_create_auth_tables", time.Now().UnixMicro())
	upFile := reg.RootPath + "/migrations/" + fileName + ".up.sql"
	downFile := reg.RootPath + "/migrations/" + fileName + ".down.sql"

	err := copyFileFromTemplate("templates/migrations/auth_tables."+dbType+".sql", upFile)
	if err != nil {
		exitGracefully(err)
	}

	err = copyDataToFile(
		[]byte(
			"DROP TABLE IF EXISTS users CASCADE; DROP TABLE IF EXISTS tokens CASCADE; DROP TABLE IF EXISTS remember_tokens;",
		),
		downFile,
	)
	if err != nil {
		exitGracefully(err)
	}

	err = doMigrate("up", "")
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

	err = copyFileFromTemplate(
		"templates/middleware/remember",
		reg.RootPath+"/middleware/remember.go",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/handlers/auth-handlers",
		reg.RootPath+"/handlers/auth-handlers.go",
	)
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
		"templates/mailer/password-reset.html.tmpl",
		reg.RootPath+"/mail/password-reset.html.tmpl",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/mailer/password-reset.plain.tmpl",
		reg.RootPath+"/mail/password-reset.plain.tmpl",
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
