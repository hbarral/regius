package regius

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// migrationSource returns the golang-migrate source URL for the application's
// migrations directory.
func (r *Regius) migrationSource() string {
	return "file://" + filepath.Join(r.RootPath, "migrations")
}

// newMigrate returns a new golang-migrate instance for the configured
// migrations directory and the given database DSN.
func (r *Regius) newMigrate(dsn string) (*migrate.Migrate, error) {
	return migrate.New(r.migrationSource(), dsn)
}

// OpenAppDB opens a standard *sql.DB for the application using environment
// variables and applies connection pool configuration.
func (r *Regius) OpenAppDB() (*sql.DB, error) {
	dsn, err := r.BuildDSN()
	if err != nil {
		return nil, fmt.Errorf("failed to build database DSN: %w", err)
	}

	db, err := r.OpenDB(os.Getenv("DATABASE_TYPE"), dsn)
	if err != nil {
		return nil, err
	}

	d := Database{Pool: db}
	if err := d.ConfigurePool(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// CreateMigration creates up and down migration files in the migrations
// directory. Only "sql" format is supported
func (r *Regius) CreateMigration(up, down []byte, migrationName, migrationType string) error {
	migrationPath := filepath.Join(r.RootPath, "migrations")
	if err := os.MkdirAll(migrationPath, 0755); err != nil {
		return fmt.Errorf("failed to create migrations directory: %w", err)
	}

	timestamp := time.Now().UTC().Format("20060102150405")
	baseName := fmt.Sprintf("%s_%s", timestamp, migrationName)

	upPath := filepath.Join(migrationPath, baseName+".up.sql")
	downPath := filepath.Join(migrationPath, baseName+".down.sql")

	if _, err := os.Stat(upPath); err == nil {
		return fmt.Errorf("migration %s already exists", baseName)
	}
	if _, err := os.Stat(downPath); err == nil {
		return fmt.Errorf("migration %s already exists", baseName)
	}

	if len(up) == 0 {
		up = []byte("-- Migration: " + migrationName + "\n")
	}
	if len(down) == 0 {
		down = []byte("-- Revert migration: " + migrationName + "\n")
	}

	if err := os.WriteFile(upPath, up, 0644); err != nil {
		return fmt.Errorf("failed to write up migration: %w", err)
	}
	if err := os.WriteFile(downPath, down, 0644); err != nil {
		return fmt.Errorf("failed to write down migration: %w", err)
	}

	return nil
}

// RunMigrations runs all pending up migrations using golang-migrate.
func (r *Regius) RunMigrations(dsn string) error {
	if err := r.importPopMigrationState(); err != nil {
		return fmt.Errorf("failed to import legacy migration state: %w", err)
	}

	m, err := r.newMigrate(dsn)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// MigrateDown reverses migrations. Use steps=-1 to reverse all migrations.
func (r *Regius) MigrateDown(dsn string, steps int) error {
	m, err := r.newMigrate(dsn)
	if err != nil {
		return err
	}
	defer m.Close()

	if steps < 0 {
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
		return nil
	}

	if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// MigrateReset runs all down migrations in reverse order, then all up migrations.
func (r *Regius) MigrateReset(dsn string) error {
	m, err := r.newMigrate(dsn)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// MigrateVersion returns the current migration version and dirty flag using
// golang-migrate.
func (r *Regius) MigrateVersion(dsn string) (uint, bool, error) {
	m, err := r.newMigrate(dsn)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()

	version, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, false, nil
		}
		return 0, false, err
	}

	return version, dirty, nil
}

// MigrationDSNForCLI builds a golang-migrate DSN from environment variables.
// MySQL/MariaDB and SQLite require a protocol prefix that the raw driver DSN
// omits.
func (r *Regius) MigrationDSNForCLI() (string, error) {
	dsn, err := r.BuildDSN()
	if err != nil {
		return "", err
	}

	switch strings.ToLower(os.Getenv("DATABASE_TYPE")) {
	case "mysql", "mariadb":
		return "mysql://" + dsn, nil
	case "sqlite", "sqlite3":
		return "sqlite://" + dsn, nil
	default:
		return dsn, nil
	}
}

// importPopMigrationState copies migration version state from pop's
// schema_migration table to golang-migrate's schema_migrations table when the
// former exists and the latter does not. This is idempotent: once
// schema_migrations exists, the import is skipped.
func (r *Regius) importPopMigrationState() error {
	rawDSN, err := r.BuildDSN()
	if err != nil {
		return err
	}

	driver := normalizeDBTypeForMigrate(os.Getenv("DATABASE_TYPE"))
	if driver == "" {
		return fmt.Errorf("unsupported database type for migration import: %s", os.Getenv("DATABASE_TYPE"))
	}

	db, err := sql.Open(driver, rawDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	var hasPopTable bool
	if err := db.QueryRow(tableExistsSQL(os.Getenv("DATABASE_TYPE"), "schema_migration")).Scan(&hasPopTable); err != nil {
		// Treat lookup errors as "no pop table".
		hasPopTable = false
	}
	if !hasPopTable {
		return nil
	}

	var hasGMTable bool
	if err := db.QueryRow(tableExistsSQL(os.Getenv("DATABASE_TYPE"), "schema_migrations")).Scan(&hasGMTable); err != nil {
		hasGMTable = false
	}
	if hasGMTable {
		return nil
	}

	var versionStr string
	row := db.QueryRow("SELECT version FROM schema_migration ORDER BY version DESC LIMIT 1")
	if err := row.Scan(&versionStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}

	version, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid pop migration version %q: %w", versionStr, err)
	}

	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version bigint primary key, dirty boolean not null)"); err != nil {
		return err
	}

	if _, err := db.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (?, false)", version); err != nil {
		return err
	}

	if r.InfoLog != nil {
		r.InfoLog.Printf("Imported legacy pop migration state: version %d", version)
	}

	return nil
}

// normalizeDBTypeForMigrate returns the golang-migrate database name for the
// given DATABASE_TYPE value.
func normalizeDBTypeForMigrate(dbType string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return "postgres"
	case "mysql", "mariadb":
		return "mysql"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return ""
	}
}

// tableExistsSQL returns a database-specific query that selects 1 if the table
// exists and 0 otherwise.
func tableExistsSQL(dbType, tableName string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return fmt.Sprintf(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = '%s'",
			tableName,
		)
	case "mysql", "mariadb":
		return fmt.Sprintf(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = '%s'",
			tableName,
		)
	case "sqlite", "sqlite3":
		return fmt.Sprintf(
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = '%s'",
			tableName,
		)
	default:
		return ""
	}
}

// listMigrations returns the base names of all migration files in the
// migrations directory, sorted by version.
func (r *Regius) listMigrations() ([]string, error) {
	migrationPath := filepath.Join(r.RootPath, "migrations")
	entries, err := os.ReadDir(migrationPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	seen := make(map[string]struct{})
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") && !strings.HasSuffix(name, ".down.sql") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".up.sql"), ".down.sql")
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}
		names = append(names, base)
	}

	sort.Strings(names)
	return names, nil
}
