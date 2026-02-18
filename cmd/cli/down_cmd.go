package main

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(downCmd)
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Put server in maintenance mode",
	Long:  `Puts the server in maintenance mode via RPC.`,
	Run: func(cmd *cobra.Command, args []string) {
		rpcClient(true)
	},
}
