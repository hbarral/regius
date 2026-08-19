package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	makeCmd.AddCommand(makeSeedCmd)
}

var makeSeedCmd = &cobra.Command{
	Use:   "seed [name]",
	Short: "Create a database seed file",
	Long:  `Create a new SQL seed file in the seeds/ directory.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeSeed(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Seed file created!")
	},
}

func doMakeSeed(name string) error {
	if name == "" {
		return errors.New("you must give the seed a name")
	}

	seedsDir := filepath.Join(b.RootPath, "seeds")
	if err := os.MkdirAll(seedsDir, 0755); err != nil {
		return err
	}

	fileName := fmt.Sprintf("%d_%s.sql", time.Now().Unix(), name)
	filePath := filepath.Join(seedsDir, fileName)

	content := "-- Seed: " + name + "\n\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return err
	}

	return nil
}
