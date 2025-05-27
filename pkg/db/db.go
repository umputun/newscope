package db

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // pure Go SQLite driver
)

//go:embed schema.sql
var schema string

// DB wraps the database connection and provides methods for data access
type DB struct {
	conn *sqlx.DB
}

// Config represents database configuration
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// New creates a new database connection
func New(cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		cfg.DSN = "file:newscope.db?cache=shared&mode=rwc"
	}

	conn, err := sqlx.Open("sqlite", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// configure connection pool
	if cfg.MaxOpenConns > 0 {
		conn.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		conn.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	// enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// optimize SQLite settings
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000", // 64MB cache
		"PRAGMA temp_store = MEMORY",
	}

	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			return nil, fmt.Errorf("execute %s: %w", pragma, err)
		}
	}

	db := &DB{conn: conn}

	// initialize schema
	if err := db.InitSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return db, nil
}

// InitSchema creates the database schema
func (db *DB) InitSchema(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// DB returns the underlying sqlx.DB connection for direct access if needed
func (db *DB) DB() *sqlx.DB {
	return db.conn
}

// Ping verifies the database connection
func (db *DB) Ping(ctx context.Context) error {
	return db.conn.PingContext(ctx)
}

// InTransaction executes a function within a database transaction
func (db *DB) InTransaction(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := db.conn.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction failed: %w (rollback also failed: %s)", err, rbErr.Error())
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
