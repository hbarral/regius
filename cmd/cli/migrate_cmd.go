package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateResetCmd)
	migrateCmd.AddCommand(migrateVersionCmd)
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database migrations",
	Long:  `Run database migrations to update your database schema.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to "up" if no subcommand specified
		if err := doMigrate("up", ""); err != nil {
			exitGracefully(err)
		}
		color.Green("Migrations complete!")
	},
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Run all pending migrations",
	Long:  `Run all up migrations that have not been run previously.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMigrate("up", ""); err != nil {
			exitGracefully(err)
		}
		color.Green("Migrations complete!")
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down [steps|all]",
	Short: "Reverse migrations",
	Long: `Reverse the most recent migration.
Use "all" to reverse all migrations, or specify a number of steps to reverse.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		steps := ""
		if len(args) > 0 {
			steps = args[0]
		}
		if err := doMigrate("down", steps); err != nil {
			exitGracefully(err)
		}
		color.Green("Migrations reversed!")
	},
}

var migrateResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset and re-run all migrations",
	Long:  `Run all down migrations in reverse order, and then all up migrations.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMigrate("reset", ""); err != nil {
			exitGracefully(err)
		}
		color.Green("Migrations reset complete!")
	},
}

var migrateVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show current migration version",
	Long:  `Display the current database migration version using golang-migrate, falling back to pop's schema_migration table for SQLite.`,
	Run: func(cmd *cobra.Command, args []string) {
		checkForDB()

		// SQLite is handled via pop because golang-migrate's SQLite driver is
		// not vendored in this version of the framework.
		dbType := strings.ToLower(os.Getenv("DATABASE_TYPE"))
		if dbType == "sqlite" || dbType == "sqlite3" {
			tx, err := reg.PopConnect()
			if err != nil {
				exitGracefully(err)
			}
			defer tx.Close()

			version, err := reg.PopMigrateVersion(tx)
			if err != nil {
				exitGracefully(err)
			}

			color.Yellow("Current migration version: %s (pop)", version)
			return
		}

		dsn, err := migrationDSN()
		if err != nil {
			exitGracefully(err)
		}

		version, dirty, err := reg.MigrateVersion(dsn)
		if err != nil {
			exitGracefully(err)
		}

		dirtyFlag := "clean"
		if dirty {
			dirtyFlag = "dirty"
		}

		color.Yellow("Current migration version: %d (%s)", version, dirtyFlag)
	},
}

func doMigrate(caseToCheck string, steps string) error {
	checkForDB()

	tx, err := reg.PopConnect()
	if err != nil {
		return err
	}
	defer tx.Close()

	switch caseToCheck {
	case "up":
		err := reg.RunPopMigrations(tx)
		if err != nil {
			return err
		}
	case "down":
		if steps == "all" {
			return reg.PopMigrateDown(tx, -1)
		}

		stepsInt, err := strconv.Atoi(steps)
		if err != nil {
			return fmt.Errorf("the number of steps must be a valid integer: %w", err)
		}
		return reg.PopMigrateDown(tx, stepsInt)
	case "reset":
		err := reg.PopMigrateReset(tx)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown migration command: %s", caseToCheck)
	}

	return nil
}
