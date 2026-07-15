package regius

import (
	"log"
	"os"

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

// MigrateVersion returns the current migration version and dirty flag using golang-migrate.
func (r *Regius) MigrateVersion(dsn string) (uint, bool, error) {
	m, err := migrate.New("file://"+r.RootPath+"/migrations", dsn)
	if err != nil {
		return 0, false, err
	}

	defer m.Close()

	version, dirty, err := m.Version()
	if err != nil {
		return 0, false, err
	}

	return version, dirty, nil
}

func (r *Regius) PopConnect() (*pop.Connection, error) {
	sslMode := os.Getenv("DATABASE_SSL_MODE")
	cd := &pop.ConnectionDetails{
		Dialect:  r.DB.DataType,
		Host:     os.Getenv("DATABASE_HOST"),
		Port:     os.Getenv("DATABASE_PORT"),
		Database: os.Getenv("DATABASE_NAME"),
		User:     os.Getenv("DATABASE_USER"),
		Password: os.Getenv("DATABASE_PASS"),
	}
	if sslMode != "" {
		cd.Options = map[string]string{"sslmode": sslMode}
	}

	conn, err := pop.NewConnection(cd)
	if err != nil {
		return nil, err
	}

	if err := conn.Open(); err != nil {
		return nil, err
	}

	return conn, nil
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

type schemaMigration struct {
	Version string `db:"version"`
}

// PopMigrateVersion returns the most recently applied migration version from
// pop's schema_migration table. It is used as a fallback for databases where
// golang-migrate is not configured (e.g. SQLite).
func (r *Regius) PopMigrateVersion(tx *pop.Connection) (string, error) {
	var sm schemaMigration
	if err := tx.RawQuery("SELECT version FROM schema_migration ORDER BY version DESC LIMIT 1").First(&sm); err != nil {
		return "", err
	}
	return sm.Version, nil
}
