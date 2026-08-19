package cli

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	// This will be called from make_cmd.go
	makeAuthCmd.Flags().StringVar(&makeRenderer, "renderer", "", "template engine for auth views (templ|jet|go); defaults to RENDERER env or templ")
}

var makeAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Create authentication system",
	Long: `Creates migrations for authentication tables,
and creates models, middleware, handlers, and views.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := doAuth(); err != nil {
			exitGracefully(err)
		}
		color.Green("Authentication system created!")
	},
}
