package regius

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestCreateHashConfig(t *testing.T) {
	t.Setenv("HASH_ALGORITHM", "argon2")
	t.Setenv("HASH_COST", "12")
	t.Setenv("HASH_SCRYPT_N", "32768")
	t.Setenv("HASH_SCRYPT_R", "8")
	t.Setenv("HASH_SCRYPT_P", "1")
	t.Setenv("HASH_ARGON2_MEMORY", "65536")
	t.Setenv("HASH_ARGON2_ITERATIONS", "3")
	t.Setenv("HASH_ARGON2_PARALLELISM", "2")

	r := &Regius{}
	cfg := r.createHashConfig()

	assert.Equal(t, "argon2", cfg.algorithm)
	assert.Equal(t, 12, cfg.cost)
	assert.Equal(t, 32768, cfg.scryptN)
	assert.Equal(t, 8, cfg.scryptR)
	assert.Equal(t, 1, cfg.scryptP)
	assert.Equal(t, uint32(65536), cfg.argon2Memory)
	assert.Equal(t, uint32(3), cfg.argon2Iterations)
	assert.Equal(t, uint8(2), cfg.argon2Parallelism)
}

func TestCreateHashConfig_DefaultsWhenUnset(t *testing.T) {
	for _, k := range []string{
		"HASH_ALGORITHM", "HASH_COST", "HASH_SCRYPT_N", "HASH_SCRYPT_R",
		"HASH_SCRYPT_P", "HASH_ARGON2_MEMORY", "HASH_ARGON2_ITERATIONS",
		"HASH_ARGON2_PARALLELISM",
	} {
		t.Setenv(k, "")
	}

	r := &Regius{}
	cfg := r.createHashConfig()

	assert.Equal(t, "", cfg.algorithm)
	assert.Equal(t, 0, cfg.cost)
	assert.Equal(t, uint32(0), cfg.argon2Memory)
	assert.Equal(t, uint8(0), cfg.argon2Parallelism)
}

func TestCreateHashConfig_InvalidValues(t *testing.T) {
	t.Setenv("HASH_COST", "not-a-number")
	t.Setenv("HASH_ARGON2_MEMORY", "abc")
	t.Setenv("HASH_ARGON2_PARALLELISM", "xyz")

	r := &Regius{}
	cfg := r.createHashConfig()

	// strconv parse errors are silently ignored -> zero values.
	assert.Equal(t, 0, cfg.cost)
	assert.Equal(t, uint32(0), cfg.argon2Memory)
	assert.Equal(t, uint8(0), cfg.argon2Parallelism)
}

func TestCreateMailer(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.com")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "user")
	t.Setenv("SMTP_PASSWORD", "pass")
	t.Setenv("SMTP_ENCRYPTION", "tls")
	t.Setenv("FROM_NAME", "Regius")
	t.Setenv("FROM_ADDRESS", "noreply@example.com")
	t.Setenv("MAILER_API", "sendgrid")
	t.Setenv("MAILER_KEY", "key123")
	t.Setenv("MAILER_URL", "https://api.sendgrid.com")

	r := &Regius{RootPath: "/app/root"}
	m := r.createMailer()

	assert.Equal(t, "example.com", m.Domain)
	assert.Equal(t, "/app/root/mail", m.Templates)
	assert.Equal(t, "smtp.example.com", m.Host)
	assert.Equal(t, 587, m.Port)
	assert.Equal(t, "user", m.Username)
	assert.Equal(t, "pass", m.Password)
	assert.Equal(t, "tls", m.Encryption)
	assert.Equal(t, "Regius", m.FromName)
	assert.Equal(t, "noreply@example.com", m.FromAddress)
	assert.Equal(t, "sendgrid", m.API)
	assert.Equal(t, "key123", m.APIKey)
	assert.Equal(t, "https://api.sendgrid.com", m.APIUrl)
	assert.NotNil(t, m.Jobs)
	assert.NotNil(t, m.Results)
	assert.Equal(t, 20, cap(m.Jobs))
	assert.Equal(t, 20, cap(m.Results))
}

func TestCreateFileSystems_NoneConfigured(t *testing.T) {
	for _, k := range []string{"MINIO_SECRET", "SFTP_HOST", "WEBDAV_HOST", "S3_KEY"} {
		t.Setenv(k, "")
	}

	r := &Regius{}
	fs := r.createFileSystems()

	assert.Empty(t, fs)
}

func TestCreateFileSystems_AllConfigured(t *testing.T) {
	t.Setenv("MINIO_SECRET", "minio-secret")
	t.Setenv("MINIO_ENDPOINT", "minio.local:9000")
	t.Setenv("MINIO_KEY", "minio-key")
	t.Setenv("MINIO_USESSL", "true")
	t.Setenv("MINIO_REGION", "us-east-1")
	t.Setenv("MINIO_BUCKET", "bucket")

	t.Setenv("SFTP_HOST", "sftp.local")
	t.Setenv("SFTP_PORT", "22")
	t.Setenv("SFTP_USER", "u")
	t.Setenv("SFTP_PASS", "p")

	t.Setenv("WEBDAV_HOST", "webdav.local")
	t.Setenv("WEBDAV_PORT", "8080")
	t.Setenv("WEBDAV_USER", "wu")
	t.Setenv("WEBDAV_PASS", "wp")
	t.Setenv("WEBDAV_USESSL", "false")

	t.Setenv("S3_KEY", "s3-key")
	t.Setenv("S3_SECRET", "s3-secret")
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("S3_BUCKET", "s3bucket")
	t.Setenv("S3_ENDPOINT", "https://s3.local")

	r := &Regius{}
	fs := r.createFileSystems()

	assert.Len(t, fs, 4)
	assert.Contains(t, fs, "MINIO")
	assert.Contains(t, fs, "SFTP")
	assert.Contains(t, fs, "WebDAV")
	assert.Contains(t, fs, "S3")

	// Receiver fields are populated (spot-check S3 + Minio/WebDAV SSL flags).
	assert.Equal(t, "s3-key", r.S3.Key)
	assert.Equal(t, "s3-secret", r.S3.Secret)
	assert.Equal(t, "s3bucket", r.S3.Bucket)
	assert.Equal(t, "minio-secret", r.Minio.Secret)
	assert.True(t, r.Minio.UseSSL)
	assert.False(t, r.WebDAV.UseSSL)
}

func TestRPCServer_MaintenanceMode(t *testing.T) {
	// maintenanceMode is a package-global; reset before/after so the test is
	// isolated from order.
	original := maintenanceMode
	maintenanceMode = false
	t.Cleanup(func() { maintenanceMode = original })

	srv := &RPCServer{}

	var resp string
	require.NoError(t, srv.MaintenanceMode(true, &resp))
	assert.True(t, maintenanceMode)
	assert.Equal(t, "Server in maintenance mode", resp)

	require.NoError(t, srv.MaintenanceMode(false, &resp))
	assert.False(t, maintenanceMode)
	assert.Equal(t, "Server live!", resp)
}

func TestInit_CreatesFolders(t *testing.T) {
	r := &Regius{}
	root := t.TempDir()

	err := r.Init(initPath{
		rootPath:    root,
		folderNames: []string{"handlers", "migrations", "views"},
	})
	require.NoError(t, err)

	for _, dir := range []string{"handlers", "migrations", "views"} {
		info, err := os.Stat(filepath.Join(root, dir))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	}
}

func TestCheckDotEnv_CreatesFile(t *testing.T) {
	r := &Regius{}
	dir := t.TempDir()

	require.NoError(t, r.checkDotEnv(dir))

	_, err := os.Stat(filepath.Join(dir, ".env"))
	assert.NoError(t, err, ".env should be created")
}

// Note: checkDotEnv itself surfaces the create error correctly. The related
// bug lives in New() (regius.go), which calls checkDotEnv but returns nil on
// error instead of the error; New() is not unit-testable without a refactor.
func TestCheckDotEnv_NonexistentDir_ReturnsError(t *testing.T) {
	r := &Regius{}
	missing := filepath.Join(t.TempDir(), "does_not_exist")

	err := r.checkDotEnv(missing)

	require.Error(t, err, "checkDotEnv must surface the underlying create error")

	_, statErr := os.Stat(filepath.Join(missing, ".env"))
	assert.Error(t, statErr, "no .env should have been created")
}
