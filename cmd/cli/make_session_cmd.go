package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	// This will be called from make_cmd.go
}

var makeSessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Create session table",
	Long:  `Creates a table in the database as a session store.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := doSessionTable(); err != nil {
			exitGracefully(err)
		}
		color.Green("Session table created!")
	},
}
