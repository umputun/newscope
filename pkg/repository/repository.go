package repository

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // pure Go SQLite driver
)

//go:embed schema.sql migrations.sql
var schemaFS embed.FS

// Config represents database configuration
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// Repositories contains all repository instances
type Repositories struct {
	Feed           *FeedRepository
	Item           *ItemRepository
	Classification *ClassificationRepository
	Setting        *SettingRepository
	DB             *sqlx.DB
}

// NewRepositories creates all repositories with a shared database connection
func NewRepositories(ctx context.Context, cfg Config) (*Repositories, error) {
	if cfg.DSN == "" {
		cfg.DSN = "file:newscope.db?cache=shared&mode=rwc&_txlock=immediate"
	}

	db, err := sqlx.Open("sqlite", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// configure connection pool
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	// enable foreign keys
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// optimize SQLite settings
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000", // 64MB cache
		"PRAGMA temp_store = MEMORY",
		"PRAGMA busy_timeout = 5000", // 5 second timeout for locks
	}

	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return nil, fmt.Errorf("execute %s: %w", pragma, err)
		}
	}

	// initialize schema
	if err := initSchema(ctx, db); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// run migrations
	if err := runMigrations(ctx, db); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// create repositories
	repos := &Repositories{
		Feed:           NewFeedRepository(db),
		Item:           NewItemRepository(db),
		Classification: NewClassificationRepository(db),
		Setting:        NewSettingRepository(db),
		DB:             db,
	}

	return repos, nil
}

// Close closes the database connection
func (r *Repositories) Close() error {
	return r.DB.Close()
}

// Ping verifies the database connection
func (r *Repositories) Ping(ctx context.Context) error {
	return r.DB.PingContext(ctx)
}

// initSchema creates tables if they don't exist
func initSchema(ctx context.Context, db *sqlx.DB) error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}

	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	return nil
}

// criticalError wraps an error to signal repeater to stop retrying
type criticalError struct {
	err error
}

func (e *criticalError) Error() string {
	return e.err.Error()
}

// isLockError checks if an error is a SQLite lock/busy error
func isLockError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "SQLITE_BUSY") ||
		strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "database table is locked")
}

// runMigrations runs database migrations to update schema
func runMigrations(ctx context.Context, db *sqlx.DB) error {
	// check if summary column exists
	var count int
	err := db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM pragma_table_info('items') WHERE name = 'summary'`)
	if err != nil {
		return fmt.Errorf("check summary column: %w", err)
	}

	// add summary column if it doesn't exist
	if count == 0 {
		if _, err := db.ExecContext(ctx, `ALTER TABLE items ADD COLUMN summary TEXT DEFAULT ''`); err != nil {
			return fmt.Errorf("add summary column: %w", err)
		}
	}

	// read and execute migrations.sql
	migrations, err := schemaFS.ReadFile("migrations.sql")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	// execute migrations (indexes are idempotent with IF NOT EXISTS)
	if _, err := db.ExecContext(ctx, string(migrations)); err != nil {
		return fmt.Errorf("execute migrations: %w", err)
	}

	return nil
}
