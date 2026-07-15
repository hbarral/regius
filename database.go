package regius

import (
	"context"
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
func (d *Database) ConfigurePool() error {
	if d.Pool == nil {
		return fmt.Errorf("database pool is nil")
	}

	if v := os.Getenv("DATABASE_MAX_OPEN_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid DATABASE_MAX_OPEN_CONNS: %w", err)
		}
		d.Pool.SetMaxOpenConns(n)
	}

	if v := os.Getenv("DATABASE_MAX_IDLE_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid DATABASE_MAX_IDLE_CONNS: %w", err)
		}
		d.Pool.SetMaxIdleConns(n)
	}

	if v := os.Getenv("DATABASE_CONN_MAX_LIFETIME"); v != "" {
		duration, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid DATABASE_CONN_MAX_LIFETIME: %w", err)
		}
		d.Pool.SetConnMaxLifetime(duration)
	}

	return nil
}

// HealthCheck verifies the database connection is alive.
func (d *Database) HealthCheck(ctx context.Context) error {
	if d.Pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	return d.Pool.PingContext(ctx)
}
