package main

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(makeCmd)

	// Add subcommands
	makeCmd.AddCommand(makeMigrationCmd)
	makeCmd.AddCommand(makeAuthCmd)
	makeCmd.AddCommand(makeHandlerCmd)
	makeCmd.AddCommand(makeModelCmd)
	makeCmd.AddCommand(makeSessionCmd)
	makeCmd.AddCommand(makeKeyCmd)
	makeCmd.AddCommand(makeMailCmd)
}

var makeCmd = &cobra.Command{
	Use:   "make",
	Short: "Code generation commands",
	Long: `Generate code and configuration files for your Regius application.
Includes migrations, authentication, handlers, models, and more.`,
}
