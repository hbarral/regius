package main

import (
	"fmt"
	"strconv"
)

func doMigrate(caseToCheck string, steps string) error {
	checkForDB()

	tx, err := reg.PopConnect()
	if err != nil {
		exitGracefully(err)
	}
	defer tx.Close()

	switch caseToCheck {
	case "up":
		err := reg.RunPopMigrations(tx)
		if err != nil {
			return err
		}
	case "down":
		if steps == "all" {
			return reg.PopMigrateDown(tx, -1)
		}

		steps, err := strconv.Atoi(steps)
		if err != nil {
			return fmt.Errorf("the number of steps must be a valid integer: %w", err)
		}
		return reg.PopMigrateDown(tx, steps)
	case "reset":
		err := reg.PopMigrateReset(tx)
		if err != nil {
			return err
		}
	default:
		showHelp()
	}

	return nil
}
