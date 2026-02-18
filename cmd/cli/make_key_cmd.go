package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	// This will be called from make_cmd.go
}

var makeKeyCmd = &cobra.Command{
	Use:   "key",
	Short: "Generate encryption key",
	Long:  `Generates a 32-character random encryption key.`,
	Run: func(cmd *cobra.Command, args []string) {
		key := reg.RandomString(32)
		color.Yellow("32 character encryption key: %s", key)
	},
}
