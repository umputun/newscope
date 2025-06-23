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

	// split and execute migrations one by one to handle SQLite limitations
	statements := splitMigrationStatements(string(migrations))
	for _, stmt := range statements {
		if stmt != "" {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				// check if this is an expected "already exists" error for idempotent migrations
				errStr := err.Error()
				if strings.Contains(errStr, "already exists") ||
					strings.Contains(errStr, "duplicate") ||
					strings.Contains(errStr, "UNIQUE constraint failed") {
					// these errors are expected for idempotent migrations, skip
					continue
				}
				// return actual errors
				return fmt.Errorf("execute migration statement: %w", err)
			}
		}
	}

	return nil
}

// splitMigrationStatements splits SQL migration statements by semicolon
func splitMigrationStatements(migrations string) []string {
	// special handling for triggers with BEGIN/END blocks
	lines := strings.Split(migrations, "\n")
	var statements []string
	var current strings.Builder
	inTrigger := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// skip comment-only lines
		if strings.HasPrefix(trimmed, "--") && current.Len() == 0 {
			continue
		}

		// skip empty lines when not building a statement
		if trimmed == "" && current.Len() == 0 {
			continue
		}

		// detect trigger start
		upperTrimmed := strings.ToUpper(trimmed)
		if strings.Contains(upperTrimmed, "CREATE TRIGGER") {
			inTrigger = true
		}

		// add line to current statement
		if current.Len() > 0 || (trimmed != "" && !strings.HasPrefix(trimmed, "--")) {
			current.WriteString(line)
			current.WriteString("\n")
		}

		// check for statement end
		if strings.HasSuffix(trimmed, ";") {
			// for triggers, check if this is the END; statement
			if inTrigger && strings.EqualFold(trimmed, "END;") {
				inTrigger = false
			}

			// if not in trigger, or if we just ended a trigger, save the statement
			if !inTrigger {
				stmt := strings.TrimSpace(current.String())
				if stmt != "" {
					statements = append(statements, stmt)
				}
				current.Reset()
			}
		}
	}

	// add any remaining statement
	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements
}
