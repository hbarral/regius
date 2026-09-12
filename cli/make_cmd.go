package cli

import (
	"github.com/spf13/cobra"
)

// makeRenderer is set by --renderer on the make subcommands that care about the
// template engine (make auth, make handler). Empty falls back to the RENDERER
// env var and then the default (templ).
var makeRenderer string

// makeMiddlewareGlobal is set by --global on make middleware: also wire the
// generated middleware into the global chain in routes.go.
var makeMiddlewareGlobal bool

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
	makeCmd.AddCommand(makeWebhookCmd)
	makeCmd.AddCommand(makeJobCmd)
	makeCmd.AddCommand(makeWebsocketCmd)
	makeCmd.AddCommand(makeMiddlewareCmd)
	makeCmd.AddCommand(makeServiceCmd)
	makeCmd.AddCommand(makeResourceCmd)
	makeCmd.AddCommand(makeCrudCmd)
}

var makeCmd = &cobra.Command{
	Use:   "make",
	Short: "Code generation commands",
	Long: `Generate code and configuration files for your Regius application.
Includes migrations, authentication, handlers, models, and more.`,
}
