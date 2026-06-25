package regius

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseStringSliceEnv(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		setEnv       bool
		defaultValue string
		want         []string
	}{
		{
			name:         "empty env uses default list",
			setEnv:       true,
			envValue:     "",
			defaultValue: "a,b,c",
			want:         []string{"a", "b", "c"},
		},
		{
			name:         "env overrides default",
			setEnv:       true,
			envValue:     "x,y,z",
			defaultValue: "a,b,c",
			want:         []string{"x", "y", "z"},
		},
		{
			name:         "values are trimmed",
			setEnv:       true,
			envValue:     " x , y , z ",
			defaultValue: "",
			want:         []string{"x", "y", "z"},
		},
		{
			name:         "single value",
			setEnv:       true,
			envValue:     "solo",
			defaultValue: "",
			want:         []string{"solo"},
		},
		{
			name:         "empty env and empty default returns empty slice",
			setEnv:       true,
			envValue:     "",
			defaultValue: "",
			want:         []string{},
		},
		{
			name:         "default with empty entries is filtered",
			setEnv:       true,
			envValue:     ", ,",
			defaultValue: "",
			want:         []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RSS_TEST_KEY", tt.envValue)
			got := parseStringSliceEnv("RSS_TEST_KEY", tt.defaultValue)
			if len(tt.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseStringSliceEnv_DefaultWhenUnset(t *testing.T) {
	// An env var that is genuinely unset falls back to the default value.
	got := parseStringSliceEnv("RSS_DEFINITELY_UNSET_KEY", "one,two")
	assert.Equal(t, []string{"one", "two"}, got)
}

func TestBuildDSN_PostgresWithPassword(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgres")
	t.Setenv("DATABASE_HOST", "dbhost")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "dbuser")
	t.Setenv("DATABASE_NAME", "appdb")
	t.Setenv("DATABASE_SSL_MODE", "disable")
	t.Setenv("DATABASE_PASS", "s3cr3t")

	r := &Regius{}
	dsn := r.BuildDSN()

	assert.Contains(t, dsn, "host=dbhost")
	assert.Contains(t, dsn, "port=5432")
	assert.Contains(t, dsn, "user=dbuser")
	assert.Contains(t, dsn, "dbname=appdb")
	assert.Contains(t, dsn, "sslmode=disable")
	assert.Contains(t, dsn, "timezone=UTC")
	assert.Contains(t, dsn, "connect_timeout=5")
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
	dsn := r.BuildDSN()

	assert.Contains(t, dsn, "host=dbhost")
	assert.NotContains(t, dsn, "password=", "no password= segment when DATABASE_PASS is empty")
}

func TestBuildDSN_PostgresqlAlias(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgresql")
	r := &Regius{}
	assert.NotEmpty(t, r.BuildDSN())
}

func TestBuildDSN_EmptyType(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "")
	r := &Regius{}
	assert.Empty(t, r.BuildDSN())
}

func TestBuildDSN_UnsupportedType(t *testing.T) {
	// TODO: bug — BuildDSN only supports postgres; mysql falls through to the
	// empty default case. Pinned here so the gap is visible. See
	// things_to_improve.md "Migration: BuildDSN only supports PostgreSQL".
	t.Setenv("DATABASE_TYPE", "mysql")
	r := &Regius{}
	assert.Empty(t, r.BuildDSN())
}

func TestStartLoggers(t *testing.T) {
	r := &Regius{}

	infoLog, errorLog := r.startLoggers()

	assert.NotNil(t, infoLog)
	assert.NotNil(t, errorLog)
}
