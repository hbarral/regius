package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const createSeedsTableSQL = `CREATE TABLE IF NOT EXISTS regius_seeds (
	name VARCHAR(255) PRIMARY KEY,
	executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`

// Seeder runs SQL seed files from the application's seeds/ directory.
// Each seed is executed once and tracked in the regius_seeds table.
type Seeder struct {
	DB       *sql.DB
	RootPath string
}

// RunSeeds executes all pending .sql seed files in seeds/ ordered by filename.
func (s *Seeder) RunSeeds() error {
	if s.DB == nil {
		return fmt.Errorf("database pool is nil")
	}

	seedsDir := filepath.Join(s.RootPath, "seeds")
	if err := os.MkdirAll(seedsDir, 0755); err != nil {
		return fmt.Errorf("failed to create seeds directory: %w", err)
	}

	if _, err := s.DB.Exec(createSeedsTableSQL); err != nil {
		return fmt.Errorf("failed to create seeds table: %w", err)
	}

	entries, err := os.ReadDir(seedsDir)
	if err != nil {
		return fmt.Errorf("failed to read seeds directory: %w", err)
	}

	var sqlFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}
	sort.Strings(sqlFiles)

	for _, name := range sqlFiles {
		var exists int
		err := s.DB.QueryRow("SELECT 1 FROM regius_seeds WHERE name = ?", name).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to check seed status for %s: %w", name, err)
		}

		content, err := os.ReadFile(filepath.Join(seedsDir, name))
		if err != nil {
			return fmt.Errorf("failed to read seed %s: %w", name, err)
		}

		if err := s.transaction(context.Background(), func(tx *sql.Tx) error {
			if _, err := tx.Exec(string(content)); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO regius_seeds (name) VALUES (?)", name); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed to run seed %s: %w", name, err)
		}
	}

	return nil
}

func (s *Seeder) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
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
