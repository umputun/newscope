package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // pure Go SQLite driver
)

//go:embed schema.sql
var schemaFS embed.FS

// DB wraps sqlx.DB with our custom methods
type DB struct {
	*sqlx.DB
}

// Config represents database configuration
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// New creates a new database connection
func New(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		cfg.DSN = "file:newscope.db?cache=shared&mode=rwc"
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

	sdb := &DB{DB: db}

	// initialize schema
	if err := sdb.initSchema(ctx); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return sdb, nil
}

// initSchema creates tables if they don't exist
func (db *DB) initSchema(ctx context.Context) error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}

	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	return nil
}

// Feed operations

// CreateFeed inserts a new feed
func (db *DB) CreateFeed(ctx context.Context, feed *Feed) error {
	query := `
		INSERT INTO feeds (url, title, description, fetch_interval, enabled)
		VALUES (:url, :title, :description, :fetch_interval, :enabled)
	`
	result, err := db.NamedExecContext(ctx, query, feed)
	if err != nil {
		return fmt.Errorf("create feed: %w", err)
	}

	feed.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get insert id: %w", err)
	}

	return nil
}

// GetFeed retrieves a feed by ID
func (db *DB) GetFeed(ctx context.Context, id int64) (*Feed, error) {
	var feed Feed
	err := db.GetContext(ctx, &feed, "SELECT * FROM feeds WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("get feed: %w", err)
	}
	return &feed, nil
}

// GetFeeds retrieves all feeds, optionally filtered by enabled status
func (db *DB) GetFeeds(ctx context.Context, enabledOnly bool) ([]Feed, error) {
	query := "SELECT * FROM feeds"
	if enabledOnly {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY title"

	var feeds []Feed
	err := db.SelectContext(ctx, &feeds, query)
	if err != nil {
		return nil, fmt.Errorf("get feeds: %w", err)
	}
	return feeds, nil
}

// GetFeedsToFetch retrieves feeds that need updating
func (db *DB) GetFeedsToFetch(ctx context.Context, limit int) ([]Feed, error) {
	query := `
		SELECT * FROM feeds 
		WHERE enabled = 1 
		AND (next_fetch IS NULL OR next_fetch <= datetime('now'))
		ORDER BY next_fetch ASC
		LIMIT ?
	`
	var feeds []Feed
	err := db.SelectContext(ctx, &feeds, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get feeds to fetch: %w", err)
	}
	return feeds, nil
}

// UpdateFeedFetched updates feed after successful fetch
func (db *DB) UpdateFeedFetched(ctx context.Context, feedID int64, nextFetch time.Time) error {
	query := `
		UPDATE feeds 
		SET last_fetched = datetime('now'), 
		    next_fetch = ?,
		    error_count = 0,
		    last_error = ''
		WHERE id = ?
	`
	_, err := db.ExecContext(ctx, query, nextFetch, feedID)
	if err != nil {
		return fmt.Errorf("update feed fetched: %w", err)
	}
	return nil
}

// UpdateFeedError updates feed after fetch error
func (db *DB) UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error {
	query := `
		UPDATE feeds 
		SET error_count = error_count + 1,
		    last_error = ?
		WHERE id = ?
	`
	_, err := db.ExecContext(ctx, query, errMsg, feedID)
	if err != nil {
		return fmt.Errorf("update feed error: %w", err)
	}
	return nil
}

// UpdateFeedStatus enables or disables a feed
func (db *DB) UpdateFeedStatus(ctx context.Context, feedID int64, enabled bool) error {
	query := "UPDATE feeds SET enabled = ? WHERE id = ?"
	_, err := db.ExecContext(ctx, query, enabled, feedID)
	if err != nil {
		return fmt.Errorf("update feed status: %w", err)
	}
	return nil
}

// DeleteFeed removes a feed and all its items
func (db *DB) DeleteFeed(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM feeds WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	return nil
}

// Item operations

// CreateItem inserts a new item
func (db *DB) CreateItem(ctx context.Context, item *Item) error {
	query := `
		INSERT INTO items (
			feed_id, guid, title, link, description, content, 
			author, published
		) VALUES (
			:feed_id, :guid, :title, :link, :description, :content,
			:author, :published
		)
	`
	result, err := db.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("create item: %w", err)
	}

	item.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get insert id: %w", err)
	}

	return nil
}

// GetItem retrieves an item by ID
func (db *DB) GetItem(ctx context.Context, id int64) (*Item, error) {
	var item Item
	err := db.GetContext(ctx, &item, "SELECT * FROM items WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	return &item, nil
}

// GetItems retrieves items with optional filters
func (db *DB) GetItems(ctx context.Context, limit int, minScore float64) ([]Item, error) {
	query := `
		SELECT * FROM items 
		WHERE relevance_score >= ?
		ORDER BY published DESC
		LIMIT ?
	`
	var items []Item
	err := db.SelectContext(ctx, &items, query, minScore, limit)
	if err != nil {
		return nil, fmt.Errorf("get items: %w", err)
	}
	return items, nil
}

// GetUnclassifiedItems retrieves items that need classification
func (db *DB) GetUnclassifiedItems(ctx context.Context, limit int) ([]Item, error) {
	query := `
		SELECT * FROM items 
		WHERE classified_at IS NULL
		AND extracted_content != ''
		AND extraction_error = ''
		ORDER BY published DESC
		LIMIT ?
	`
	var items []Item
	err := db.SelectContext(ctx, &items, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get unclassified items: %w", err)
	}
	return items, nil
}

// GetItemsNeedingExtraction retrieves items that need content extraction
func (db *DB) GetItemsNeedingExtraction(ctx context.Context, limit int) ([]Item, error) {
	query := `
		SELECT * FROM items 
		WHERE extracted_at IS NULL
		AND extraction_error = ''
		ORDER BY published DESC
		LIMIT ?
	`
	var items []Item
	err := db.SelectContext(ctx, &items, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get items needing extraction: %w", err)
	}
	return items, nil
}

// UpdateItemExtraction updates item after content extraction
func (db *DB) UpdateItemExtraction(ctx context.Context, itemID int64, content, richContent string, err error) error {
	var query string
	var args []interface{}

	if err != nil {
		query = `
			UPDATE items 
			SET extraction_error = ?, extracted_at = datetime('now')
			WHERE id = ?
		`
		args = []interface{}{err.Error(), itemID}
	} else {
		query = `
			UPDATE items 
			SET extracted_content = ?, extracted_rich_content = ?, extracted_at = datetime('now')
			WHERE id = ?
		`
		args = []interface{}{content, richContent, itemID}
	}

	_, execErr := db.ExecContext(ctx, query, args...)
	if execErr != nil {
		return fmt.Errorf("update item extraction: %w", execErr)
	}
	return nil
}

// UpdateItemClassification updates item with LLM classification results
func (db *DB) UpdateItemClassification(ctx context.Context, itemID int64, score float64, explanation string, topics []string) error {
	query := `
		UPDATE items 
		SET relevance_score = ?, 
		    explanation = ?,
		    topics = ?,
		    classified_at = datetime('now')
		WHERE id = ?
	`
	_, err := db.ExecContext(ctx, query, score, explanation, Topics(topics), itemID)
	if err != nil {
		return fmt.Errorf("update item classification: %w", err)
	}
	return nil
}

// UpdateItemProcessed updates item with both extraction and classification results
func (db *DB) UpdateItemProcessed(ctx context.Context, itemID int64, content, richContent string, classification Classification) error {
	// build query based on whether we have a summary
	var query string
	var args []interface{}

	if classification.Summary != "" {
		// update all fields including description in a single atomic operation
		query = `
			UPDATE items 
			SET extracted_content = ?, 
			    extracted_rich_content = ?, 
			    extracted_at = datetime('now'),
			    relevance_score = ?, 
			    explanation = ?,
			    topics = ?,
			    classified_at = datetime('now'),
			    description = ?
			WHERE id = ?
		`
		args = []interface{}{content, richContent, classification.Score, classification.Explanation,
			Topics(classification.Topics), classification.Summary, itemID}
	} else {
		// update without changing description
		query = `
			UPDATE items 
			SET extracted_content = ?, 
			    extracted_rich_content = ?, 
			    extracted_at = datetime('now'),
			    relevance_score = ?, 
			    explanation = ?,
			    topics = ?,
			    classified_at = datetime('now')
			WHERE id = ?
		`
		args = []interface{}{content, richContent, classification.Score, classification.Explanation,
			Topics(classification.Topics), itemID}
	}

	_, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update item processed: %w", err)
	}

	return nil
}

// UpdateItemFeedback updates user feedback on an item
func (db *DB) UpdateItemFeedback(ctx context.Context, itemID int64, feedback string) error {
	query := `
		UPDATE items 
		SET user_feedback = ?, feedback_at = datetime('now')
		WHERE id = ?
	`
	_, err := db.ExecContext(ctx, query, feedback, itemID)
	if err != nil {
		return fmt.Errorf("update item feedback: %w", err)
	}
	return nil
}

// GetRecentFeedback retrieves recent user feedback for LLM context
func (db *DB) GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]FeedbackExample, error) {
	query := `
		SELECT title, description, 
		       SUBSTR(extracted_content, 1, 500) as content,
		       user_feedback as feedback, 
		       topics
		FROM items 
		WHERE user_feedback = ?
		AND feedback_at IS NOT NULL
		ORDER BY feedback_at DESC
		LIMIT ?
	`

	rows, err := db.QueryContext(ctx, query, feedbackType, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent feedback: %w", err)
	}
	defer rows.Close()

	var examples []FeedbackExample
	for rows.Next() {
		var example FeedbackExample
		var topics Topics
		err := rows.Scan(&example.Title, &example.Description, &example.Content, &example.Feedback, &topics)
		if err != nil {
			return nil, fmt.Errorf("scan feedback row: %w", err)
		}
		example.Topics = topics
		examples = append(examples, example)
	}

	return examples, nil
}

// ItemExists checks if an item already exists
func (db *DB) ItemExists(ctx context.Context, feedID int64, guid string) (bool, error) {
	var exists bool
	err := db.GetContext(ctx, &exists,
		"SELECT EXISTS(SELECT 1 FROM items WHERE feed_id = ? AND guid = ?)",
		feedID, guid)
	if err != nil {
		return false, fmt.Errorf("check item exists: %w", err)
	}
	return exists, nil
}

// Setting operations

// GetSetting retrieves a setting value
func (db *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := db.GetContext(ctx, &value, "SELECT value FROM settings WHERE key = ?", key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting: %w", err)
	}
	return value, nil
}

// SetSetting stores a setting value
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	query := `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`
	_, err := db.ExecContext(ctx, query, key, value)
	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}

// GetClassifiedItems returns classified items with feed information
func (db *DB) GetClassifiedItems(ctx context.Context, minScore float64, limit int) ([]Item, error) {
	query := `
		SELECT 
			i.*,
			f.title as feed_title
		FROM items i
		JOIN feeds f ON i.feed_id = f.id
		WHERE i.relevance_score >= ?
		AND i.classified_at IS NOT NULL
		ORDER BY i.published DESC
		LIMIT ?
	`

	var items []Item
	if err := db.SelectContext(ctx, &items, query, minScore, limit); err != nil {
		return nil, fmt.Errorf("get classified items: %w", err)
	}
	return items, nil
}

// GetClassifiedItem returns a single classified item with feed information
func (db *DB) GetClassifiedItem(ctx context.Context, itemID int64) (*Item, error) {
	query := `
		SELECT 
			i.*,
			f.title as feed_title
		FROM items i
		JOIN feeds f ON i.feed_id = f.id
		WHERE i.id = ?
	`

	var item Item
	if err := db.GetContext(ctx, &item, query, itemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("item not found")
		}
		return nil, fmt.Errorf("get classified item: %w", err)
	}
	return &item, nil
}

// GetTopics returns all unique topics from classified items
func (db *DB) GetTopics(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT value 
		FROM (
			SELECT json_each.value 
			FROM items, json_each(items.topics)
			WHERE items.classified_at IS NOT NULL
		)
		ORDER BY value
	`

	var topics []string
	if err := db.SelectContext(ctx, &topics, query); err != nil {
		return nil, fmt.Errorf("get topics: %w", err)
	}
	return topics, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// Ping verifies the database connection
func (db *DB) Ping(ctx context.Context) error {
	return db.PingContext(ctx)
}

// InTransaction executes a function within a database transaction
func (db *DB) InTransaction(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
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
