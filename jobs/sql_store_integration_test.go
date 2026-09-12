//go:build integration

package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v4/stdlib"
)

// newDockerSQLStore starts a disposable postgres/mysql container, waits for
// it, applies the schema, and returns a store over it. The tests skip when
// Docker is unavailable; build with -tags integration to enable them (the
// same convention the mailer package uses).
func newDockerSQLStore(t *testing.T, dialect string) *SQLStore {
	t.Helper()
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	if err := pool.Client.Ping(); err != nil {
		t.Skipf("docker not reachable: %v", err)
	}
	pool.MaxWait = 120 * time.Second

	var opts dockertest.RunOptions
	switch dialect {
	case dialectPostgres:
		opts = dockertest.RunOptions{
			Repository:   "postgres",
			Tag:          "16-alpine",
			Env:          []string{"POSTGRES_USER=regius", "POSTGRES_PASSWORD=regius", "POSTGRES_DB=regius_test"},
			ExposedPorts: []string{"5432/tcp"},
		}
	case dialectMySQL:
		opts = dockertest.RunOptions{
			Repository:   "mysql",
			Tag:          "8",
			Env:          []string{"MYSQL_ROOT_PASSWORD=root", "MYSQL_DATABASE=regius_test", "MYSQL_USER=regius", "MYSQL_PASSWORD=regius"},
			ExposedPorts: []string{"3306/tcp"},
		}
	default:
		t.Fatalf("no docker setup for dialect %q", dialect)
	}
	resource, err := pool.RunWithOptions(&opts)
	if err != nil {
		t.Skipf("could not start %s container: %v", dialect, err)
	}
	t.Cleanup(func() { _ = pool.Purge(resource) })

	var db *sql.DB
	if err := pool.Retry(func() error {
		var err error
		var driver, dsn string
		if dialect == dialectPostgres {
			driver = "pgx"
			dsn = fmt.Sprintf("postgres://regius:regius@%s/regius_test?sslmode=disable", resource.GetHostPort("5432/tcp"))
		} else {
			driver = "mysql"
			dsn = fmt.Sprintf("regius:regius@tcp(%s)/regius_test?parseTime=true&multiStatements=true&loc=UTC", resource.GetHostPort("3306/tcp"))
		}
		db, err = sql.Open(driver, dsn)
		if err != nil {
			return err
		}
		return db.Ping()
	}); err != nil {
		t.Fatalf("could not connect to %s: %v", dialect, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := ApplySchema(context.Background(), db, dialect); err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLStore(db, dialect)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestPostgresStore_Matrix exercises the full store matrix against a
// disposable postgres, including the SKIP LOCKED claim under 8 concurrent
// workers.
func TestPostgresStore_Matrix(t *testing.T) {
	runSQLStoreMatrix(t, newDockerSQLStore(t, dialectPostgres))
}

// TestMySQLStore_Matrix exercises the full store matrix against a
// disposable mysql 8, including the SELECT ... FOR UPDATE claim under 8
// concurrent workers.
func TestMySQLStore_Matrix(t *testing.T) {
	runSQLStoreMatrix(t, newDockerSQLStore(t, dialectMySQL))
}
