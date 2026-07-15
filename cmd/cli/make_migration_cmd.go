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
	Short: "Create SQL migration files",
	Long:  `Create two new up and down SQL migrations in the migrations folder.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeMigration(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Migration created!")
	},
}

func doMakeMigration(name string) error {
	checkForDB()

	if name == "" {
		return errors.New("you must give the migration a name")
	}

	return reg.CreateMigration(nil, nil, name, "sql")
}
