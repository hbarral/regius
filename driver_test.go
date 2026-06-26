package regius

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
