package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	// This will be called from make_cmd.go
}

var makeAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Create authentication system",
	Long: `Creates and runs migrations for authentication tables,
and creates models, middleware, handlers, and views.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := doAuth(); err != nil {
			exitGracefully(err)
		}
		color.Green("Authentication system created!")
	},
}
