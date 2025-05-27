package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestItemOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create a test feed first
	feed := &Feed{
		URL:   "https://example.com/feed.xml",
		Title: "Test Feed",
	}
	err := db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	t.Run("create and get item", func(t *testing.T) {
		item := &Item{
			FeedID:      feed.ID,
			GUID:        "unique-guid-1",
			Title:       "Test Article",
			Link:        "https://example.com/article1",
			Description: sql.NullString{String: "Test description", Valid: true},
			Published:   sql.NullTime{Time: time.Now(), Valid: true},
			Author:      sql.NullString{String: "Test Author", Valid: true},
		}

		// create item
		err := db.CreateItem(ctx, item)
		require.NoError(t, err)
		assert.NotZero(t, item.ID)

		// get item
		retrieved, err := db.GetItem(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, item.Title, retrieved.Title)
		assert.Equal(t, item.GUID, retrieved.GUID)

		// get by GUID
		retrieved, err = db.GetItemByGUID(ctx, feed.ID, item.GUID)
		require.NoError(t, err)
		assert.Equal(t, item.ID, retrieved.ID)

		// get by non-existent GUID
		_, err = db.GetItemByGUID(ctx, feed.ID, "non-existent-guid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("create duplicate item", func(t *testing.T) {
		duplicate := &Item{
			FeedID: feed.ID,
			GUID:   "unique-guid-1", // same GUID
			Title:  "Duplicate Article",
			Link:   "https://example.com/duplicate",
		}

		// should not error due to ON CONFLICT clause
		err := db.CreateItem(ctx, duplicate)
		require.NoError(t, err)
		assert.Zero(t, duplicate.ID) // ID should not be set for duplicate

		// verify only one item exists
		count, err := db.CountItemsByFeed(ctx, feed.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("create multiple items", func(t *testing.T) {
		items := []Item{
			{
				FeedID: feed.ID,
				GUID:   "guid-2",
				Title:  "Article 2",
				Link:   "https://example.com/article2",
			},
			{
				FeedID: feed.ID,
				GUID:   "guid-3",
				Title:  "Article 3",
				Link:   "https://example.com/article3",
			},
		}

		err := db.CreateItems(ctx, items)
		require.NoError(t, err)

		count, err := db.CountItemsByFeed(ctx, feed.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})

	t.Run("get items with pagination", func(t *testing.T) {
		// get first page
		items, err := db.GetItems(ctx, 2, 0)
		require.NoError(t, err)
		assert.Len(t, items, 2)

		// get second page
		items, err = db.GetItems(ctx, 2, 2)
		require.NoError(t, err)
		assert.Len(t, items, 1)

		// get items by feed
		items, err = db.GetItemsByFeed(ctx, feed.ID, 10, 0)
		require.NoError(t, err)
		assert.Len(t, items, 3)
	})

	t.Run("get items for extraction", func(t *testing.T) {
		items, err := db.GetItemsForExtraction(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, items, 3) // all items don't have content yet
	})

	t.Run("update item content extracted", func(t *testing.T) {
		err := db.UpdateItemContentExtracted(ctx, 1)
		require.NoError(t, err)

		item, err := db.GetItem(ctx, 1)
		require.NoError(t, err)
		assert.True(t, item.ContentExtracted)

		// should not appear in extraction list anymore
		items, err := db.GetItemsForExtraction(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, items, 2)
	})

	t.Run("search items", func(t *testing.T) {
		// search should work even without content
		results, err := db.SearchItems(ctx, "Article", 10, 0)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		// search for specific article
		results, err = db.SearchItems(ctx, "Article 2", 10, 0)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Article 2", results[0].Title)
	})

	t.Run("delete old items", func(t *testing.T) {
		// create an old item
		oldItem := &Item{
			FeedID: feed.ID,
			GUID:   "old-guid",
			Title:  "Old Article",
			Link:   "https://example.com/old",
		}
		err := db.CreateItem(ctx, oldItem)
		require.NoError(t, err)

		// manually update created_at to be old
		_, err = db.conn.Exec(
			"UPDATE items SET created_at = ? WHERE id = ?",
			time.Now().Add(-48*time.Hour),
			oldItem.ID,
		)
		require.NoError(t, err)

		// delete items older than 24 hours
		deleted, err := db.DeleteOldItems(ctx, 24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, int64(1), deleted)

		count, err := db.CountItems(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count) // only 3 remain
	})

	t.Run("delete item", func(t *testing.T) {
		err := db.DeleteItem(ctx, 1)
		require.NoError(t, err)

		_, err = db.GetItem(ctx, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")

		// delete non-existent item
		err = db.DeleteItem(ctx, 9999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("get item with content not found", func(t *testing.T) {
		_, err := db.GetItemWithContent(ctx, 9999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}
