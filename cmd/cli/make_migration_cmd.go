package main

import (
	"errors"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	// This will be called from make_cmd.go
}

var makeMigrationCmd = &cobra.Command{
	Use:   "migration [name]",
	Short: "Create migration files",
	Long: `Create two new up and down migrations in the migrations folder.
Format can be fizz or sql (default: fizz).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		format, _ := cmd.Flags().GetString("format")
		if err := doMakeMigration(args[0], format); err != nil {
			exitGracefully(err)
		}
		color.Green("Migration created!")
	},
}

func init() {
	makeMigrationCmd.Flags().StringP("format", "f", "fizz", "Migration format (fizz or sql)")
}

func doMakeMigration(name, format string) error {
	checkForDB()

	if name == "" {
		return errors.New("you must give the migration a name")
	}

	migrationType := "fizz"
	var up, down []byte

	if format == "fizz" || format == "" {
		upBytes, err := templateFS.ReadFile("templates/migrations/migration_up.fizz")
		if err != nil {
			return err
		}
		downBytes, err := templateFS.ReadFile("templates/migrations/migration_down.fizz")
		if err != nil {
			return err
		}

		up = upBytes
		down = downBytes
	} else {
		migrationType = "sql"
	}

	return reg.CreatePopMigration(up, down, name, migrationType)
}
