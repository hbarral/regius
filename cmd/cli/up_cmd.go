package main

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(upCmd)
}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Bring server back from maintenance mode",
	Long:  `Brings the server back from maintenance mode via RPC.`,
	Run: func(cmd *cobra.Command, args []string) {
		rpcClient(false)
	},
}
