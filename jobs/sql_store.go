package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Dialect identifiers accepted (and normalized) by NewSQLStore and Schema.
// Aliases like "postgresql", "pgx", "mariadb", and "sqlite3" are allowed.
const (
	dialectPostgres = "postgres"
	dialectMySQL    = "mysql"
	dialectSQLite   = "sqlite"
)

const jobCols = "id, name, payload, queue, status, attempts, max_attempts, run_at, lease_until, last_error, created_at, updated_at, completed_at"

// postgresSchema is the postgres DDL for the job tables.
const postgresSchema = `CREATE TABLE regius_jobs (
    id           VARCHAR(20) PRIMARY KEY,
    name         VARCHAR(255) NOT NULL,
    payload      JSONB NOT NULL,
    queue        VARCHAR(100) NOT NULL DEFAULT 'default',
    status       VARCHAR(16) NOT NULL,
    attempts     INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    run_at       TIMESTAMPTZ NOT NULL,
    lease_until  TIMESTAMPTZ NULL,
    last_error   TEXT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NULL
);

CREATE INDEX regius_jobs_claim_idx ON regius_jobs (status, run_at);

CREATE INDEX regius_jobs_completed_idx ON regius_jobs (status, completed_at);

CREATE TABLE regius_locks (
    name       VARCHAR(191) PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
)`

// mysqlSchema is the MySQL DDL for the job tables. DATETIME(6) keeps
// microsecond precision; the locks primary key stays under the utf8mb4
// index limit of older MySQL 5.7 InnoDB.
const mysqlSchema = `CREATE TABLE regius_jobs (
    id           VARCHAR(20) NOT NULL,
    name         VARCHAR(255) NOT NULL,
    payload      JSON NOT NULL,
    queue        VARCHAR(100) NOT NULL DEFAULT 'default',
    status       VARCHAR(16) NOT NULL,
    attempts     INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    run_at       DATETIME(6) NOT NULL,
    lease_until  DATETIME(6) NULL,
    last_error   TEXT NULL,
    created_at   DATETIME(6) NOT NULL,
    updated_at   DATETIME(6) NOT NULL,
    completed_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    INDEX regius_jobs_claim_idx (status, run_at),
    INDEX regius_jobs_completed_idx (status, completed_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE regius_locks (
    name       VARCHAR(191) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    PRIMARY KEY (name)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4`

// sqliteSchema is the SQLite DDL for the job tables. Time columns are
// declared "timestamp" so the driver converts them to time.Time on scan.
const sqliteSchema = `CREATE TABLE regius_jobs (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    payload      TEXT NOT NULL,
    queue        TEXT NOT NULL DEFAULT 'default',
    status       TEXT NOT NULL,
    attempts     INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    run_at       timestamp NOT NULL,
    lease_until  timestamp NULL,
    last_error   TEXT NULL,
    created_at   timestamp NOT NULL,
    updated_at   timestamp NOT NULL,
    completed_at timestamp NULL
);

CREATE INDEX regius_jobs_claim_idx ON regius_jobs (status, run_at);

CREATE INDEX regius_jobs_completed_idx ON regius_jobs (status, completed_at);

CREATE TABLE regius_locks (
    name       TEXT PRIMARY KEY,
    expires_at timestamp NOT NULL
)`

func normalizeDialect(dialect string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "postgres", "postgresql", "pg", "pgx":
		return dialectPostgres, nil
	case "mysql", "mariadb":
		return dialectMySQL, nil
	case "sqlite", "sqlite3":
		return dialectSQLite, nil
	default:
		return "", fmt.Errorf("jobs: unsupported sql dialect %q (want postgres, mysql, or sqlite)", dialect)
	}
}

// Schema returns the DDL for the given dialect: the regius_jobs and
// regius_locks tables plus their indexes. It matches the migrations
// scaffolded by `regius make job`; ApplySchema is the portable way to run
// it. Production apps should use the migrations.
func Schema(dialect string) (string, error) {
	d, err := normalizeDialect(dialect)
	if err != nil {
		return "", err
	}
	switch d {
	case dialectPostgres:
		return postgresSchema, nil
	case dialectMySQL:
		return mysqlSchema, nil
	default:
		return sqliteSchema, nil
	}
}

// ApplySchema executes the dialect's DDL statement by statement over db. It
// is a convenience for tests and dev setups; it does not replace the
// scaffolded migrations.
func ApplySchema(ctx context.Context, db *sql.DB, dialect string) error {
	schema, err := Schema(dialect)
	if err != nil {
		return err
	}
	for _, stmt := range strings.Split(schema, ";") {
		if stmt = strings.TrimSpace(stmt); stmt != "" {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("jobs: apply schema: %w", err)
			}
		}
	}
	return nil
}

// SQLStore is a Store over Go's database/sql, supporting postgres, mysql,
// and sqlite (the framework's DATABASE_* wiring — all three drivers are
// registered by the framework's driver.go). The schema must exist already
// (scaffolded migration, or jobs.ApplySchema); the store never runs DDL at
// runtime.
//
// Claiming is dialect-tuned: postgres claims in a single UPDATE ... FOR
// UPDATE SKIP LOCKED ... RETURNING statement; mysql claims inside a
// transaction with SELECT ... FOR UPDATE (works on both MySQL 5.7 and 8);
// sqlite uses a guarded select-then-update retry (single writer). All
// conditional transitions re-check the expected status, so terminal
// transitions happen exactly once.
//
// All timestamps are stored in UTC and round-trip with at least microsecond
// precision (postgres TIMESTAMPTZ, mysql DATETIME(6), sqlite nanosecond
// text).
type SQLStore struct {
	// DB is the database/sql pool.
	DB *sql.DB

	// dialect is the normalized dialect (postgres | mysql | sqlite).
	dialect string

	// Now is the clock used by TryLock and Retry. nil means time.Now. It
	// exists for deterministic tests.
	Now func() time.Time
}

// NewSQLStore creates a store over db for the given dialect
// (postgres/mysql/sqlite, aliases accepted).
func NewSQLStore(db *sql.DB, dialect string) (*SQLStore, error) {
	d, err := normalizeDialect(dialect)
	if err != nil {
		return nil, err
	}
	return &SQLStore{DB: db, dialect: d}, nil
}

func (s *SQLStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// utc normalizes timestamps so comparisons (sqlite stores them as text)
// and round-trips stay consistent across dialects.
func utc(t time.Time) time.Time {
	return t.UTC()
}

// ph returns the SQL placeholder for 1-based argument n.
func (s *SQLStore) ph(n int) string {
	if s.dialect == dialectPostgres {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

// phList returns n placeholders for a VALUES clause.
func (s *SQLStore) phList(n int) string {
	if s.dialect != dialectPostgres {
		return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "$" + strconv.Itoa(i+1)
	}
	return strings.Join(parts, ", ")
}

// rowQuerier is implemented by both *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// isDuplicateKey reports whether err is a unique-constraint violation on
// any of the three dialects, by matching the driver error text (the store
// deliberately does not import driver packages).
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value violates unique constraint") || // postgres 23505
		strings.Contains(msg, "Duplicate entry") || // mysql 1062
		strings.Contains(msg, "UNIQUE constraint failed") // sqlite
}

// isDeadlock reports whether err is an InnoDB deadlock (mysql error 1213),
// which is always safe to retry.
func isDeadlock(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Deadlock found when trying to get lock")
}

// scanJobRow fills a Job from one row of jobCols.
func scanJobRow(scan func(dest ...any) error) (*Job, error) {
	var (
		j         Job
		lease     sql.NullTime
		lastErr   sql.NullString
		completed sql.NullTime
	)
	err := scan(
		&j.ID, &j.Name, &j.Payload, &j.Queue, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.RunAt, &lease, &lastErr,
		&j.CreatedAt, &j.UpdatedAt, &completed,
	)
	if err != nil {
		return nil, err
	}
	j.LeaseUntil = lease.Time
	j.LastError = lastErr.String
	j.CompletedAt = completed.Time
	return &j, nil
}

// Enqueue inserts the job as a new pending row.
func (s *SQLStore) Enqueue(ctx context.Context, j *Job) error {
	payload := j.Payload
	if payload == nil {
		payload = []byte("null")
	}
	q := fmt.Sprintf(`INSERT INTO regius_jobs
(id, name, payload, queue, status, attempts, max_attempts, run_at, lease_until, last_error, created_at, updated_at, completed_at)
VALUES (%s)`, s.phList(13))
	_, err := s.DB.ExecContext(ctx, q,
		j.ID, j.Name, payload, j.Queue, string(j.Status), j.Attempts, j.MaxAttempts,
		utc(j.RunAt), nil, nil, utc(j.CreatedAt), utc(j.UpdatedAt), nil)
	return err
}

// Claim atomically claims the oldest ready pending job, dialect-tuned (see
// the SQLStore doc).
func (s *SQLStore) Claim(ctx context.Context, now, leaseUntil time.Time) (*Job, error) {
	switch s.dialect {
	case dialectPostgres:
		return s.claimPostgres(ctx, now, leaseUntil)
	case dialectMySQL:
		return s.claimMySQL(ctx, now, leaseUntil)
	default:
		return s.claimSQLite(ctx, now, leaseUntil)
	}
}

func (s *SQLStore) claimPostgres(ctx context.Context, now, leaseUntil time.Time) (*Job, error) {
	q := fmt.Sprintf(`UPDATE regius_jobs
SET status = 'running', lease_until = %[1]s, attempts = attempts + 1, updated_at = %[2]s
WHERE id = (
	SELECT id FROM regius_jobs
	WHERE status = 'pending' AND run_at <= %[3]s
	ORDER BY run_at
	LIMIT 1
	FOR UPDATE SKIP LOCKED
)
RETURNING %s`, s.ph(1), s.ph(2), s.ph(3), jobCols)
	j, err := scanJobRow(s.DB.QueryRowContext(ctx, q, utc(leaseUntil), utc(now), utc(now)).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoJob
	}
	return j, err
}

func (s *SQLStore) claimMySQL(ctx context.Context, now, leaseUntil time.Time) (*Job, error) {
	// InnoDB deadlocks (error 1213) are transient by definition; retry the
	// whole transaction a few times before surfacing the error.
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		var j *Job
		j, err = s.claimMySQLOnce(ctx, now, leaseUntil)
		if !isDeadlock(err) {
			return j, err
		}
	}
	return nil, err
}

func (s *SQLStore) claimMySQLOnce(ctx context.Context, now, leaseUntil time.Time) (*Job, error) {
	// SELECT ... FOR UPDATE either locks the oldest ready row or blocks on
	// it until the holder's transaction commits, then re-scans (current
	// read) and locks the next one — correct on MySQL 5.7 and 8 alike. The
	// claim runs at READ COMMITTED so the (status, run_at) index scan takes
	// record locks only (no gap locks), which keeps concurrent claimers
	// locking rows in a consistent order instead of deadlocking.
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id string
	selQ := fmt.Sprintf(`SELECT id FROM regius_jobs
WHERE status = 'pending' AND run_at <= %s
ORDER BY run_at
LIMIT 1
FOR UPDATE`, s.ph(1))
	err = tx.QueryRowContext(ctx, selQ, utc(now)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoJob
	}
	if err != nil {
		return nil, err
	}

	updQ := fmt.Sprintf(`UPDATE regius_jobs
SET status = 'running', lease_until = %[1]s, attempts = attempts + 1, updated_at = %[2]s
WHERE id = %[3]s AND status = 'pending'`, s.ph(1), s.ph(2), s.ph(3))
	res, err := tx.ExecContext(ctx, updQ, utc(leaseUntil), utc(now), id)
	if err != nil {
		return nil, err
	}
	if n, err := intAffected(res); err != nil || n == 0 {
		if err != nil {
			return nil, err
		}
		return nil, ErrNoJob
	}

	j, err := s.getJob(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return j, nil
}

func (s *SQLStore) claimSQLite(ctx context.Context, now, leaseUntil time.Time) (*Job, error) {
	// No row locks in sqlite: select the oldest ready id, then claim it
	// with a guarded update; if another connection won the race, retry
	// with the next candidate (writes serialize on the database lock, so
	// contention is rare).
	selQ := fmt.Sprintf(`SELECT id FROM regius_jobs
WHERE status = 'pending' AND run_at <= %s
ORDER BY run_at
LIMIT 1`, s.ph(1))
	updQ := fmt.Sprintf(`UPDATE regius_jobs
SET status = 'running', lease_until = %[1]s, attempts = attempts + 1, updated_at = %[2]s
WHERE id = %[3]s AND status = 'pending'`, s.ph(1), s.ph(2), s.ph(3))
	for attempt := 0; attempt < 3; attempt++ {
		var id string
		err := s.DB.QueryRowContext(ctx, selQ, utc(now)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoJob
		}
		if err != nil {
			return nil, err
		}
		res, err := s.DB.ExecContext(ctx, updQ, utc(leaseUntil), utc(now), id)
		if err != nil {
			return nil, err
		}
		if n, err := intAffected(res); err != nil {
			return nil, err
		} else if n == 1 {
			return s.Get(ctx, id)
		}
	}
	return nil, ErrNoJob
}

// Complete marks a running job completed.
func (s *SQLStore) Complete(ctx context.Context, id string, now time.Time) error {
	q := fmt.Sprintf(`UPDATE regius_jobs
SET status = 'completed', completed_at = %[1]s, updated_at = %[2]s, lease_until = NULL
WHERE id = %[3]s AND status = 'running'`, s.ph(1), s.ph(2), s.ph(3))
	return s.affected(ctx, q, utc(now), utc(now), id)
}

// Fail records a failed attempt on a running job: a non-zero retryAt
// requeues it as pending with RunAt = retryAt; a zero retryAt dead-letters
// it.
func (s *SQLStore) Fail(ctx context.Context, j *Job, now, retryAt time.Time, lastErr string) error {
	if retryAt.IsZero() {
		q := fmt.Sprintf(`UPDATE regius_jobs
SET status = 'dead', last_error = %[1]s, updated_at = %[2]s, lease_until = NULL
WHERE id = %[3]s AND status = 'running'`, s.ph(1), s.ph(2), s.ph(3))
		return s.affected(ctx, q, lastErr, utc(now), j.ID)
	}
	q := fmt.Sprintf(`UPDATE regius_jobs
SET status = 'pending', run_at = %[1]s, last_error = %[2]s, updated_at = %[3]s, lease_until = NULL
WHERE id = %[4]s AND status = 'running'`, s.ph(1), s.ph(2), s.ph(3), s.ph(4))
	return s.affected(ctx, q, utc(retryAt), lastErr, utc(now), j.ID)
}

// ReclaimExpired returns running jobs whose lease expired to pending.
func (s *SQLStore) ReclaimExpired(ctx context.Context, now time.Time) (int, error) {
	q := fmt.Sprintf(`UPDATE regius_jobs
SET status = 'pending', run_at = %[1]s, updated_at = %[2]s, lease_until = NULL
WHERE status = 'running' AND lease_until IS NOT NULL AND lease_until <= %[3]s`, s.ph(1), s.ph(2), s.ph(3))
	res, err := s.DB.ExecContext(ctx, q, utc(now), utc(now), utc(now))
	if err != nil {
		return 0, err
	}
	return intAffected(res)
}

// TryLock acquires a best-effort exclusive lock with a TTL, backed by the
// regius_locks table: it takes over an expired row, or inserts a fresh one.
func (s *SQLStore) TryLock(ctx context.Context, name string, ttl time.Duration) (bool, error) {
	now := s.now()
	expires := utc(now.Add(ttl))

	takeQ := fmt.Sprintf(`UPDATE regius_locks SET expires_at = %[1]s WHERE name = %[2]s AND expires_at <= %[3]s`,
		s.ph(1), s.ph(2), s.ph(3))
	res, err := s.DB.ExecContext(ctx, takeQ, expires, name, utc(now))
	if err != nil {
		return false, err
	}
	if n, err := intAffected(res); err == nil && n == 1 {
		return true, nil
	}

	insertQ := fmt.Sprintf(`INSERT INTO regius_locks (name, expires_at) VALUES (%[1]s, %[2]s)`, s.ph(1), s.ph(2))
	if _, err := s.DB.ExecContext(ctx, insertQ, name, expires); err == nil {
		return true, nil
	} else if isDuplicateKey(err) {
		return false, nil
	} else {
		return false, err
	}
}

// Stats returns job counts by status.
func (s *SQLStore) Stats(ctx context.Context) (Stats, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT
	COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN status = 'dead' THEN 1 ELSE 0 END), 0)
	FROM regius_jobs`)
	var st Stats
	if err := row.Scan(&st.Pending, &st.Running, &st.Completed, &st.Dead); err != nil {
		return Stats{}, err
	}
	return st, nil
}

// List returns jobs matching the filter: pending and running ordered by
// RunAt, then completed and dead newest first. Each status bucket scans up
// to listScanLimit rows before the limit is applied.
func (s *SQLStore) List(ctx context.Context, f ListFilter) ([]*Job, error) {
	buckets := []sqlBucket{
		{string(StatusPending), "run_at", "ASC"},
		{string(StatusRunning), "run_at", "ASC"},
		{string(StatusCompleted), "completed_at", "DESC"},
		{string(StatusDead), "updated_at", "DESC"},
	}
	var out []*Job
	for _, b := range buckets {
		if f.Status != "" && string(f.Status) != b.status {
			continue
		}
		jobs, err := s.listBucket(ctx, b, f.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, jobs...)
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

type sqlBucket struct {
	status string
	col    string
	dir    string
}

func (s *SQLStore) listBucket(ctx context.Context, b sqlBucket, name string) ([]*Job, error) {
	var q string
	var args []any
	if name != "" {
		q = fmt.Sprintf(`SELECT %[1]s FROM regius_jobs
WHERE status = %[2]s AND name = %[3]s
ORDER BY %[4]s %[5]s, id %[5]s
LIMIT %[6]d`, jobCols, s.ph(1), s.ph(2), b.col, b.dir, listScanLimit)
		args = []any{b.status, name}
	} else {
		q = fmt.Sprintf(`SELECT %[1]s FROM regius_jobs
WHERE status = %[2]s
ORDER BY %[3]s %[4]s, id %[4]s
LIMIT %[5]d`, jobCols, s.ph(1), b.col, b.dir, listScanLimit)
		args = []any{b.status}
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*Job
	for rows.Next() {
		j, err := scanJobRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// Get returns the job with the given ID regardless of status.
func (s *SQLStore) Get(ctx context.Context, id string) (*Job, error) {
	return s.getJob(ctx, s.DB, id)
}

func (s *SQLStore) getJob(ctx context.Context, q rowQuerier, id string) (*Job, error) {
	query := fmt.Sprintf(`SELECT %s FROM regius_jobs WHERE id = %s`, jobCols, s.ph(1))
	j, err := scanJobRow(q.QueryRowContext(ctx, query, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoJob
	}
	return j, err
}

// Retry moves a dead job back to pending with its attempts reset.
func (s *SQLStore) Retry(ctx context.Context, id string) error {
	now := s.now()
	q := fmt.Sprintf(`UPDATE regius_jobs
SET status = 'pending', attempts = 0, run_at = %[1]s, updated_at = %[2]s, lease_until = NULL
WHERE id = %[3]s AND status = 'dead'`, s.ph(1), s.ph(2), s.ph(3))
	return s.affected(ctx, q, utc(now), utc(now), id)
}

// Drop permanently removes a dead job.
func (s *SQLStore) Drop(ctx context.Context, id string) error {
	q := fmt.Sprintf(`DELETE FROM regius_jobs WHERE id = %s AND status = 'dead'`, s.ph(1))
	return s.affected(ctx, q, id)
}

// Prune deletes completed jobs older than retention. Dead jobs are never
// pruned automatically.
func (s *SQLStore) Prune(ctx context.Context, now time.Time, retention time.Duration) (int, error) {
	q := fmt.Sprintf(`DELETE FROM regius_jobs WHERE status = 'completed' AND completed_at < %s`, s.ph(1))
	res, err := s.DB.ExecContext(ctx, q, utc(now.Add(-retention)))
	if err != nil {
		return 0, err
	}
	return intAffected(res)
}

// affected runs a guarded UPDATE/DELETE and maps zero affected rows to
// ErrNoJob.
func (s *SQLStore) affected(ctx context.Context, q string, args ...any) error {
	res, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := intAffected(res)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoJob
	}
	return nil
}

func intAffected(res sql.Result) (int, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
