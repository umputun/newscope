package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (db *DB, cleanup func()) {
	t.Helper()

	// create temp file for test database
	tmpFile, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	tmpFile.Close()

	cfg := Config{
		DSN: "file:" + tmpFile.Name() + "?mode=rwc",
	}

	db, err = New(cfg)
	require.NoError(t, err)

	cleanup = func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}

	return db, cleanup
}

func TestDB_InitSchema(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// schema should already be initialized by New()
	// verify tables exist
	var count int
	err := db.conn.Get(&count, `
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name IN ('feeds', 'items', 'content', 'categories')
	`)
	require.NoError(t, err)
	assert.Equal(t, 4, count)
}

func TestDB_NewWithDefaults(t *testing.T) {
	// test with empty DSN (should use default)
	cfg := Config{}
	db, err := New(cfg)
	require.NoError(t, err)
	defer func() {
		db.Close()
		// clean up default db file
		os.Remove("newscope.db")
	}()

	// verify it works
	ctx := context.Background()
	err = db.Ping(ctx)
	assert.NoError(t, err)
}

func TestDB_NewWithConnectionSettings(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-conn-*.db")
	require.NoError(t, err)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	cfg := Config{
		DSN:             "file:" + tmpFile.Name() + "?mode=rwc",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Hour,
	}

	db, err := New(cfg)
	require.NoError(t, err)
	defer db.Close()

	// verify settings were applied
	sqlxDB := db.DB()
	stats := sqlxDB.Stats()
	assert.LessOrEqual(t, stats.MaxOpenConnections, 5)
}

func TestDB_Ping(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	err := db.Ping(ctx)
	assert.NoError(t, err)
}

func TestDB_GetUnderlyingDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// test DB() method
	sqlxDB := db.DB()
	assert.NotNil(t, sqlxDB)

	// verify we can use the underlying DB
	var count int
	err := sqlxDB.Get(&count, "SELECT COUNT(*) FROM feeds")
	assert.NoError(t, err)
}

func TestDB_InTransaction(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// successful transaction
	err := db.InTransaction(ctx, func(tx *sqlx.Tx) error {
		feed := &Feed{
			URL:   "https://example.com/feed.xml",
			Title: "Test Feed",
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO feeds (url, title) VALUES (:url, :title)
		`, feed)
		return err
	})
	require.NoError(t, err)

	// verify feed was inserted
	var count int
	err = db.conn.Get(&count, `SELECT COUNT(*) FROM feeds`)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// failed transaction should rollback
	err = db.InTransaction(ctx, func(tx *sqlx.Tx) error {
		feed := &Feed{
			URL:   "https://example2.com/feed.xml",
			Title: "Test Feed 2",
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO feeds (url, title) VALUES (:url, :title)
		`, feed)
		if err != nil {
			return err
		}
		// force error
		return assert.AnError
	})
	require.Error(t, err)

	// verify second feed was not inserted
	err = db.conn.Get(&count, `SELECT COUNT(*) FROM feeds`)
	require.NoError(t, err)
	assert.Equal(t, 1, count) // still only 1

	// test transaction error cases
	t.Run("transaction begin error", func(t *testing.T) {
		// close the connection to force an error
		db.Close()

		err := db.InTransaction(ctx, func(tx *sqlx.Tx) error {
			return nil
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "begin transaction")
	})
}
