package repository

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/domain"
)

func TestFTS5Support(t *testing.T) {
	// create in-memory database
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// create simple table and FTS
	_, err = db.Exec(`
		CREATE TABLE test_items (
			id INTEGER PRIMARY KEY,
			title TEXT,
			content TEXT
		)
	`)
	require.NoError(t, err)

	// create FTS5 table
	_, err = db.Exec(`
		CREATE VIRTUAL TABLE test_fts USING fts5(
			title,
			content,
			content=test_items,
			content_rowid=id
		)
	`)
	require.NoError(t, err)

	// insert test data
	_, err = db.Exec(`INSERT INTO test_items (title, content) VALUES (?, ?)`,
		"Go Programming", "Learn Go programming language")
	require.NoError(t, err)

	// populate FTS
	_, err = db.Exec(`INSERT INTO test_fts (rowid, title, content) 
		SELECT id, title, content FROM test_items`)
	require.NoError(t, err)

	// search
	var count int
	err = db.Get(&count, `SELECT COUNT(*) FROM test_fts WHERE test_fts MATCH 'programming'`)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestClassificationRepository_SearchItemsSimple(t *testing.T) {
	// setup test database with clean state
	ctx := context.Background()
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// read and execute schema
	schema, err := schemaFS.ReadFile("schema.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(schema))
	require.NoError(t, err)

	// verify FTS table exists
	var ftsCount int
	err = db.Get(&ftsCount, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='items_fts'`)
	require.NoError(t, err)
	assert.Equal(t, 1, ftsCount, "FTS table should exist")

	// create repository
	repo := &ClassificationRepository{db: db}

	// insert test data directly
	_, err = db.Exec(`INSERT INTO feeds (url, title) VALUES (?, ?)`,
		"https://example.com/feed", "Test Feed")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO items (feed_id, guid, title, link, description, content, published, 
		relevance_score, topics, summary, classified_at) 
		VALUES (1, 'test-1', 'Go Programming', 'https://example.com/1', 
		'Learn Go', 'Content about Go programming', datetime('now'),
		8.0, '["go", "programming"]', 'Go summary', datetime('now'))`)
	require.NoError(t, err)

	// search should work now
	items, err := repo.SearchItems(ctx, "programming", &domain.ItemFilter{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "Go Programming", items[0].Title)
}
