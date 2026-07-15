package regius

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

func normalizeDBType(dbType string) (string, error) {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return "pgx", nil
	case "mysql", "mariadb":
		return "mysql", nil
	case "sqlite", "sqlite3":
		return "sqlite3", nil
	case "":
		return "", fmt.Errorf("database type is empty")
	default:
		return "", fmt.Errorf("unsupported database type: %s", dbType)
	}
}

func (r *Regius) OpenDB(dbType, dsn string) (*sql.DB, error) {
	driver, err := normalizeDBType(dbType)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}
