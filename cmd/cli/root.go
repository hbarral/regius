package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/hbarral/regius"
)

var reg regius.Regius

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "regius",
	Short: "Regius CLI tool for web application development",
	Long: `Regius CLI provides commands for creating and managing Regius web applications.
It includes tools for database migrations, code generation, and application management.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Skip setup for commands that don't need environment
		if cmd.Name() == "new" || cmd.Name() == "version" || cmd.Name() == "help" {
			return
		}

		// Load .env file
		if err := godotenv.Load(); err != nil {
			exitWithError(fmt.Errorf("failed to load .env file: %w", err))
		}

		// Set up regius instance
		path, err := os.Getwd()
		if err != nil {
			exitWithError(fmt.Errorf("failed to get current directory: %w", err))
		}

		reg.RootPath = path
		reg.DB.DataType = os.Getenv("DATABASE_TYPE")
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {

}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		exitWithError(err)
	}
}

func exitWithError(err error) {
	color.Red("Error: %v", err)
	os.Exit(1)
}

func exitWithSuccess(message string) {
	color.Green(message)
	os.Exit(0)
}
