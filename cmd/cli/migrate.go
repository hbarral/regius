package main

func doMigrate(arg2, arg3 string) error {
	checkForDB()

	tx, err := reg.PopConnect()
	if err != nil {
		exitGracefully(err)
	}
	defer tx.Close()

	switch arg2 {
	case "up":
		err := reg.RunPopMigrations(tx)
		if err != nil {
			return err
		}
	case "down":
		if arg3 == "all" {
			err := reg.PopMigrateDown(tx, -1)
			if err != nil {
				return err
			}
		} else {
			err := reg.PopMigrateDown(tx, 1)
			if err != nil {
				return err
			}
		}
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
