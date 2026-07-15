package regius

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"
)

// ConfigurePool applies connection pool settings from environment variables.
// Supported variables:
//   - DATABASE_MAX_OPEN_CONNS
//   - DATABASE_MAX_IDLE_CONNS
//   - DATABASE_CONN_MAX_LIFETIME (e.g. 15m, 1h)
//
// When a separate read pool is configured, the same settings are applied to it.
func (d *Database) ConfigurePool() error {
	if d.Pool == nil {
		return fmt.Errorf("database pool is nil")
	}

	apply := func(db *sql.DB) error {
		if v := os.Getenv("DATABASE_MAX_OPEN_CONNS"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("invalid DATABASE_MAX_OPEN_CONNS: %w", err)
			}
			db.SetMaxOpenConns(n)
		}

		if v := os.Getenv("DATABASE_MAX_IDLE_CONNS"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("invalid DATABASE_MAX_IDLE_CONNS: %w", err)
			}
			db.SetMaxIdleConns(n)
		}

		if v := os.Getenv("DATABASE_CONN_MAX_LIFETIME"); v != "" {
			duration, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("invalid DATABASE_CONN_MAX_LIFETIME: %w", err)
			}
			db.SetConnMaxLifetime(duration)
		}

		return nil
	}

	if err := apply(d.Pool); err != nil {
		return err
	}
	if d.ReadPool != nil && d.ReadPool != d.Pool {
		if err := apply(d.ReadPool); err != nil {
			return err
		}
	}

	return nil
}

// HealthCheck verifies the database connection is alive.
// If a separate read pool is configured, it is checked as well.
func (d *Database) HealthCheck(ctx context.Context) error {
	if d.Pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	if err := d.Pool.PingContext(ctx); err != nil {
		return err
	}
	if d.ReadPool != nil && d.ReadPool != d.Pool {
		if err := d.ReadPool.PingContext(ctx); err != nil {
			return fmt.Errorf("read replica ping failed: %w", err)
		}
	}
	return nil
}

// Reader returns the pool to use for read queries.
// When no read replica is configured, this returns the main pool.
func (d *Database) Reader() *sql.DB {
	if d.ReadPool != nil {
		return d.ReadPool
	}
	return d.Pool
}

// Writer returns the pool to use for write queries and transactions.
func (d *Database) Writer() *sql.DB {
	if d.WritePool != nil {
		return d.WritePool
	}
	return d.Pool
}

// Transaction runs the given function inside a database transaction on the
// write pool. If the function returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
func (d *Database) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	pool := d.Writer()
	if pool == nil {
		return fmt.Errorf("database pool is nil")
	}

	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction rollback failed: %w (original error: %v)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Transaction runs the given function inside a database transaction using the
// framework's configured write pool.
func (r *Regius) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	return r.DB.Transaction(ctx, fn)
}
