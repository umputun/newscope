package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("create and get feed", func(t *testing.T) {
		feed := &Feed{
			URL:         "https://example.com/feed.xml",
			Title:       "Test Feed",
			Description: sql.NullString{String: "Test Description", Valid: true},
			Enabled:     true,
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

		// get feed by URL
		retrieved, err = db.GetFeedByURL(ctx, feed.URL)
		require.NoError(t, err)
		assert.Equal(t, feed.ID, retrieved.ID)
	})

	t.Run("get all feeds", func(t *testing.T) {
		// add another feed
		feed2 := &Feed{
			URL:     "https://example2.com/feed.xml",
			Title:   "Test Feed 2",
			Enabled: false,
		}
		err := db.CreateFeed(ctx, feed2)
		require.NoError(t, err)

		// get all feeds
		feeds, err := db.GetFeeds(ctx)
		require.NoError(t, err)
		assert.Len(t, feeds, 2)

		// get enabled feeds only
		enabled, err := db.GetEnabledFeeds(ctx)
		require.NoError(t, err)
		assert.Len(t, enabled, 1)
		assert.Equal(t, "Test Feed", enabled[0].Title)
	})

	t.Run("update feed", func(t *testing.T) {
		feed, err := db.GetFeed(ctx, 1)
		require.NoError(t, err)

		feed.Title = "Updated Feed"
		feed.LastFetched = sql.NullTime{Time: time.Now(), Valid: true}

		err = db.UpdateFeed(ctx, feed)
		require.NoError(t, err)

		updated, err := db.GetFeed(ctx, feed.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Feed", updated.Title)
		assert.True(t, updated.LastFetched.Valid)
	})

	t.Run("update feed fetch status", func(t *testing.T) {
		now := time.Now()
		err := db.UpdateFeedLastFetched(ctx, 1, now)
		require.NoError(t, err)

		feed, err := db.GetFeed(ctx, 1)
		require.NoError(t, err)
		assert.True(t, feed.LastFetched.Valid)
		assert.WithinDuration(t, now, feed.LastFetched.Time, time.Second)
		assert.Equal(t, 0, feed.ErrorCount)
	})

	t.Run("update feed error", func(t *testing.T) {
		err := db.UpdateFeedError(ctx, 1, "test error")
		require.NoError(t, err)

		feed, err := db.GetFeed(ctx, 1)
		require.NoError(t, err)
		assert.True(t, feed.LastError.Valid)
		assert.Equal(t, "test error", feed.LastError.String)
		assert.Equal(t, 1, feed.ErrorCount)
	})

	t.Run("delete feed", func(t *testing.T) {
		err := db.DeleteFeed(ctx, 2)
		require.NoError(t, err)

		_, err = db.GetFeed(ctx, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("get feeds with stats", func(t *testing.T) {
		// add some items to feed 1
		item1 := &Item{FeedID: 1, GUID: "item-1", Title: "Item 1", Link: "http://example.com/1"}
		err := db.CreateItem(ctx, item1)
		require.NoError(t, err)

		item2 := &Item{FeedID: 1, GUID: "item-2", Title: "Item 2", Link: "http://example.com/2"}
		err = db.CreateItem(ctx, item2)
		require.NoError(t, err)

		item3 := &Item{FeedID: 1, GUID: "item-3", Title: "Item 3", Link: "http://example.com/3"}
		err = db.CreateItem(ctx, item3)
		require.NoError(t, err)

		// mark one as extracted
		err = db.UpdateItemContentExtracted(ctx, item1.ID)
		require.NoError(t, err)

		// get feeds with stats
		feedsWithStats, err := db.GetFeedsWithStats(ctx)
		require.NoError(t, err)
		assert.Len(t, feedsWithStats, 1) // only feed 1 remains

		stats := feedsWithStats[0]
		assert.Equal(t, int64(1), stats.ID)
		assert.Equal(t, 3, stats.ItemCount)
		assert.Equal(t, 2, stats.UnreadCount)    // 2 items without content extracted
		assert.Equal(t, 1, stats.ExtractedCount) // 1 item with content extracted
	})

	t.Run("get feed by URL not found", func(t *testing.T) {
		_, err := db.GetFeedByURL(ctx, "https://nonexistent.com/feed.xml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("delete non-existent feed", func(t *testing.T) {
		err := db.DeleteFeed(ctx, 9999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}
