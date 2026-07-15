package regius

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/mattn/go-sqlite3"
)

var (
	queryLogMu     sync.RWMutex
	queryLogLogger *log.Logger
)

func setQueryLogLogger(logger *log.Logger) {
	queryLogMu.Lock()
	defer queryLogMu.Unlock()
	queryLogLogger = logger
}

func getQueryLogLogger() *log.Logger {
	queryLogMu.RLock()
	defer queryLogMu.RUnlock()
	return queryLogLogger
}

func logQuery(query string, args []driver.Value, duration time.Duration, err error) {
	logger := getQueryLogLogger()
	if logger == nil {
		return
	}

	var argStr string
	if len(args) > 0 {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = valueToString(a)
		}
		argStr = " [" + strings.Join(parts, ", ") + "]"
	}

	if err != nil {
		logger.Printf("[DB] %s%s | %s | error: %v", query, argStr, duration, err)
	} else {
		logger.Printf("[DB] %s%s | %s", query, argStr, duration)
	}
}

func valueToString(v driver.Value) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		return "'" + val + "'"
	case []byte:
		return "'" + string(val) + "'"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// loggingDriver wraps an underlying sql/driver.Driver and logs every operation.
type loggingDriver struct {
	base driver.Driver
	name string
}

func (d *loggingDriver) Open(name string) (driver.Conn, error) {
	start := time.Now()
	conn, err := d.base.Open(name)
	logQuery("open("+d.name+")", nil, time.Since(start), err)
	if err != nil {
		return nil, err
	}
	return &loggingConn{conn: conn}, nil
}

// loggingConn wraps driver.Conn and optional context-aware interfaces.
type loggingConn struct {
	conn driver.Conn
}

func (c *loggingConn) Prepare(query string) (driver.Stmt, error) {
	start := time.Now()
	stmt, err := c.conn.Prepare(query)
	logQuery("prepare", nil, time.Since(start), err)
	if err != nil {
		return nil, err
	}
	return &loggingStmt{stmt: stmt, query: query}, nil
}

func (c *loggingConn) Close() error {
	return c.conn.Close()
}

func (c *loggingConn) Begin() (driver.Tx, error) {
	start := time.Now()
	tx, err := c.conn.Begin()
	logQuery("begin", nil, time.Since(start), err)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (c *loggingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if btx, ok := c.conn.(driver.ConnBeginTx); ok {
		start := time.Now()
		tx, err := btx.BeginTx(ctx, opts)
		logQuery("begin", nil, time.Since(start), err)
		if err != nil {
			return nil, err
		}
		return tx, nil
	}
	return c.Begin()
}

func (c *loggingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := c.conn.(driver.ExecerContext); ok {
		start := time.Now()
		res, err := ec.ExecContext(ctx, query, args)
		logQuery(query, namedToValues(args), time.Since(start), err)
		return res, err
	}
	return nil, driver.ErrSkip
}

func (c *loggingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := c.conn.(driver.QueryerContext); ok {
		start := time.Now()
		rows, err := qc.QueryContext(ctx, query, args)
		logQuery(query, namedToValues(args), time.Since(start), err)
		return rows, err
	}
	return nil, driver.ErrSkip
}

func (c *loggingConn) Ping(ctx context.Context) error {
	if p, ok := c.conn.(driver.Pinger); ok {
		start := time.Now()
		err := p.Ping(ctx)
		logQuery("ping", nil, time.Since(start), err)
		return err
	}
	return nil
}

// loggingStmt wraps driver.Stmt and optional context-aware statement interfaces.
type loggingStmt struct {
	stmt  driver.Stmt
	query string
}

func (s *loggingStmt) Close() error {
	return s.stmt.Close()
}

func (s *loggingStmt) NumInput() int {
	return s.stmt.NumInput()
}

func (s *loggingStmt) Exec(args []driver.Value) (driver.Result, error) {
	start := time.Now()
	res, err := s.stmt.Exec(args)
	logQuery(s.query, args, time.Since(start), err)
	return res, err
}

func (s *loggingStmt) Query(args []driver.Value) (driver.Rows, error) {
	start := time.Now()
	rows, err := s.stmt.Query(args)
	logQuery(s.query, args, time.Since(start), err)
	return rows, err
}

func (s *loggingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if es, ok := s.stmt.(driver.StmtExecContext); ok {
		start := time.Now()
		res, err := es.ExecContext(ctx, args)
		logQuery(s.query, namedToValues(args), time.Since(start), err)
		return res, err
	}
	return s.Exec(namedToValues(args))
}

func (s *loggingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if qs, ok := s.stmt.(driver.StmtQueryContext); ok {
		start := time.Now()
		rows, err := qs.QueryContext(ctx, args)
		logQuery(s.query, namedToValues(args), time.Since(start), err)
		return rows, err
	}
	return s.Query(namedToValues(args))
}

func namedToValues(named []driver.NamedValue) []driver.Value {
	values := make([]driver.Value, len(named))
	for i, n := range named {
		values[i] = n.Value
	}
	return values
}

var loggingDriversRegistered sync.Once

func registerLoggingDrivers() {
	loggingDriversRegistered.Do(func() {
		sql.Register("pgx-log", &loggingDriver{name: "pgx", base: &stdlib.Driver{}})
		sql.Register("mysql-log", &loggingDriver{name: "mysql", base: &mysql.MySQLDriver{}})
		sql.Register("sqlite3-log", &loggingDriver{name: "sqlite3", base: &sqlite3.SQLiteDriver{}})
	})
}

func loggingDriverName(base string) string {
	return base + "-log"
}
