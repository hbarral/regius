package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Regius CLI",
	Long:  `Print the version number of Regius CLI`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Yellow(fmt.Sprintf("Regius CLI version: %s", Version))
	},
}
