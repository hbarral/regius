package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
)

func setup(arg1, arg2 string) {
	if arg1 != "new" && arg1 != "version" && arg1 != "help" {
		err := godotenv.Load()
		if err != nil {
			exitGracefully(err)
		}

		path, err := os.Getwd()
		if err != nil {
			exitGracefully(err)
		}

		reg.RootPath = path
		reg.DB.DataType = os.Getenv("DATABASE_TYPE")

	}
}

func getDSN() string {
	dbType := reg.DB.DataType

	if dbType == "pgx" {
		dbType = "postgres"
	}

	if dbType == "postgres" {
		var dsn string
		if os.Getenv("DATABASE_PASS") != "" {
			dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
				os.Getenv("DATABASE_USER"),
				os.Getenv("DATABASE_PASS"),
				os.Getenv("DATABASE_HOST"),
				os.Getenv("DATABASE_PORT"),
				os.Getenv("DATABASE_NAME"),
				os.Getenv("DATABASE_SSL_MODE"),
			)
		} else {
			dsn = fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=%s",
				os.Getenv("DATABASE_USER"),
				os.Getenv("DATABASE_HOST"),
				os.Getenv("DATABASE_PORT"),
				os.Getenv("DATABASE_NAME"),
				os.Getenv("DATABASE_SSL_MODE"),
			)
		}
		return dsn
	}
	return "mysql://" + reg.BuildDSN()
}

func showHelp() {
	color.Yellow(`Available commands:

  help                            - show the help commands
  new <name>                      - creates a new application
  version                         - print application version
  migrate                         - runs all up mirgrations that have not been run previously
  migrate down                    - reverses the most recent migration
  migrate reset                   - runs all down mirgrations in reverse order, and then all up migrations
  make migration <name> <format>  - creates two new up and down migrations in the migrations folder; format can be fizz or sql
  make auth                       - creates and runs migrations for authentication tables, and creates models and middleware
  make handler <name>             - creates a stub handler in the handlers directory
  make model <name>               - creates a new model in the data directory
  make session                    - creates a table in the database as session store
  make mail <name>                - creates two starter mail templates in the mail directory
  down                            - put the server in maintenance mode
  up                              - bring the server back from maintenance mode

  `)
}

func updateSourceFiles(path string, fi os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if fi.IsDir() {
		return nil
	}

	matched, err := filepath.Match("*.go", fi.Name())
	if err != nil {
		return err
	}

	if matched {
		read, err := os.ReadFile(path)
		if err != nil {
			exitGracefully(err)
		}

		newContents := strings.Replace(string(read), "regius-app", appURL, -1)

		err = os.WriteFile(path, []byte(newContents), 0o644)
		if err != nil {
			exitGracefully(err)
		}
	}

	return nil
}

func updateSource() {
	err := filepath.Walk(".", updateSourceFiles)
	if err != nil {
		exitGracefully(err)
	}
}

func checkForDB() {
	if reg.DB.DataType == "" {
		exitGracefully(errors.New("you must set DATABASE_TYPE in .env"))
	}

	if !fileExists(reg.RootPath + "/config/database.yml") {
		exitGracefully(errors.New("config/database.yml not found"))
	}
}
