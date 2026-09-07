package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const initRegiusFixture = `package main

import (
	"log"
	"os"

	"github.com/hbarral/regius"
	"github.com/hbarral/regius/i18n"

	"regius-app/data"
	"regius-app/handlers"
	"regius-app/locales"
	"regius-app/middleware"
)

func initApplication() *application {
	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	reg := &regius.Regius{}
	err = reg.New(path)

	if err != nil {
		log.Fatal(err)
	}

	app := &application{
		App: reg,
	}

	// register background workers here

	return app
}
`

// withJobRoot points the CLI's global backend at a fresh temp dir with an
// init.regius.go fixture, the process env a PersistentPreRun would have
// set (APP_NAME, DATABASE_TYPE), and b.DBType loaded from it.
func withJobRoot(t *testing.T, dbType string) string {
	t.Helper()

	old := b.RootPath
	oldDBType := b.DBType
	dir := t.TempDir()
	b.RootPath = dir
	b.DBType = dbType
	t.Setenv("APP_NAME", "testapp")
	t.Setenv("DATABASE_TYPE", dbType)
	t.Cleanup(func() {
		b.RootPath = old
		b.DBType = oldDBType
	})

	if err := os.WriteFile(filepath.Join(dir, "init.regius.go"), []byte(initRegiusFixture), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_NAME=testapp\nKEY=abc\nDATABASE_TYPE="+dbType+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func readJobFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}

func TestDoMakeJob_FirstRunBootstraps(t *testing.T) {
	dir := withJobRoot(t, "postgres")

	if err := doMakeJob("send-welcome-email"); err != nil {
		t.Fatalf("doMakeJob: %v", err)
	}

	// the handler file, with every placeholder replaced
	job := readJobFile(t, filepath.Join(dir, "workers", "send_welcome_email.go"))
	for _, want := range []string{
		"type SendWelcomeEmailPayload struct",
		"func SendWelcomeEmail(ctx context.Context, j *jobs.Job) error",
		"func EnqueueSendWelcomeEmail(ctx context.Context, m *jobs.Manager, p SendWelcomeEmailPayload) (*jobs.Job, error)",
		`m.Enqueue(ctx, "send_welcome_email", p)`,
	} {
		if !strings.Contains(job, want) {
			t.Errorf("generated job missing %q", want)
		}
	}
	if strings.Contains(job, "$") {
		t.Errorf("generated job still contains unreplaced placeholder:\n%s", job)
	}

	// the registration hub carries the first job
	reg := readJobFile(t, filepath.Join(dir, "workers", "register.go"))
	if !strings.Contains(reg, `m.MustRegister("send_welcome_email", SendWelcomeEmail, jobs.Options{`) {
		t.Errorf("register.go missing the first registration:\n%s", reg)
	}

	// init.regius.go is wired: import + RegisterAll call at the marker
	initGo := readJobFile(t, filepath.Join(dir, "init.regius.go"))
	if !strings.Contains(initGo, "\"testapp/workers\"") {
		t.Errorf("init.regius.go missing workers import:\n%s", initGo)
	}
	if !strings.Contains(initGo, "workers.RegisterAll(app.App.Jobs)") {
		t.Errorf("init.regius.go missing RegisterAll call:\n%s", initGo)
	}

	// the jobs table migration exists for the postgres dialect
	matches, err := filepath.Glob(filepath.Join(dir, "migrations", "*_create_regius_jobs_table.up.sql"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one up migration, got %v (%v)", matches, err)
	}
	up := readJobFile(t, matches[0])
	for _, want := range []string{"CREATE TABLE regius_jobs", "TIMESTAMPTZ", "CREATE TABLE regius_locks"} {
		if !strings.Contains(up, want) {
			t.Errorf("postgres up migration missing %q", want)
		}
	}
	downMatches, err := filepath.Glob(filepath.Join(dir, "migrations", "*_create_regius_jobs_table.down.sql"))
	if err != nil || len(downMatches) != 1 {
		t.Fatalf("expected one down migration, got %v (%v)", downMatches, err)
	}
	if !strings.Contains(readJobFile(t, downMatches[0]), "DROP TABLE IF EXISTS regius_jobs") {
		t.Error("down migration missing the drop statements")
	}
}

func TestDoMakeJob_SecondJobAppends(t *testing.T) {
	dir := withJobRoot(t, "sqlite")

	if err := doMakeJob("send-welcome-email"); err != nil {
		t.Fatalf("first doMakeJob: %v", err)
	}
	if err := doMakeJob("cleanup_expired_sessions"); err != nil {
		t.Fatalf("second doMakeJob: %v", err)
	}

	// the hub carries both registrations, marker kept last
	reg := readJobFile(t, filepath.Join(dir, "workers", "register.go"))
	for _, want := range []string{
		`m.MustRegister("send_welcome_email", SendWelcomeEmail, jobs.Options{`,
		`m.MustRegister("cleanup_expired_sessions", CleanupExpiredSessions, jobs.Options{`,
	} {
		if !strings.Contains(reg, want) {
			t.Errorf("register.go missing %q:\n%s", want, reg)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(reg), "// additional jobs are registered here\n}") {
		t.Errorf("register.go marker is not last:\n%s", reg)
	}

	// no second migration, and the wiring is not duplicated
	matches, _ := filepath.Glob(filepath.Join(dir, "migrations", "*_create_regius_jobs_table.*.sql"))
	if len(matches) != 2 {
		t.Errorf("expected exactly 2 migration files, got %v", matches)
	}
	initGo := readJobFile(t, filepath.Join(dir, "init.regius.go"))
	if strings.Count(initGo, "workers.RegisterAll(app.App.Jobs)") != 1 {
		t.Errorf("RegisterAll call duplicated:\n%s", initGo)
	}
	if strings.Count(initGo, "\"testapp/workers\"") != 1 {
		t.Errorf("workers import duplicated:\n%s", initGo)
	}
}

func TestDoMakeJob_Dialects(t *testing.T) {
	tests := []struct {
		dbType string
		marker string
	}{
		{"postgres", "TIMESTAMPTZ"},
		{"postgresql", "TIMESTAMPTZ"},
		{"mysql", "DATETIME(6)"},
		{"mariadb", "DATETIME(6)"},
		{"sqlite", "INTEGER NOT NULL DEFAULT 0"},
		{"", "INTEGER NOT NULL DEFAULT 0"}, // unset defaults to sqlite
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.dbType, func(t *testing.T) {
			dir := withJobRoot(t, tt.dbType)

			if err := doMakeJob("hook"); err != nil {
				t.Fatalf("doMakeJob: %v", err)
			}
			matches, err := filepath.Glob(filepath.Join(dir, "migrations", "*_create_regius_jobs_table.up.sql"))
			if err != nil || len(matches) != 1 {
				t.Fatalf("expected one up migration, got %v (%v)", matches, err)
			}
			if !strings.Contains(readJobFile(t, matches[0]), tt.marker) {
				t.Errorf("migration for %q missing %q", tt.dbType, tt.marker)
			}
		})
	}
}

func TestDoMakeJob_UnsupportedDialect(t *testing.T) {
	withJobRoot(t, "oracle")

	if err := doMakeJob("hook"); err == nil {
		t.Fatal("expected error for unsupported DATABASE_TYPE")
	} else if !strings.Contains(err.Error(), "unsupported DATABASE_TYPE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoMakeJob_DuplicateName(t *testing.T) {
	withJobRoot(t, "sqlite")

	if err := doMakeJob("hook"); err != nil {
		t.Fatalf("first doMakeJob: %v", err)
	}
	err := doMakeJob("hook")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}

func TestDoMakeJob_MissingAppName(t *testing.T) {
	withJobRoot(t, "sqlite")
	os.Unsetenv("APP_NAME")

	err := doMakeJob("hook")
	if err == nil || !strings.Contains(err.Error(), "APP_NAME") {
		t.Fatalf("expected APP_NAME error, got %v", err)
	}
}

func TestDoMakeJob_NameNormalization(t *testing.T) {
	dir := withJobRoot(t, "sqlite")

	if err := doMakeJob("Send_Welcome-Email"); err != nil {
		t.Fatalf("doMakeJob: %v", err)
	}

	job := readJobFile(t, filepath.Join(dir, "workers", "send_welcome_email.go"))
	if !strings.Contains(job, "func SendWelcomeEmail(ctx context.Context, j *jobs.Job) error") {
		t.Errorf("expected handler SendWelcomeEmail, got:\n%s", job)
	}
	if !strings.Contains(job, `m.Enqueue(ctx, "send_welcome_email", p)`) {
		t.Errorf("expected job name send_welcome_email, got:\n%s", job)
	}
}

func TestWireWorkersRegistration_FallbackAndIdempotent(t *testing.T) {
	noMarker := strings.Replace(initRegiusFixture, "\n\t// register background workers here\n", "", 1)

	tests := []struct {
		name    string
		fixture string
	}{
		{"marker", initRegiusFixture},
		{"fallback", noMarker},
	}

	for _, e := range tests {
		tt := e
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			initPath := filepath.Join(dir, "init.regius.go")
			if err := os.WriteFile(initPath, []byte(tt.fixture), 0644); err != nil {
				t.Fatal(err)
			}

			if err := wireWorkersRegistration(initPath, "testapp"); err != nil {
				t.Fatalf("wireWorkersRegistration: %v", err)
			}
			// second call must be a no-op
			if err := wireWorkersRegistration(initPath, "testapp"); err != nil {
				t.Fatalf("wireWorkersRegistration (2nd): %v", err)
			}

			initGo := readJobFile(t, initPath)
			if strings.Count(initGo, "workers.RegisterAll(app.App.Jobs)") != 1 {
				t.Errorf("expected exactly one RegisterAll call:\n%s", initGo)
			}
			if !strings.Contains(initGo, "\"testapp/workers\"") {
				t.Errorf("missing workers import:\n%s", initGo)
			}
		})
	}
}

func TestAppendRegistration_MarkerMissing(t *testing.T) {
	dir := t.TempDir()
	registerPath := filepath.Join(dir, "register.go")
	hub := `package workers

import (
	"github.com/hbarral/regius/jobs"
)

func RegisterAll(m *jobs.Manager) {
	m.MustRegister("first", First, jobs.Options{
		MaxAttempts: 3,
	})
}
`
	if err := os.WriteFile(registerPath, []byte(hub), 0644); err != nil {
		t.Fatal(err)
	}

	if err := appendRegistration(registerPath, "second", "Second"); err != nil {
		t.Fatalf("appendRegistration: %v", err)
	}
	// idempotent on repeat
	if err := appendRegistration(registerPath, "second", "Second"); err != nil {
		t.Fatalf("appendRegistration (2nd): %v", err)
	}

	reg := readJobFile(t, registerPath)
	if strings.Count(reg, `m.MustRegister("second", Second, jobs.Options{`) != 1 {
		t.Errorf("expected exactly one appended registration:\n%s", reg)
	}
	if !strings.HasSuffix(strings.TrimSpace(reg), "}") {
		t.Errorf("register.go lost its closing brace:\n%s", reg)
	}
}
