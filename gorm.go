package regius

import (
	"fmt"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// GORM returns a GORM instance backed by the framework's configured database pool.
// The dialector is chosen from DATABASE_TYPE. If no database is configured, an
// error is returned.
func (r *Regius) GORM() (*gorm.DB, error) {
	if r.DB.Pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}

	dbType := strings.ToLower(r.DB.DataType)

	switch dbType {
	case "postgres", "postgresql":
		return gorm.Open(postgres.New(postgres.Config{
			Conn: r.DB.Pool,
		}), &gorm.Config{})

	case "mysql", "mariadb":
		return gorm.Open(mysql.New(mysql.Config{
			Conn: r.DB.Pool,
		}), &gorm.Config{})

	case "sqlite", "sqlite3":
		return gorm.Open(sqlite.New(sqlite.Config{
			Conn: r.DB.Pool,
		}), &gorm.Config{})

	default:
		return nil, fmt.Errorf("unsupported database type for GORM: %s", r.DB.DataType)
	}
}

// GORMWithConfig returns a GORM instance using the provided config. Use this
// when you need custom GORM options (naming strategies, logger, etc.).
func (r *Regius) GORMWithConfig(cfg *gorm.Config) (*gorm.DB, error) {
	if r.DB.Pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}

	dbType := strings.ToLower(r.DB.DataType)

	switch dbType {
	case "postgres", "postgresql":
		return gorm.Open(postgres.New(postgres.Config{Conn: r.DB.Pool}), cfg)
	case "mysql", "mariadb":
		return gorm.Open(mysql.New(mysql.Config{Conn: r.DB.Pool}), cfg)
	case "sqlite", "sqlite3":
		return gorm.Open(sqlite.New(sqlite.Config{Conn: r.DB.Pool}), cfg)
	default:
		return nil, fmt.Errorf("unsupported database type for GORM: %s", r.DB.DataType)
	}
}

// AutoMigrate runs GORM AutoMigrate for the given models against the
// configured database. This is a convenience wrapper around GORM's
// AutoMigrate method.
func (r *Regius) AutoMigrate(dst ...interface{}) error {
	db, err := r.GORM()
	if err != nil {
		return err
	}
	return db.AutoMigrate(dst...)
}
