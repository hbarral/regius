package regius

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	return db
}

func TestDatabase_ConfigurePool(t *testing.T) {
	t.Setenv("DATABASE_MAX_OPEN_CONNS", "10")
	t.Setenv("DATABASE_MAX_IDLE_CONNS", "5")
	t.Setenv("DATABASE_CONN_MAX_LIFETIME", "15m")

	d := &Database{Pool: openSQLiteDB(t)}
	require.NoError(t, d.ConfigurePool())

	assert.Equal(t, 10, d.Pool.Stats().MaxOpenConnections)
}

func TestDatabase_ConfigurePool_InvalidMaxOpenConns(t *testing.T) {
	t.Setenv("DATABASE_MAX_OPEN_CONNS", "not-a-number")

	d := &Database{Pool: openSQLiteDB(t)}
	err := d.ConfigurePool()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_MAX_OPEN_CONNS")
}

func TestDatabase_ConfigurePool_InvalidMaxIdleConns(t *testing.T) {
	t.Setenv("DATABASE_MAX_IDLE_CONNS", "not-a-number")

	d := &Database{Pool: openSQLiteDB(t)}
	err := d.ConfigurePool()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_MAX_IDLE_CONNS")
}

func TestDatabase_ConfigurePool_InvalidConnMaxLifetime(t *testing.T) {
	t.Setenv("DATABASE_CONN_MAX_LIFETIME", "not-a-duration")

	d := &Database{Pool: openSQLiteDB(t)}
	err := d.ConfigurePool()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_CONN_MAX_LIFETIME")
}

func TestDatabase_ConfigurePool_NilPool(t *testing.T) {
	d := &Database{}
	err := d.ConfigurePool()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool is nil")
}

func TestDatabase_HealthCheck(t *testing.T) {
	d := &Database{Pool: openSQLiteDB(t)}

	err := d.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestDatabase_HealthCheck_NilPool(t *testing.T) {
	d := &Database{}

	err := d.HealthCheck(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool is nil")
}

func TestDatabase_HealthCheck_ClosedPool(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	d := &Database{Pool: db}
	err = d.HealthCheck(context.Background())
	assert.Error(t, err)
}

func TestDatabase_Transaction_Commit(t *testing.T) {
	d := &Database{Pool: openSQLiteDB(t)}

	_, err := d.Pool.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	err = d.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (name) VALUES (?)", "committed")
		return err
	})

	require.NoError(t, err)

	var count int
	require.NoError(t, d.Pool.QueryRow("SELECT COUNT(*) FROM test WHERE name = ?", "committed").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestDatabase_Transaction_Rollback(t *testing.T) {
	d := &Database{Pool: openSQLiteDB(t)}

	_, err := d.Pool.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	err = d.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (name) VALUES (?)", "rolled-back")
		if err != nil {
			return err
		}
		return assert.AnError
	})

	require.Error(t, err)

	var count int
	require.NoError(t, d.Pool.QueryRow("SELECT COUNT(*) FROM test WHERE name = ?", "rolled-back").Scan(&count))
	assert.Equal(t, 0, count)
}

func TestDatabase_Transaction_NilPool(t *testing.T) {
	d := &Database{}

	err := d.Transaction(context.Background(), func(tx *sql.Tx) error {
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool is nil")
}

func TestRegius_Transaction(t *testing.T) {
	db := openSQLiteDB(t)
	_, err := db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	r := &Regius{DB: Database{Pool: db}}

	err = r.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (name) VALUES (?)", "via-regius")
		return err
	})

	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM test WHERE name = ?", "via-regius").Scan(&count))
	assert.Equal(t, 1, count)
}
