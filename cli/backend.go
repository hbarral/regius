package cli

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"

	_ "github.com/golang-migrate/migrate/v4/source/file"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v4/stdlib"
)

// Backend is the lightweight runtime context the CLI needs from the framework.
// It replaces the full *regius.Regius struct so the CLI binary does not link
// framework-only dependencies (AWS SDK, go-rod, badger, mail providers, etc.).
type Backend struct {
	RootPath string
	DBType   string
}

// RandomString generates a random string of length n using crypto/rand.
func (b *Backend) RandomString(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_+"
	byt := make([]byte, n)
	_, _ = rand.Read(byt)
	for i := range byt {
		byt[i] = chars[byt[i]%byte(len(chars))]
	}
	return string(byt)
}

// CreateMigration creates up and down migration files in the migrations
// directory.
func (b *Backend) CreateMigration(up, down []byte, migrationName, migrationType string) error {
	migrationPath := filepath.Join(b.RootPath, "migrations")
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

// BuildDSN builds a database DSN from environment variables.
func (b *Backend) BuildDSN() (string, error) {
	return b.buildDSN("DATABASE_")
}

func (b *Backend) buildDSN(prefix string) (string, error) {
	dbType := strings.ToLower(os.Getenv("DATABASE_TYPE"))

	get := func(name string) string {
		if v := os.Getenv(prefix + name); v != "" {
			return v
		}
		if prefix != "DATABASE_" {
			return os.Getenv("DATABASE_" + name)
		}
		return ""
	}

	switch dbType {
	case "postgres", "postgresql":
		dsn := fmt.Sprintf(
			"host=%s port=%s user=%s dbname=%s sslmode=%s timezone=UTC connect_timeout=5",
			get("HOST"),
			get("PORT"),
			get("USER"),
			get("NAME"),
			get("SSL_MODE"),
		)
		if pass := get("PASS"); pass != "" {
			dsn = fmt.Sprintf("%s password=%s", dsn, pass)
		}
		return dsn, nil

	case "mysql", "mariadb":
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true&loc=UTC",
			get("USER"),
			get("PASS"),
			get("HOST"),
			get("PORT"),
			get("NAME"),
		), nil

	case "sqlite", "sqlite3":
		name := get("NAME")
		if name == "" {
			name = "regius"
		}
		if name == ":memory:" {
			return "file::memory:?cache=shared", nil
		}
		if !filepath.IsAbs(name) {
			name = filepath.Join(b.RootPath, "data", name+".db")
		}
		return fmt.Sprintf("file:%s?_foreign_keys=on", name), nil

	default:
		return "", fmt.Errorf("unsupported database type: %s", os.Getenv("DATABASE_TYPE"))
	}
}

// OpenDB opens a standard *sql.DB for the application using environment
// variables.
func (b *Backend) OpenDB() (*sql.DB, error) {
	dsn, err := b.BuildDSN()
	if err != nil {
		return nil, fmt.Errorf("failed to build database DSN: %w", err)
	}

	driver := normalizeDriver(os.Getenv("DATABASE_TYPE"))
	if driver == "" {
		return nil, fmt.Errorf("unsupported database type: %s", os.Getenv("DATABASE_TYPE"))
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// MigrationDSNForCLI builds a golang-migrate DSN from environment variables.
func (b *Backend) MigrationDSNForCLI() (string, error) {
	dsn, err := b.BuildDSN()
	if err != nil {
		return "", err
	}

	switch strings.ToLower(os.Getenv("DATABASE_TYPE")) {
	case "mysql", "mariadb":
		return "mysql://" + dsn, nil
	case "sqlite", "sqlite3":
		return "sqlite3://" + dsn, nil
	default:
		return dsn, nil
	}
}

func (b *Backend) migrationSource() string {
	return "file://" + filepath.Join(b.RootPath, "migrations")
}

func (b *Backend) newMigrate(dsn string) (*migrate.Migrate, error) {
	return migrate.New(b.migrationSource(), dsn)
}

// RunMigrations runs all pending up migrations using golang-migrate.
func (b *Backend) RunMigrations(dsn string) error {
	if err := b.importPopMigrationState(); err != nil {
		return fmt.Errorf("failed to import legacy migration state: %w", err)
	}

	m, err := b.newMigrate(dsn)
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
func (b *Backend) MigrateDown(dsn string, steps int) error {
	m, err := b.newMigrate(dsn)
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
func (b *Backend) MigrateReset(dsn string) error {
	m, err := b.newMigrate(dsn)
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

// MigrateVersion returns the current migration version and dirty flag.
func (b *Backend) MigrateVersion(dsn string) (uint, bool, error) {
	m, err := b.newMigrate(dsn)
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

// NewSeeder creates a seeder for the configured application database.
func (b *Backend) NewSeeder() *Seeder {
	db, err := b.OpenDB()
	if err != nil {
		log.Printf("warning: failed to open database for seeder: %v", err)
		return &Seeder{DB: nil, RootPath: b.RootPath}
	}
	return &Seeder{DB: db, RootPath: b.RootPath}
}

// importPopMigrationState copies migration version state from pop's
// schema_migration table to golang-migrate's schema_migrations table.
func (b *Backend) importPopMigrationState() error {
	rawDSN, err := b.BuildDSN()
	if err != nil {
		return err
	}

	driver := normalizeMigrateDriver(os.Getenv("DATABASE_TYPE"))
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

	log.Printf("Imported legacy pop migration state: version %d", version)

	return nil
}

func normalizeDriver(dbType string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return "pgx"
	case "mysql", "mariadb":
		return "mysql"
	case "sqlite", "sqlite3":
		return "sqlite3"
	default:
		return ""
	}
}

func normalizeMigrateDriver(dbType string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return "postgres"
	case "mysql", "mariadb":
		return "mysql"
	case "sqlite", "sqlite3":
		return "sqlite3"
	default:
		return ""
	}
}

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
