package regius

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMigrationTest(t *testing.T) (*Regius, string, string) {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	r := &Regius{RootPath: root}
	return r, dbPath, root
}

func TestRegius_CreateMigration(t *testing.T) {
	r, _, _ := setupMigrationTest(t)

	err := r.CreateMigration([]byte("CREATE TABLE users (id INTEGER);"), []byte("DROP TABLE users;"), "create_users", "sql")
	require.NoError(t, err)

	files, err := os.ReadDir(filepath.Join(r.RootPath, "migrations"))
	require.NoError(t, err)
	require.Len(t, files, 2)

	var hasUp, hasDown bool
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".sql" {
			if strings.HasSuffix(f.Name(), ".up.sql") {
				hasUp = true
			}
			if strings.HasSuffix(f.Name(), ".down.sql") {
				hasDown = true
			}
		}
	}
	assert.True(t, hasUp)
	assert.True(t, hasDown)
}

func TestRegius_CreateMigration_Duplicate(t *testing.T) {
	r, _, _ := setupMigrationTest(t)

	require.NoError(t, r.CreateMigration(nil, nil, "create_users", "sql"))
	err := r.CreateMigration(nil, nil, "create_users", "sql")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRegius_RunMigrations(t *testing.T) {
	r, dbPath, _ := setupMigrationTest(t)
	t.Setenv("DATABASE_TYPE", "sqlite3")
	t.Setenv("DATABASE_NAME", dbPath)

	require.NoError(t, r.CreateMigration(
		[]byte("CREATE TABLE users (id INTEGER PRIMARY KEY);"),
		[]byte("DROP TABLE users;"),
		"create_users",
		"sql",
	))

	dsn := "sqlite3://" + dbPath
	err := r.RunMigrations(dsn)
	require.NoError(t, err)

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer db.Close()

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = 'users'").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestRegius_MigrateVersion(t *testing.T) {
	r, dbPath, _ := setupMigrationTest(t)
	t.Setenv("DATABASE_TYPE", "sqlite3")
	t.Setenv("DATABASE_NAME", dbPath)

	require.NoError(t, r.CreateMigration(
		[]byte("CREATE TABLE users (id INTEGER PRIMARY KEY);"),
		[]byte("DROP TABLE users;"),
		"create_users",
		"sql",
	))

	dsn := "sqlite3://" + dbPath
	require.NoError(t, r.RunMigrations(dsn))

	version, dirty, err := r.MigrateVersion(dsn)
	require.NoError(t, err)
	assert.NotZero(t, version)
	assert.False(t, dirty)
}

func TestRegius_MigrateDown(t *testing.T) {
	r, dbPath, _ := setupMigrationTest(t)
	t.Setenv("DATABASE_TYPE", "sqlite3")
	t.Setenv("DATABASE_NAME", dbPath)

	require.NoError(t, r.CreateMigration(
		[]byte("CREATE TABLE users (id INTEGER PRIMARY KEY);"),
		[]byte("DROP TABLE users;"),
		"create_users",
		"sql",
	))

	dsn := "sqlite3://" + dbPath
	require.NoError(t, r.RunMigrations(dsn))
	require.NoError(t, r.MigrateDown(dsn, 1))

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer db.Close()

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = 'users'").Scan(&count))
	assert.Equal(t, 0, count)
}

func TestRegius_MigrateReset(t *testing.T) {
	r, dbPath, _ := setupMigrationTest(t)
	t.Setenv("DATABASE_TYPE", "sqlite3")
	t.Setenv("DATABASE_NAME", dbPath)

	require.NoError(t, r.CreateMigration(
		[]byte("CREATE TABLE users (id INTEGER PRIMARY KEY);"),
		[]byte("DROP TABLE users;"),
		"create_users",
		"sql",
	))

	dsn := "sqlite3://" + dbPath
	require.NoError(t, r.RunMigrations(dsn))
	require.NoError(t, r.MigrateReset(dsn))

	version, _, err := r.MigrateVersion(dsn)
	require.NoError(t, err)
	assert.NotZero(t, version)
}

func TestRegius_importPopMigrationState(t *testing.T) {
	r, dbPath, _ := setupMigrationTest(t)
	t.Setenv("DATABASE_TYPE", "sqlite3")
	t.Setenv("DATABASE_NAME", dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE schema_migration (version TEXT PRIMARY KEY)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO schema_migration (version) VALUES ('20240101000000')")
	require.NoError(t, err)

	require.NoError(t, r.importPopMigrationState())

	var version int64
	var dirty bool
	require.NoError(t, db.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
	assert.Equal(t, int64(20240101000000), version)
	assert.False(t, dirty)
}

func TestRegius_MigrationDSNForCLI(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "mysql")
	t.Setenv("DATABASE_USER", "user")
	t.Setenv("DATABASE_PASS", "pass")
	t.Setenv("DATABASE_HOST", "host")
	t.Setenv("DATABASE_PORT", "3306")
	t.Setenv("DATABASE_NAME", "db")

	r := &Regius{}
	dsn, err := r.MigrationDSNForCLI()

	require.NoError(t, err)
	assert.Equal(t, "mysql://user:pass@tcp(host:3306)/db?parseTime=true&multiStatements=true&loc=UTC", dsn)
}

func TestRegius_MigrationDSNForCLI_SQLite(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "sqlite3")
	t.Setenv("DATABASE_NAME", "/tmp/app.db")

	r := &Regius{}
	dsn, err := r.MigrationDSNForCLI()

	require.NoError(t, err)
	assert.Equal(t, "sqlite3://file:/tmp/app.db?_foreign_keys=on", dsn)
}

func TestRegius_MigrationDSNForCLI_Postgres(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgres")
	t.Setenv("DATABASE_HOST", "host")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "user")
	t.Setenv("DATABASE_NAME", "db")
	t.Setenv("DATABASE_SSL_MODE", "disable")

	r := &Regius{}
	dsn, err := r.MigrationDSNForCLI()

	require.NoError(t, err)
	assert.Equal(t, "host=host port=5432 user=user dbname=db sslmode=disable timezone=UTC connect_timeout=5", dsn)
}
