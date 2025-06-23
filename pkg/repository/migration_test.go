package repository

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMigrations_AddSummaryColumn(t *testing.T) {
	// create test database with old schema (without summary column)
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// create old schema without summary column
	oldSchema := `
		CREATE TABLE feeds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL UNIQUE,
			title TEXT DEFAULT '',
			description TEXT DEFAULT '',
			last_fetched DATETIME,
			next_fetch DATETIME,
			fetch_interval INTEGER DEFAULT 1800,
			error_count INTEGER DEFAULT 0,
			last_error TEXT DEFAULT '',
			enabled BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		
		CREATE TABLE items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			feed_id INTEGER NOT NULL,
			guid TEXT NOT NULL,
			title TEXT NOT NULL,
			link TEXT NOT NULL,
			description TEXT DEFAULT '',
			content TEXT DEFAULT '',
			author TEXT DEFAULT '',
			published DATETIME,
			extracted_content TEXT DEFAULT '',
			extracted_rich_content TEXT DEFAULT '',
			extracted_at DATETIME,
			extraction_error TEXT DEFAULT '',
			relevance_score REAL DEFAULT 0,
			explanation TEXT DEFAULT '',
			topics JSON DEFAULT '[]',
			classified_at DATETIME,
			user_feedback TEXT DEFAULT '',
			feedback_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
		);
	`
	_, err = db.ExecContext(ctx, oldSchema)
	require.NoError(t, err)

	// insert test data
	_, err = db.ExecContext(ctx, `
		INSERT INTO items (feed_id, guid, title, link, relevance_score, explanation)
		VALUES (1, 'test-guid', 'Test Article', 'http://example.com', 8.5, 'Good article')
	`)
	require.NoError(t, err)

	// check that summary column doesn't exist yet
	var count int
	err = db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM pragma_table_info('items') WHERE name = 'summary'`)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "summary column should not exist before migration")

	// run migrations
	err = runMigrations(ctx, db)
	require.NoError(t, err)

	// check that summary column now exists
	err = db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM pragma_table_info('items') WHERE name = 'summary'`)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "summary column should exist after migration")

	// verify we can insert data with summary
	_, err = db.ExecContext(ctx, `
		INSERT INTO items (feed_id, guid, title, link, summary)
		VALUES (1, 'test-guid-2', 'Test Article 2', 'http://example2.com', 'This is a summary')
	`)
	require.NoError(t, err)

	// verify we can query the summary column
	var summary string
	err = db.GetContext(ctx, &summary,
		`SELECT summary FROM items WHERE guid = 'test-guid-2'`)
	require.NoError(t, err)
	assert.Equal(t, "This is a summary", summary)

	// verify old data has empty summary
	err = db.GetContext(ctx, &summary,
		`SELECT summary FROM items WHERE guid = 'test-guid'`)
	require.NoError(t, err)
	assert.Empty(t, summary)
}

func TestRunMigrations_IdempotentIndexes(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// initialize schema
	err = initSchema(ctx, db)
	require.NoError(t, err)

	// run migrations twice - should be idempotent
	err = runMigrations(ctx, db)
	require.NoError(t, err)

	err = runMigrations(ctx, db)
	require.NoError(t, err, "migrations should be idempotent")

	// verify indexes exist
	var indexCount int
	err = db.GetContext(ctx, &indexCount,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name LIKE 'idx_items_%'`)
	require.NoError(t, err)
	assert.Greater(t, indexCount, 5, "should have created performance indexes")
}

func TestTopicJSONIndex(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// initialize schema with indexes
	err = initSchema(ctx, db)
	require.NoError(t, err)
	err = runMigrations(ctx, db)
	require.NoError(t, err)

	// verify JSON index exists
	var indexExists bool
	err = db.GetContext(ctx, &indexExists,
		`SELECT COUNT(*) > 0 FROM sqlite_master 
		WHERE type = 'index' AND name = 'idx_items_topics_json'`)
	require.NoError(t, err)
	assert.True(t, indexExists, "JSON topic index should exist")

	// test that index can be used for topic queries
	// create a test feed first
	_, err = db.ExecContext(ctx, `
		INSERT INTO feeds (url, title, enabled) 
		VALUES ('http://example.com/feed', 'Test Feed', 1)
	`)
	require.NoError(t, err)

	// insert test data
	_, err = db.ExecContext(ctx, `
		INSERT INTO items (feed_id, guid, title, link, topics, relevance_score, classified_at)
		VALUES 
			(1, 'test-1', 'Article 1', 'http://example.com/1', '["tech", "ai"]', 8.0, datetime('now')),
			(1, 'test-2', 'Article 2', 'http://example.com/2', '["science", "ai"]', 7.5, datetime('now')),
			(1, 'test-3', 'Article 3', 'http://example.com/3', '["tech", "web"]', 9.0, datetime('now'))
	`)
	require.NoError(t, err)

	// test optimized topic query
	var count int
	err = db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM items
		WHERE EXISTS (
			SELECT 1 FROM json_each(items.topics)
			WHERE json_each.value = 'tech'
		)
	`)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "should find 2 items with 'tech' topic")
}
