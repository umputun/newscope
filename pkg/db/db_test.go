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

// setupTestDB creates a test database
func setupTestDB(t *testing.T) (db *DB, cleanup func()) {
	t.Helper()

	// create temp file for test database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	tmpfile.Close()

	cfg := Config{
		DSN:          tmpfile.Name(),
		MaxOpenConns: 1,
	}

	ctx := context.Background()
	db, err = New(ctx, cfg)
	require.NoError(t, err)

	cleanup = func() {
		db.Close()
		os.Remove(tmpfile.Name())
	}

	return db, cleanup
}

func TestFeedOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("create and get feed", func(t *testing.T) {
		feed := &Feed{
			URL:           "https://example.com/feed.xml",
			Title:         "Test Feed",
			Description:   "Test Description",
			FetchInterval: 3600,
			Enabled:       true,
		}

		// create feed
		err := db.CreateFeed(ctx, feed)
		require.NoError(t, err)
		assert.NotZero(t, feed.ID)

		// get feed by ID
		retrieved, err := db.GetFeed(ctx, feed.ID)
		require.NoError(t, err)
		assert.Equal(t, feed.URL, retrieved.URL)
		assert.Equal(t, feed.Title, retrieved.Title)
		assert.Equal(t, feed.Description, retrieved.Description)
		assert.Equal(t, feed.FetchInterval, retrieved.FetchInterval)
		assert.Equal(t, feed.Enabled, retrieved.Enabled)
	})

	t.Run("get all feeds", func(t *testing.T) {
		// add another feed
		feed2 := &Feed{
			URL:           "https://example2.com/feed.xml",
			Title:         "Test Feed 2",
			FetchInterval: 1800,
			Enabled:       false,
		}
		err := db.CreateFeed(ctx, feed2)
		require.NoError(t, err)

		// get all feeds
		feeds, err := db.GetFeeds(ctx, false)
		require.NoError(t, err)
		assert.Len(t, feeds, 2)

		// get enabled feeds only
		enabled, err := db.GetFeeds(ctx, true)
		require.NoError(t, err)
		assert.Len(t, enabled, 1)
		assert.Equal(t, "Test Feed", enabled[0].Title)
	})

	t.Run("update feed after fetch", func(t *testing.T) {
		feed, err := db.GetFeed(ctx, 1)
		require.NoError(t, err)

		nextFetch := time.Now().Add(time.Hour)
		err = db.UpdateFeedFetched(ctx, feed.ID, nextFetch)
		require.NoError(t, err)

		updated, err := db.GetFeed(ctx, feed.ID)
		require.NoError(t, err)
		assert.NotNil(t, updated.LastFetched)
		assert.NotNil(t, updated.NextFetch)
		assert.WithinDuration(t, nextFetch, *updated.NextFetch, time.Second)
		assert.Equal(t, 0, updated.ErrorCount)
		assert.Empty(t, updated.LastError)
	})

	t.Run("update feed error", func(t *testing.T) {
		err := db.UpdateFeedError(ctx, 1, "test error")
		require.NoError(t, err)

		feed, err := db.GetFeed(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, 1, feed.ErrorCount)
		assert.Equal(t, "test error", feed.LastError)
	})

	t.Run("get feeds to fetch", func(t *testing.T) {
		// set one feed to need fetching
		_, err := db.ExecContext(ctx, "UPDATE feeds SET next_fetch = datetime('now', '-1 hour') WHERE id = 1")
		require.NoError(t, err)

		feeds, err := db.GetFeedsToFetch(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, feeds, 1)
		assert.Equal(t, int64(1), feeds[0].ID)
	})

	t.Run("delete feed", func(t *testing.T) {
		err := db.DeleteFeed(ctx, 2)
		require.NoError(t, err)

		feeds, err := db.GetFeeds(ctx, false)
		require.NoError(t, err)
		assert.Len(t, feeds, 1)
	})
}

func TestItemOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create a feed first
	feed := &Feed{
		URL:     "https://example.com/feed.xml",
		Title:   "Test Feed",
		Enabled: true,
	}
	err := db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	t.Run("create and get item", func(t *testing.T) {
		item := &Item{
			FeedID:      feed.ID,
			GUID:        "unique-guid-123",
			Title:       "Test Article",
			Link:        "https://example.com/article",
			Description: "Test description",
			Content:     "Test content",
			Author:      "Test Author",
			Published:   time.Now(),
		}

		// create item
		err := db.CreateItem(ctx, item)
		require.NoError(t, err)
		assert.NotZero(t, item.ID)

		// get item by ID
		retrieved, err := db.GetItem(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, item.GUID, retrieved.GUID)
		assert.Equal(t, item.Title, retrieved.Title)
		assert.Equal(t, item.Link, retrieved.Link)
		assert.Equal(t, item.Description, retrieved.Description)
		assert.Equal(t, item.Content, retrieved.Content)
		assert.Equal(t, item.Author, retrieved.Author)
		assert.WithinDuration(t, item.Published, retrieved.Published, time.Second)
	})

	t.Run("check item exists", func(t *testing.T) {
		exists, err := db.ItemExists(ctx, feed.ID, "unique-guid-123")
		require.NoError(t, err)
		assert.True(t, exists)

		exists, err = db.ItemExists(ctx, feed.ID, "non-existent-guid")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("get items needing extraction", func(t *testing.T) {
		// create item needing extraction
		item2 := &Item{
			FeedID:    feed.ID,
			GUID:      "guid-needs-extraction",
			Title:     "Needs Extraction",
			Link:      "https://example.com/article2",
			Published: time.Now(),
		}
		err := db.CreateItem(ctx, item2)
		require.NoError(t, err)

		items, err := db.GetItemsNeedingExtraction(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, items, 2)
	})

	t.Run("update item extraction", func(t *testing.T) {
		// successful extraction
		err := db.UpdateItemExtraction(ctx, 1, "Extracted full content", "<p>Rich content</p>", nil)
		require.NoError(t, err)

		item, err := db.GetItem(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, "Extracted full content", item.ExtractedContent)
		assert.NotNil(t, item.ExtractedAt)
		assert.Empty(t, item.ExtractionError)

		// failed extraction
		err = db.UpdateItemExtraction(ctx, 2, "", "", assert.AnError)
		require.NoError(t, err)

		item, err = db.GetItem(ctx, 2)
		require.NoError(t, err)
		assert.Empty(t, item.ExtractedContent)
		assert.NotNil(t, item.ExtractedAt)
		assert.NotEmpty(t, item.ExtractionError)
	})

	t.Run("get unclassified items", func(t *testing.T) {
		items, err := db.GetUnclassifiedItems(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, items, 1) // only item 1 has extracted content
		assert.Equal(t, int64(1), items[0].ID)
	})

	t.Run("update item classification", func(t *testing.T) {
		err := db.UpdateItemClassification(ctx, 1, 8.5, "Highly relevant to interests", []string{"golang", "programming"})
		require.NoError(t, err)

		item, err := db.GetItem(ctx, 1)
		require.NoError(t, err)
		assert.InEpsilon(t, 8.5, item.RelevanceScore, 0.001)
		assert.Equal(t, "Highly relevant to interests", item.Explanation)
		assert.Equal(t, Topics{"golang", "programming"}, item.Topics)
		assert.NotNil(t, item.ClassifiedAt)
	})

	t.Run("get items with min score", func(t *testing.T) {
		items, err := db.GetItems(ctx, 10, 5.0)
		require.NoError(t, err)
		assert.Len(t, items, 1)
		assert.Equal(t, int64(1), items[0].ID)
	})

	t.Run("update item feedback", func(t *testing.T) {
		err := db.UpdateItemFeedback(ctx, 1, "like")
		require.NoError(t, err)

		item, err := db.GetItem(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, "like", item.UserFeedback)
		assert.NotNil(t, item.FeedbackAt)
	})

	t.Run("get recent feedback", func(t *testing.T) {
		examples, err := db.GetRecentFeedback(ctx, "like", 10)
		require.NoError(t, err)
		assert.Len(t, examples, 1)
		assert.Equal(t, "Test Article", examples[0].Title)
		assert.Equal(t, "Test description", examples[0].Description)
		assert.Equal(t, "like", examples[0].Feedback)
		assert.Equal(t, []string{"golang", "programming"}, examples[0].Topics)
	})

	t.Run("batch update classifications", func(t *testing.T) {
		// create more items
		item3 := &Item{
			FeedID:           feed.ID,
			GUID:             "guid-3",
			Title:            "Article 3",
			Link:             "https://example.com/article3",
			Published:        time.Now(),
			ExtractedContent: "Content 3",
		}
		err := db.CreateItem(ctx, item3)
		require.NoError(t, err)

		classifications := []Classification{
			{GUID: "guid-3", Score: 7.0, Explanation: "Good match", Topics: []string{"tech"}},
			{GUID: "guid-needs-extraction", Score: 3.0, Explanation: "Not relevant", Topics: []string{"other"}},
		}

		itemsByGUID := map[string]int64{
			"guid-3":                3,
			"guid-needs-extraction": 2,
		}

		err = db.UpdateClassifications(ctx, classifications, itemsByGUID)
		require.NoError(t, err)

		// verify updates
		item, err := db.GetItem(ctx, 3)
		require.NoError(t, err)
		assert.InEpsilon(t, 7.0, item.RelevanceScore, 0.001)
		assert.Equal(t, "Good match", item.Explanation)
	})

	t.Run("get classified items", func(t *testing.T) {
		// get all classified items with score >= 7
		items, err := db.GetClassifiedItems(ctx, 7.0, 10)
		require.NoError(t, err)
		
		// find item3 in results
		var item3Found bool
		for _, item := range items {
			if item.Title == "Article 3" {
				item3Found = true
				assert.Equal(t, "Test Feed", item.FeedTitle)
				assert.InEpsilon(t, 7.0, item.RelevanceScore, 0.001)
			}
		}
		assert.True(t, item3Found, "Article 3 should be in results")

		// get classified items with lower score to include more items
		items, err = db.GetClassifiedItems(ctx, 3.0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(items), 2) // at least item3 and item2 are classified
	})

	t.Run("get single classified item", func(t *testing.T) {
		// get item3 with feed info
		item, err := db.GetClassifiedItem(ctx, 3)
		require.NoError(t, err)
		assert.Equal(t, "Article 3", item.Title)
		assert.Equal(t, "Test Feed", item.FeedTitle)
		assert.InEpsilon(t, 7.0, item.RelevanceScore, 0.001)

		// try to get non-existent item
		_, err = db.GetClassifiedItem(ctx, 99999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("get classified item with rich content", func(t *testing.T) {
		// update item with rich content
		err := db.UpdateItemExtraction(ctx, 3, "Plain content", "<p>Rich content</p>", nil)
		require.NoError(t, err)

		// get item and verify both content fields
		item, err := db.GetClassifiedItem(ctx, 3)
		require.NoError(t, err)
		assert.Equal(t, "Plain content", item.ExtractedContent)
		assert.Equal(t, "<p>Rich content</p>", item.ExtractedRichContent)
	})
}

func TestSettingOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("set and get setting", func(t *testing.T) {
		// get non-existent setting
		value, err := db.GetSetting(ctx, "interests")
		require.NoError(t, err)
		assert.Empty(t, value)

		// set setting
		err = db.SetSetting(ctx, "interests", "golang, distributed systems, databases")
		require.NoError(t, err)

		// get setting
		value, err = db.GetSetting(ctx, "interests")
		require.NoError(t, err)
		assert.Equal(t, "golang, distributed systems, databases", value)

		// update setting
		err = db.SetSetting(ctx, "interests", "updated interests")
		require.NoError(t, err)

		value, err = db.GetSetting(ctx, "interests")
		require.NoError(t, err)
		assert.Equal(t, "updated interests", value)
	})
}

func TestTransactions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("successful transaction", func(t *testing.T) {
		var feedID int64
		err := db.InTransaction(ctx, func(tx *sqlx.Tx) error {
			result, err := tx.ExecContext(ctx, "INSERT INTO feeds (url, title, enabled) VALUES (?, ?, ?)",
				"https://tx.example.com", "TX Feed", true)
			if err != nil {
				return err
			}
			feedID, _ = result.LastInsertId()
			return nil
		})
		require.NoError(t, err)
		assert.NotZero(t, feedID)

		// verify feed was created
		feed, err := db.GetFeed(ctx, feedID)
		require.NoError(t, err)
		assert.Equal(t, "TX Feed", feed.Title)
	})

	t.Run("rolled back transaction", func(t *testing.T) {
		err := db.InTransaction(ctx, func(tx *sqlx.Tx) error {
			_, err := tx.ExecContext(ctx, "INSERT INTO feeds (url, title, enabled) VALUES (?, ?, ?)",
				"https://rollback.example.com", "Rollback Feed", true)
			if err != nil {
				return err
			}
			return assert.AnError // force rollback
		})
		require.Error(t, err)

		// verify feed was not created
		var count int
		err = db.GetContext(ctx, &count, "SELECT COUNT(*) FROM feeds WHERE url = ?", "https://rollback.example.com")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestDatabaseConnection(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("ping", func(t *testing.T) {
		err := db.Ping(ctx)
		require.NoError(t, err)
	})

	t.Run("close and reopen", func(t *testing.T) {
		err := db.Close()
		require.NoError(t, err)

		// ping should fail after close
		err = db.Ping(ctx)
		require.Error(t, err)
	})
}
