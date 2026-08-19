package cli

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var b = &Backend{}

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "regius",
	Short: "Regius CLI tool for web application development",
	Long: `Regius CLI provides commands for creating and managing Regius web applications.
It includes tools for database migrations, code generation, and application management.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Skip setup for commands that don't need environment.
		// The top-level "version" command is skipped, but "migrate version"
		// (a child of "migrate") still needs the environment.
		if cmd.Name() == "new" || cmd.Name() == "help" {
			return
		}
		if cmd.Name() == "version" && cmd.Parent() != nil && cmd.Parent().Name() == "regius" {
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

		b.RootPath = path
		b.DBType = os.Getenv("DATABASE_TYPE")
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
