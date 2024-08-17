package regius

import (
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gobuffalo/pop"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func (r *Regius) MigrateUp(dsn string) error {
	m, err := migrate.New("file://"+r.RootPath+"/migrations", dsn)
	if err != nil {
		return err
	}

	defer m.Close()

	if err := m.Up(); err != nil {
		log.Println("Error running migration:", err)
	}

	return nil
}

func (r *Regius) MigrateDownAll(dsn string) error {
	m, err := migrate.New("file://"+r.RootPath+"/migrations", dsn)
	if err != nil {
		return err
	}

	defer m.Close()

	if err := m.Down(); err != nil {
		return err
	}

	return nil
}

func (r *Regius) Steps(n int, dsn string) error {
	m, err := migrate.New("file://"+r.RootPath+"/migrations", dsn)
	if err != nil {
		return err
	}

	defer m.Close()

	if err := m.Steps(n); err != nil {
		return err
	}

	return nil
}

func (r *Regius) MigrateForce(dsn string) error {
	m, err := migrate.New("file://"+r.RootPath+"/migrations", dsn)
	if err != nil {
		return err
	}

	defer m.Close()

	if err := m.Force(-1); err != nil {
		return err
	}

	return nil
}

func (r *Regius) PopConnect() (*pop.Connection, error) {
	tx, err := pop.Connect("development")
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (r *Regius) CreatePopMigration(up, down []byte, migrationName, migrationType string) error {
	migrationPath := r.RootPath + "/migrations"
	err := pop.MigrationCreate(migrationPath, migrationName, migrationType, up, down)
	if err != nil {
		return err
	}

	return nil
}

func (r *Regius) RunPopMigrations(tx *pop.Connection) error {
	migrationPath := r.RootPath + "/migrations"

	fm, err := pop.NewFileMigrator(migrationPath, tx)
	if err != nil {
		return err
	}

	err = fm.Up()
	if err != nil {
		return err
	}

	return nil
}

func (r *Regius) PopMigrateDown(tx *pop.Connection, steps ...int) error {
	migrationPath := r.RootPath + "/migrations"

	step := 1
	if len(steps) > 0 {
		step = steps[0]
	}

	fm, err := pop.NewFileMigrator(migrationPath, tx)
	if err != nil {
		return err
	}

	err = fm.Down(step)
	if err != nil {
		return err
	}

	return nil
}

func (r *Regius) PopMigrateReset(tx *pop.Connection) error {
	migrationPath := r.RootPath + "/migrations"

	fm, err := pop.NewFileMigrator(migrationPath, tx)
	if err != nil {
		return err
	}

	err = fm.Reset()
	if err != nil {
		return err
	}

	return nil
}
