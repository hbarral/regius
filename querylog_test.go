package regius

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryLogging_LogsQueries(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	setQueryLogLogger(logger)
	t.Cleanup(func() { setQueryLogLogger(nil) })

	t.Setenv("DATABASE_QUERY_LOGGING", "true")

	r := &Regius{}
	db, err := r.OpenDB("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE querylog_test (id INTEGER)")
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "CREATE TABLE querylog_test")
	assert.Contains(t, output, "[DB]")
}

func TestQueryLogging_DisabledByDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	setQueryLogLogger(logger)
	t.Cleanup(func() { setQueryLogLogger(nil) })

	t.Setenv("DATABASE_QUERY_LOGGING", "false")

	r := &Regius{}
	db, err := r.OpenDB("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE disabled_test (id INTEGER)")
	require.NoError(t, err)

	assert.Empty(t, buf.String())
}

func TestValueToString(t *testing.T) {
	assert.Equal(t, "NULL", valueToString(nil))
	assert.Equal(t, "'hello'", valueToString("hello"))
	assert.Equal(t, "'world'", valueToString([]byte("world")))
	assert.Equal(t, "42", valueToString(int64(42)))
}
