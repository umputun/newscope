package repository

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // pure Go SQLite driver
)

//go:embed schema.sql
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