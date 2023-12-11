package main

func doMigrate(arg2, arg3 string) error {
	dsn := getDSN()

	switch arg2 {
	case "up":
		err := reg.MigrateUp(dsn)
		if err != nil {
			return err
		}
	case "down":
		if arg3 == "all" {
			err := reg.MigrateDownAll(dsn)
			if err != nil {
				return err
			}
		} else {
			err := reg.Steps(-1, dsn)
			if err != nil {
				return err
			}
		}
	case "reset":
		err := reg.MigrateDownAll(dsn)
		if err != nil {
			return err
		}
		err = reg.MigrateUp(dsn)
		if err != nil {
			return err
		}
	default:
		showHelp()
	}

	return nil
}
