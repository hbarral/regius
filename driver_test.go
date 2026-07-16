package regius

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDBType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"postgres", "pgx", false},
		{"postgresql", "pgx", false},
		{"mysql", "mysql", false},
		{"mariadb", "mysql", false},
		{"sqlite", "sqlite3", false},
		{"sqlite3", "sqlite3", false},
		{"", "", true},
		{"oracle", "", true},
		{"unknown", "", true},
	}

	for _, e := range tests {
		got, err := normalizeDBType(e.input)
		if e.wantErr {
			assert.Error(t, err, "input: %q", e.input)
		} else {
			require.NoError(t, err, "input: %q", e.input)
			assert.Equal(t, e.expected, got, "input: %q", e.input)
		}
	}
}

func TestOpenDB_SQLiteMemory(t *testing.T) {
	r := &Regius{}
	db, err := r.OpenDB("sqlite", "file::memory:?cache=shared")

	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	assert.NoError(t, db.Ping())
}

func TestOpenDB_SQLiteMemoryAlias(t *testing.T) {
	r := &Regius{}
	db, err := r.OpenDB("sqlite3", "file::memory:?cache=shared")

	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()
}

func TestOpenDB_MySQLConnectionRefused(t *testing.T) {
	r := &Regius{}
	// Port 1 -> connection refused immediately.
	_, err := r.OpenDB("mysql", "user:pass@tcp(127.0.0.1:1)/test?timeout=2s")

	require.Error(t, err)
}

func TestOpenDB_PostgresConnectionRefused(t *testing.T) {
	r := &Regius{}
	_, err := r.OpenDB("postgres", "host=localhost port=1 sslmode=disable connect_timeout=2")

	require.Error(t, err)
}

func TestOpenDB_UnsupportedType(t *testing.T) {
	r := &Regius{}
	_, err := r.OpenDB("oracle", "some-dsn")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
}

func TestBuildDSN_Postgres(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgres")
	t.Setenv("DATABASE_HOST", "dbhost")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "dbuser")
	t.Setenv("DATABASE_NAME", "appdb")
	t.Setenv("DATABASE_SSL_MODE", "disable")
	t.Setenv("DATABASE_PASS", "s3cr3t")

	r := &Regius{}
	dsn, err := r.BuildDSN()

	require.NoError(t, err)
	assert.Contains(t, dsn, "host=dbhost")
	assert.Contains(t, dsn, "port=5432")
	assert.Contains(t, dsn, "user=dbuser")
	assert.Contains(t, dsn, "dbname=appdb")
	assert.Contains(t, dsn, "sslmode=disable")
	assert.Contains(t, dsn, "password=s3cr3t")
}

func TestBuildDSN_PostgresWithoutPassword(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgresql")
	t.Setenv("DATABASE_HOST", "dbhost")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "dbuser")
	t.Setenv("DATABASE_NAME", "appdb")
	t.Setenv("DATABASE_SSL_MODE", "require")
	t.Setenv("DATABASE_PASS", "")

	r := &Regius{}
	dsn, err := r.BuildDSN()

	require.NoError(t, err)
	assert.Contains(t, dsn, "host=dbhost")
	assert.NotContains(t, dsn, "password=", "no password= segment when DATABASE_PASS is empty")
}

func TestBuildDSN_MySQL(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "mysql")
	t.Setenv("DATABASE_HOST", "dbhost")
	t.Setenv("DATABASE_PORT", "3306")
	t.Setenv("DATABASE_USER", "dbuser")
	t.Setenv("DATABASE_PASS", "dbpass")
	t.Setenv("DATABASE_NAME", "appdb")

	r := &Regius{}
	dsn, err := r.BuildDSN()

	require.NoError(t, err)
	assert.Equal(t, "dbuser:dbpass@tcp(dbhost:3306)/appdb?parseTime=true&multiStatements=true&loc=UTC", dsn)
}

func TestBuildDSN_MariaDB(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "mariadb")
	t.Setenv("DATABASE_HOST", "dbhost")
	t.Setenv("DATABASE_PORT", "3306")
	t.Setenv("DATABASE_USER", "dbuser")
	t.Setenv("DATABASE_PASS", "dbpass")
	t.Setenv("DATABASE_NAME", "appdb")

	r := &Regius{}
	dsn, err := r.BuildDSN()

	require.NoError(t, err)
	assert.Equal(t, "dbuser:dbpass@tcp(dbhost:3306)/appdb?parseTime=true&multiStatements=true&loc=UTC", dsn)
}

func TestBuildDSN_SQLiteMemory(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "sqlite")
	t.Setenv("DATABASE_NAME", ":memory:")

	r := &Regius{}
	dsn, err := r.BuildDSN()

	require.NoError(t, err)
	assert.Equal(t, "file::memory:?cache=shared", dsn)
}

func TestBuildDSN_SQLiteFile(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "sqlite3")
	t.Setenv("DATABASE_NAME", "app")

	r := &Regius{RootPath: "/app/root"}
	dsn, err := r.BuildDSN()

	require.NoError(t, err)
	assert.Equal(t, "file:"+filepath.Join("/app/root", "data", "app.db")+"?_foreign_keys=on", dsn)
}

func TestBuildDSN_SQLiteFileAbsolutePath(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "sqlite")
	t.Setenv("DATABASE_NAME", "/var/lib/app.db")

	r := &Regius{RootPath: "/app/root"}
	dsn, err := r.BuildDSN()

	require.NoError(t, err)
	assert.Equal(t, "file:/var/lib/app.db?_foreign_keys=on", dsn)
}

func TestBuildDSN_Unsupported(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "oracle")

	r := &Regius{}
	_, err := r.BuildDSN()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
}

func TestBuildReadDSN_NoConfig(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgres")
	t.Setenv("DATABASE_HOST", "mainhost")

	r := &Regius{}
	dsn, err := r.BuildReadDSN()

	require.NoError(t, err)
	assert.Empty(t, dsn)
}

func TestBuildReadDSN_FromHost(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgres")
	t.Setenv("DATABASE_HOST", "mainhost")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "mainuser")
	t.Setenv("DATABASE_NAME", "maindb")
	t.Setenv("DATABASE_SSL_MODE", "disable")
	t.Setenv("DATABASE_READ_HOST", "readhost")

	r := &Regius{}
	dsn, err := r.BuildReadDSN()

	require.NoError(t, err)
	assert.Contains(t, dsn, "host=readhost")
	assert.Contains(t, dsn, "port=5432")
	assert.Contains(t, dsn, "user=mainuser")
	assert.Contains(t, dsn, "dbname=maindb")
	assert.Contains(t, dsn, "sslmode=disable")
}

func TestBuildReadDSN_Overrides(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "mysql")
	t.Setenv("DATABASE_HOST", "mainhost")
	t.Setenv("DATABASE_PORT", "3306")
	t.Setenv("DATABASE_USER", "mainuser")
	t.Setenv("DATABASE_PASS", "mainpass")
	t.Setenv("DATABASE_NAME", "maindb")
	t.Setenv("DATABASE_READ_HOST", "readhost")
	t.Setenv("DATABASE_READ_PORT", "3307")
	t.Setenv("DATABASE_READ_USER", "readuser")
	t.Setenv("DATABASE_READ_PASS", "readpass")
	t.Setenv("DATABASE_READ_NAME", "readdb")

	r := &Regius{}
	dsn, err := r.BuildReadDSN()

	require.NoError(t, err)
	assert.Equal(t, "readuser:readpass@tcp(readhost:3307)/readdb?parseTime=true&multiStatements=true&loc=UTC", dsn)
}

func TestBuildReadDSN_ExplicitDSN(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgres")
	t.Setenv("DATABASE_READ_DSN", "postgres://readuser:readpass@readhost/appdb?sslmode=disable")

	r := &Regius{}
	dsn, err := r.BuildReadDSN()

	require.NoError(t, err)
	assert.Equal(t, "postgres://readuser:readpass@readhost/appdb?sslmode=disable", dsn)
}

func TestOpenDB_UnknownDriver(t *testing.T) {
	r := &Regius{}
	_, err := r.OpenDB("sqlite", "file:test.db")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sqlite")
}

func TestOpenDB_PostgresConnectionRefused(t *testing.T) {
	r := &Regius{}
	// Port 1 -> connection refused immediately; no live DB required.
	_, err := r.OpenDB("postgres", "host=localhost port=1 sslmode=disable connect_timeout=2")

	require.Error(t, err)
}

func TestOpenDB_PostgresqlAlias(t *testing.T) {
	r := &Regius{}
	_, err := r.OpenDB("postgresql", "host=localhost port=1 sslmode=disable connect_timeout=2")

	require.Error(t, err)
}
