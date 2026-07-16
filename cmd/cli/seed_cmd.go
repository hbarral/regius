package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(seedCmd)
}

var seedCmd = &cobra.Command{
	Use:   "db:seed",
	Short: "Run database seeders",
	Long: `Run all pending SQL seed files in the seeds/ directory.
Seed files are executed in filename order and tracked in the regius_seeds table.`,
	Run: func(cmd *cobra.Command, args []string) {
		checkForDB()

		seeder := reg.NewSeeder()
		if err := seeder.RunSeeds(); err != nil {
			exitGracefully(err)
		}

		color.Green("Database seeded successfully!")
	},
}
