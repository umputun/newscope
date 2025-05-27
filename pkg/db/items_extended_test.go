package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtendedItemOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create test feed
	feed := &Feed{
		URL:   "https://example.com/feed.xml",
		Title: "Test Feed",
	}
	err := db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// create test items
	items := []Item{
		{
			FeedID:      feed.ID,
			GUID:        "item1",
			Title:       "Article 1",
			Link:        "https://example.com/1",
			Description: sql.NullString{String: "Description 1", Valid: true},
			Published:   sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
		},
		{
			FeedID:      feed.ID,
			GUID:        "item2",
			Title:       "Article 2",
			Link:        "https://example.com/2",
			Description: sql.NullString{String: "Description 2", Valid: true},
			Published:   sql.NullTime{Time: time.Now().Add(-2 * time.Hour), Valid: true},
		},
		{
			FeedID:    feed.ID,
			GUID:      "item3",
			Title:     "Article 3",
			Link:      "https://example.com/3",
			Published: sql.NullTime{Time: time.Now().Add(-10 * 24 * time.Hour), Valid: true}, // 10 days old
		},
	}

	for i := range items {
		err = db.CreateItem(ctx, &items[i])
		require.NoError(t, err)
	}

	t.Run("update item content", func(t *testing.T) {
		err := db.UpdateItemContent(ctx, items[0].ID,
			"This is the full article content with lots of text...",
			"<p>This is the <strong>HTML</strong> content</p>",
			"en",
			"trafilatura",
			"balanced",
			5, // 5 minute read
			3, // 3 images
		)
		require.NoError(t, err)

		// verify update
		updated, err := db.GetItem(ctx, items[0].ID)
		require.NoError(t, err)
		assert.True(t, updated.ContentExtracted)
		assert.True(t, updated.Content.Valid)
		assert.Equal(t, "This is the full article content with lots of text...", updated.Content.String)
		assert.True(t, updated.ContentHTML.Valid)
		assert.True(t, updated.ContentHash.Valid)
		assert.NotEmpty(t, updated.ContentHash.String)
		assert.Equal(t, "en", updated.Language.String)
		assert.Equal(t, int64(5), updated.ReadTime.Int64)
		assert.Equal(t, int64(3), updated.MediaCount.Int64)
		assert.Equal(t, "trafilatura", updated.ExtractionMethod.String)
		assert.Equal(t, "balanced", updated.ExtractionMode.String)
	})

	t.Run("get items by content hash", func(t *testing.T) {
		// update another item with the same content (duplicate)
		err := db.UpdateItemContent(ctx, items[1].ID,
			"This is the full article content with lots of text...", // same content
			"<p>Different HTML</p>",
			"en",
			"trafilatura",
			"balanced",
			5, 3,
		)
		require.NoError(t, err)

		// get first item to find its hash
		item, err := db.GetItem(ctx, items[0].ID)
		require.NoError(t, err)
		require.True(t, item.ContentHash.Valid)

		// find items with same hash
		duplicates, err := db.GetItemsByContentHash(ctx, item.ContentHash.String)
		require.NoError(t, err)
		assert.Len(t, duplicates, 2)

		// verify they're ordered by published DESC
		assert.Equal(t, items[0].ID, duplicates[0].ID) // newer
		assert.Equal(t, items[1].ID, duplicates[1].ID) // older
	})

	t.Run("get items without content", func(t *testing.T) {
		// only item3 should not have content and be recent enough
		itemsWithoutContent, err := db.GetItemsWithoutContent(ctx, 10)
		require.NoError(t, err)
		assert.Empty(t, itemsWithoutContent) // item3 is too old (10 days)

		// create a recent item without content
		newItem := &Item{
			FeedID:    feed.ID,
			GUID:      "item4",
			Title:     "Recent Article",
			Link:      "https://example.com/4",
			Published: sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
		}
		err = db.CreateItem(ctx, newItem)
		require.NoError(t, err)

		// now we should get it
		itemsWithoutContent, err = db.GetItemsWithoutContent(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, itemsWithoutContent, 1)
		assert.Equal(t, newItem.ID, itemsWithoutContent[0].ID)
	})

	t.Run("get items by language", func(t *testing.T) {
		// update item language using UpdateItemContent
		err := db.UpdateItemContent(ctx, items[2].ID,
			"French content", "<p>French content</p>", "fr",
			"test", "test", 2, 0)
		require.NoError(t, err)

		// get English items
		enItems, err := db.GetItemsByLanguage(ctx, "en", 10, 0)
		require.NoError(t, err)
		assert.Len(t, enItems, 2)

		// get French items
		frItems, err := db.GetItemsByLanguage(ctx, "fr", 10, 0)
		require.NoError(t, err)
		assert.Len(t, frItems, 1)
		assert.Equal(t, items[2].ID, frItems[0].ID)
	})

	t.Run("search items full text", func(t *testing.T) {
		// search for "article"
		results, err := db.SearchItemsFullText(ctx, "article", 10, 0)
		require.NoError(t, err)
		// should find items with "Article" in title
		assert.GreaterOrEqual(t, len(results), 3)
	})

	t.Run("get item stats", func(t *testing.T) {
		stats, err := db.GetItemStats(ctx)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, stats["total_items"].(int64), int64(4))
		assert.GreaterOrEqual(t, stats["items_with_content"].(int64), int64(2))

		if avgReadTime, ok := stats["avg_read_time_minutes"]; ok {
			assert.InDelta(t, 4.0, avgReadTime, 0.1)
		}

		languages := stats["items_by_language"].(map[string]int64)
		assert.Equal(t, int64(2), languages["en"])
		assert.Equal(t, int64(1), languages["fr"])

		methods := stats["extraction_methods"].(map[string]int64)
		assert.Equal(t, int64(2), methods["trafilatura"])
	})

	t.Run("get duplicate items", func(t *testing.T) {
		duplicates, err := db.GetDuplicateItems(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, duplicates, 1) // one set of duplicates

		dup := duplicates[0]
		assert.Equal(t, 2, dup.Count)
		assert.Len(t, dup.Items, 2)
		assert.NotEmpty(t, dup.ContentHash)
	})

	t.Run("cleanup old items", func(t *testing.T) {
		// create an old item
		oldItem := &Item{
			FeedID:    feed.ID,
			GUID:      "old-item",
			Title:     "Old Article",
			Link:      "https://example.com/old",
			Published: sql.NullTime{Time: time.Now().Add(-60 * 24 * time.Hour), Valid: true}, // 60 days old
		}
		err := db.CreateItem(ctx, oldItem)
		require.NoError(t, err)

		// cleanup items older than 30 days
		deleted, err := db.CleanupOldItems(ctx, 30*24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, int64(1), deleted) // should delete the 60-day old item

		// verify it's gone
		item, err := db.GetItem(ctx, oldItem.ID)
		require.Error(t, err)
		assert.Nil(t, item)

		// verify 10-day old item is still there
		item3Still, err := db.GetItem(ctx, items[2].ID)
		require.NoError(t, err)
		assert.NotNil(t, item3Still)
	})
}
