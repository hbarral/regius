package regius

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeeder_RunSeeds(t *testing.T) {
	db := openSQLiteDB(t)
	root := t.TempDir()
	seedsDir := filepath.Join(root, "seeds")
	require.NoError(t, os.MkdirAll(seedsDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(seedsDir, "001_create.sql"), []byte(
		"CREATE TABLE seeded (id INTEGER PRIMARY KEY, name TEXT);",
	), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(seedsDir, "002_insert.sql"), []byte(
		"INSERT INTO seeded (name) VALUES ('hello');",
	), 0644))

	seeder := &Seeder{DB: Database{Pool: db}, RootPath: root}
	require.NoError(t, seeder.RunSeeds())

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM seeded WHERE name = ?", "hello").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestSeeder_RunSeeds_Idempotent(t *testing.T) {
	db := openSQLiteDB(t)
	root := t.TempDir()
	seedsDir := filepath.Join(root, "seeds")
	require.NoError(t, os.MkdirAll(seedsDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(seedsDir, "001_insert.sql"), []byte(
		"CREATE TABLE IF NOT EXISTS seeded2 (id INTEGER PRIMARY KEY); INSERT INTO seeded2 (id) VALUES (1);",
	), 0644))

	seeder := &Seeder{DB: Database{Pool: db}, RootPath: root}
	require.NoError(t, seeder.RunSeeds())
	require.NoError(t, seeder.RunSeeds())

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM seeded2").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestSeeder_RunSeeds_NilPool(t *testing.T) {
	seeder := &Seeder{DB: Database{}, RootPath: t.TempDir()}
	err := seeder.RunSeeds()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool is nil")
}

func TestSeeder_RunSeeds_BadSQL(t *testing.T) {
	db := openSQLiteDB(t)
	root := t.TempDir()
	seedsDir := filepath.Join(root, "seeds")
	require.NoError(t, os.MkdirAll(seedsDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(seedsDir, "001_bad.sql"), []byte(
		"THIS IS NOT VALID SQL",
	), 0644))

	seeder := &Seeder{DB: Database{Pool: db}, RootPath: root}
	err := seeder.RunSeeds()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run seed")
}

func TestNewSeeder(t *testing.T) {
	db := openSQLiteDB(t)
	r := &Regius{
		DB:       Database{Pool: db},
		RootPath: "/app",
	}

	seeder := r.NewSeeder()
	require.NotNil(t, seeder)
	assert.Equal(t, "/app", seeder.RootPath)
	assert.Equal(t, db, seeder.DB.Pool)
}
