package cli

import (
	"github.com/spf13/cobra"
)

// makeRenderer is set by --renderer on the make subcommands that care about the
// template engine (make auth, make handler). Empty falls back to the RENDERER
// env var and then the default (templ).
var makeRenderer string

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
	makeCmd.AddCommand(makeGormModelCmd)
	makeCmd.AddCommand(makeLocaleCmd)
	makeCmd.AddCommand(makeAPICmd)
}

var makeCmd = &cobra.Command{
	Use:   "make",
	Short: "Code generation commands",
	Long: `Generate code and configuration files for your Regius application.
Includes migrations, authentication, handlers, models, and more.`,
}
