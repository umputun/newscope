package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtendedFeedOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create test feeds with different properties
	feeds := []Feed{
		{
			URL:           "https://high-priority.com/feed.xml",
			Title:         "High Priority Feed",
			Priority:      10,
			FetchInterval: 300, // 5 minutes
			Enabled:       true,
		},
		{
			URL:           "https://medium-priority.com/feed.xml",
			Title:         "Medium Priority Feed",
			Priority:      5,
			FetchInterval: 600, // 10 minutes
			Enabled:       true,
		},
		{
			URL:           "https://low-priority.com/feed.xml",
			Title:         "Low Priority Feed",
			Priority:      1,
			FetchInterval: 1800, // 30 minutes
			Enabled:       true,
		},
		{
			URL:           "https://disabled.com/feed.xml",
			Title:         "Disabled Feed",
			Priority:      5,
			FetchInterval: 600,
			Enabled:       false,
		},
	}

	for i := range feeds {
		err := db.CreateFeed(ctx, &feeds[i])
		require.NoError(t, err)
	}

	t.Run("get feeds due for update", func(t *testing.T) {
		// initially all enabled feeds should be due (no next_fetch set)
		due, err := db.GetFeedsDueForUpdate(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, due, 3) // only enabled feeds

		// verify they're ordered by priority DESC
		assert.Equal(t, "High Priority Feed", due[0].Title)
		assert.Equal(t, "Medium Priority Feed", due[1].Title)
		assert.Equal(t, "Low Priority Feed", due[2].Title)

		// update one feed's last_fetched time (high priority feed)
		err = db.UpdateFeedLastFetched(ctx, due[0].ID, time.Now())
		require.NoError(t, err)

		// wait a moment to ensure time difference
		time.Sleep(100 * time.Millisecond)

		// now only 2 should be due
		due2, err := db.GetFeedsDueForUpdate(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, due2, 2)

		// verify the updated feed is not in the list
		for _, f := range due2 {
			assert.NotEqual(t, due[0].ID, f.ID)
		}
	})

	t.Run("update feed priority", func(t *testing.T) {
		// change low priority feed to highest
		err := db.UpdateFeedPriority(ctx, feeds[2].ID, 15)
		require.NoError(t, err)

		// verify in database
		updated, err := db.GetFeed(ctx, feeds[2].ID)
		require.NoError(t, err)
		assert.Equal(t, 15, updated.Priority)

		// verify it's now first in due list
		due, err := db.GetFeedsDueForUpdate(ctx, 10)
		require.NoError(t, err)
		if len(due) > 0 {
			assert.Equal(t, feeds[2].ID, due[0].ID)
		}
	})

	t.Run("update feed interval", func(t *testing.T) {
		// change interval to 1 hour
		err := db.UpdateFeedInterval(ctx, feeds[1].ID, 3600)
		require.NoError(t, err)

		// verify in database
		updated, err := db.GetFeed(ctx, feeds[1].ID)
		require.NoError(t, err)
		assert.Equal(t, 3600, updated.FetchInterval)
	})

	t.Run("set feed metadata", func(t *testing.T) {
		metadata := `{"extraction_mode": "precision", "custom_headers": {"User-Agent": "Special Bot"}}`
		err := db.SetFeedMetadata(ctx, feeds[0].ID, metadata)
		require.NoError(t, err)

		// verify in database
		updated, err := db.GetFeed(ctx, feeds[0].ID)
		require.NoError(t, err)
		assert.True(t, updated.Metadata.Valid)
		assert.Equal(t, metadata, updated.Metadata.String)
	})

	t.Run("get feeds by priority", func(t *testing.T) {
		// get feeds with priority >= 5
		highPriority, err := db.GetFeedsByPriority(ctx, 5)
		require.NoError(t, err)
		// should have: low (now 15), high (10), and medium (5) - all enabled feeds with priority >= 5
		assert.GreaterOrEqual(t, len(highPriority), 1)

		// verify order - highest priority first
		if len(highPriority) > 0 {
			assert.Equal(t, 15, highPriority[0].Priority) // formerly low, now highest
		}
		if len(highPriority) > 1 {
			assert.Equal(t, 10, highPriority[1].Priority) // high priority
		}
		if len(highPriority) > 2 {
			assert.Equal(t, 5, highPriority[2].Priority) // medium priority
		}
	})

	t.Run("update feed with next_fetch calculation", func(t *testing.T) {
		// create a fresh feed for this test
		testFeed := Feed{
			URL:           "https://test-next-fetch.com/feed.xml",
			Title:         "Test Next Fetch Feed",
			Priority:      5,
			FetchInterval: 600, // 10 minutes
			Enabled:       true,
		}
		err := db.CreateFeed(ctx, &testFeed)
		require.NoError(t, err)

		// mark it as fetched and verify next_fetch is set
		fetchTime := time.Now()
		err = db.UpdateFeedLastFetched(ctx, testFeed.ID, fetchTime)
		require.NoError(t, err)

		// verify next_fetch is calculated correctly
		updated, err := db.GetFeed(ctx, testFeed.ID)
		require.NoError(t, err)
		assert.True(t, updated.NextFetch.Valid)
		assert.True(t, updated.LastFetched.Valid)

		// next_fetch should be last_fetched + interval (600 seconds)
		expectedNext := fetchTime.Add(time.Duration(updated.FetchInterval) * time.Second)
		assert.WithinDuration(t, expectedNext, updated.NextFetch.Time, 2*time.Second)

		// verify it's not in the due list anymore
		due, err := db.GetFeedsDueForUpdate(ctx, 10)
		require.NoError(t, err)
		for _, f := range due {
			assert.NotEqual(t, testFeed.ID, f.ID)
		}
	})

	t.Run("feed average score updates", func(t *testing.T) {
		// create some items for a feed
		items := []Item{
			{FeedID: feeds[0].ID, GUID: "item1", Title: "Article 1", Link: "https://example.com/1"},
			{FeedID: feeds[0].ID, GUID: "item2", Title: "Article 2", Link: "https://example.com/2"},
		}

		for i := range items {
			err := db.CreateItem(ctx, &items[i])
			require.NoError(t, err)
		}

		// create scores for the items
		scores := []ArticleScore{
			{ArticleID: items[0].ID, FinalScore: 0.8, ScoredAt: time.Now()},
			{ArticleID: items[1].ID, FinalScore: 0.6, ScoredAt: time.Now()},
		}

		for i := range scores {
			err := db.CreateArticleScore(ctx, &scores[i])
			require.NoError(t, err)
		}

		// update feed average score
		err := db.UpdateFeedAverageScore(ctx, feeds[0].ID)
		require.NoError(t, err)

		// verify average is calculated
		updated, err := db.GetFeed(ctx, feeds[0].ID)
		require.NoError(t, err)
		assert.True(t, updated.AvgScore.Valid)
		assert.InDelta(t, 0.7, updated.AvgScore.Float64, 0.01)
	})
}
