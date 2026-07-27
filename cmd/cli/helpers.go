package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// migrationDSN returns a DSN suitable for golang-migrate.
func migrationDSN() (string, error) {
	return reg.MigrationDSNForCLI()
}

// normalizeDBType maps the user-facing DATABASE_TYPE aliases (sqlite3,
// postgresql, mariadb) to the canonical template-file suffix used across the
// embedded migration templates (sqlite, postgres, mysql).
func normalizeDBType(dbType string) string {
	switch strings.ToLower(dbType) {
	case "sqlite3":
		return "sqlite"
	case "postgresql":
		return "postgres"
	case "mariadb":
		return "mysql"
	default:
		return strings.ToLower(dbType)
	}
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

func exitGracefully(err error, msg ...string) {
	if err != nil {
		exitWithError(err)
	}

	if len(msg) > 0 {
		exitWithSuccess(msg[0])
	}

	exitWithSuccess("Finished!")
}

func checkForDB() {
	if reg.DB.DataType == "" {
		exitGracefully(errors.New("you must set DATABASE_TYPE in .env"))
	}

	// SQLite uses a local file path (DATABASE_NAME, optional) and does not
	// need a network host, port, or database name.
	if normalizeDBType(reg.DB.DataType) == "sqlite" {
		return
	}

	if os.Getenv("DATABASE_HOST") == "" {
		exitGracefully(errors.New("DATABASE_HOST must be set"))
	}

	if os.Getenv("DATABASE_NAME") == "" {
		exitGracefully(errors.New("DATABASE_NAME must be set"))
	}
}
