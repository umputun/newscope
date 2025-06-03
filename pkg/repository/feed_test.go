package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/domain"
)

func TestFeedRepository_GetFeedsToFetch(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)

	// create feeds with different fetch times - GetFeedsToFetch uses next_fetch field
	recentFeed := &domain.Feed{
		URL:           "https://example.com/recent.xml",
		Title:         "Recent Feed",
		Description:   "Recently fetched feed",
		FetchInterval: 3600, // 1 hour
		Enabled:       true,
	}

	oldFeed := &domain.Feed{
		URL:           "https://example.com/old.xml",
		Title:         "Old Feed",
		Description:   "Old feed that needs fetching",
		FetchInterval: 3600, // 1 hour
		Enabled:       true,
	}

	disabledFeed := &domain.Feed{
		URL:           "https://example.com/disabled.xml",
		Title:         "Disabled Feed",
		Description:   "Disabled feed",
		FetchInterval: 3600,
		Enabled:       false,
	}

	neverFetchedFeed := &domain.Feed{
		URL:           "https://example.com/never.xml",
		Title:         "Never Fetched Feed",
		Description:   "Never fetched feed",
		FetchInterval: 3600,
		Enabled:       true,
	}

	// create feeds
	feeds := []*domain.Feed{recentFeed, oldFeed, disabledFeed, neverFetchedFeed}
	for _, feed := range feeds {
		err := repos.Feed.CreateFeed(context.Background(), feed)
		require.NoError(t, err)
	}

	// set fetch times after creation using UpdateFeedFetched
	err := repos.Feed.UpdateFeedFetched(context.Background(), recentFeed.ID, future)
	require.NoError(t, err)
	err = repos.Feed.UpdateFeedFetched(context.Background(), oldFeed.ID, past)
	require.NoError(t, err)
	err = repos.Feed.UpdateFeedFetched(context.Background(), disabledFeed.ID, past)
	require.NoError(t, err)
	// neverFetchedFeed is left with NULL next_fetch

	t.Run("get feeds that need fetching", func(t *testing.T) {
		feedsToFetch, err := repos.Feed.GetFeedsToFetch(context.Background(), 10)
		require.NoError(t, err)

		// should return old feed and never-fetched feed (but not recent or disabled)
		assert.Len(t, feedsToFetch, 2)

		guidMap := make(map[string]bool)
		for _, feed := range feedsToFetch {
			guidMap[feed.URL] = true
		}

		assert.True(t, guidMap["https://example.com/old.xml"])
		assert.True(t, guidMap["https://example.com/never.xml"])
		assert.False(t, guidMap["https://example.com/recent.xml"])
		assert.False(t, guidMap["https://example.com/disabled.xml"])
	})
}

func TestFeedRepository_UpdateFeedFetched(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := &domain.Feed{
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		Description:   "Feed for testing UpdateFeedFetched",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err := repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	t.Run("update feed fetched", func(t *testing.T) {
		beforeUpdate := time.Now()
		nextFetch := time.Now().Add(1 * time.Hour)

		err := repos.Feed.UpdateFeedFetched(context.Background(), testFeed.ID, nextFetch)
		require.NoError(t, err)

		// verify the update
		updatedFeed, err := repos.Feed.GetFeed(context.Background(), testFeed.ID)
		require.NoError(t, err)

		// last_fetched should be set to current time (close to beforeUpdate)
		assert.NotNil(t, updatedFeed.LastFetched)
		timeDiff := updatedFeed.LastFetched.Sub(beforeUpdate)
		assert.True(t, timeDiff >= -time.Second && timeDiff <= time.Second, "LastFetched should be close to beforeUpdate")

		// next_fetch should be set to the provided nextFetch time
		assert.NotNil(t, updatedFeed.NextFetch)
		// allow small time difference due to processing time
		nextFetchDiff := updatedFeed.NextFetch.Sub(nextFetch)
		assert.True(t, nextFetchDiff >= -time.Second && nextFetchDiff <= time.Second, "NextFetch time should be close to expected")
	})

	t.Run("update non-existent feed", func(t *testing.T) {
		nextFetch := time.Now().Add(1 * time.Hour)
		err := repos.Feed.UpdateFeedFetched(context.Background(), 99999, nextFetch)
		assert.NoError(t, err) // sQLite doesn't error on UPDATE with no matches
	})
}

func TestFeedRepository_UpdateFeedError(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := &domain.Feed{
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		Description:   "Feed for testing UpdateFeedError",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err := repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	t.Run("update feed error", func(t *testing.T) {
		errorMsg := "Failed to fetch feed: connection timeout"

		// get initial error count
		initialFeed, err := repos.Feed.GetFeed(context.Background(), testFeed.ID)
		require.NoError(t, err)
		initialErrorCount := initialFeed.ErrorCount

		err = repos.Feed.UpdateFeedError(context.Background(), testFeed.ID, errorMsg)
		require.NoError(t, err)

		// verify the update
		updatedFeed, err := repos.Feed.GetFeed(context.Background(), testFeed.ID)
		require.NoError(t, err)

		assert.Equal(t, errorMsg, updatedFeed.LastError)
		assert.Equal(t, initialErrorCount+1, updatedFeed.ErrorCount)
	})

	t.Run("clear feed error", func(t *testing.T) {
		err := repos.Feed.UpdateFeedError(context.Background(), testFeed.ID, "")
		require.NoError(t, err)

		// verify the error was cleared
		updatedFeed, err := repos.Feed.GetFeed(context.Background(), testFeed.ID)
		require.NoError(t, err)

		assert.Empty(t, updatedFeed.LastError)
	})
}

func TestFeedRepository_UpdateFeed(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create a test feed
	testFeed := &domain.Feed{
		URL:           "https://example.com/test-feed.xml",
		Title:         "Original Title",
		Description:   "Feed for testing UpdateFeed",
		FetchInterval: 1800, // 30 minutes
		Enabled:       true,
	}

	err := repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	t.Run("update feed title and interval", func(t *testing.T) {
		newTitle := "Updated Title"
		newInterval := 3600 // 60 minutes

		err := repos.Feed.UpdateFeed(context.Background(), testFeed.ID, newTitle, newInterval)
		require.NoError(t, err)

		// verify the update
		updatedFeed, err := repos.Feed.GetFeed(context.Background(), testFeed.ID)
		require.NoError(t, err)
		assert.Equal(t, newTitle, updatedFeed.Title)
		assert.Equal(t, newInterval, updatedFeed.FetchInterval)

		// verify other fields unchanged
		assert.Equal(t, testFeed.URL, updatedFeed.URL)
		assert.Equal(t, testFeed.Description, updatedFeed.Description)
		assert.Equal(t, testFeed.Enabled, updatedFeed.Enabled)
	})

	t.Run("update non-existent feed", func(t *testing.T) {
		err := repos.Feed.UpdateFeed(context.Background(), 99999, "New Title", 7200)
		assert.NoError(t, err) // sQLite doesn't return error for no rows affected
	})
}

func TestFeedRepository_GetActiveFeedNames(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feeds
	feed1 := &domain.Feed{
		URL:           "https://example.com/feed1.xml",
		Title:         "High Quality Feed",
		Description:   "Feed with high quality articles",
		FetchInterval: 3600,
		Enabled:       true,
	}

	feed2 := &domain.Feed{
		URL:           "https://example.com/feed2.xml",
		Title:         "Mixed Quality Feed",
		Description:   "Feed with mixed quality articles",
		FetchInterval: 3600,
		Enabled:       true,
	}

	feed3 := &domain.Feed{
		URL:           "https://noname.com/feed3.xml",
		Title:         "", // no title, should use URL
		Description:   "Feed without title",
		FetchInterval: 3600,
		Enabled:       true,
	}

	feeds := []*domain.Feed{feed1, feed2, feed3}
	for _, feed := range feeds {
		err := repos.Feed.CreateFeed(context.Background(), feed)
		require.NoError(t, err)
	}

	// create items with different scores
	items := []domain.Item{
		{
			FeedID:      feed1.ID,
			GUID:        "high-quality-1",
			Title:       "High Quality Article 1",
			Link:        "https://example.com/hq1",
			Description: "High quality description",
			Published:   time.Now(),
		},
		{
			FeedID:      feed1.ID,
			GUID:        "high-quality-2",
			Title:       "High Quality Article 2",
			Link:        "https://example.com/hq2",
			Description: "High quality description",
			Published:   time.Now(),
		},
		{
			FeedID:      feed2.ID,
			GUID:        "low-quality-1",
			Title:       "Low Quality Article",
			Link:        "https://example.com/lq1",
			Description: "Low quality description",
			Published:   time.Now(),
		},
		{
			FeedID:      feed3.ID,
			GUID:        "medium-quality-1",
			Title:       "Medium Quality Article",
			Link:        "https://noname.com/mq1",
			Description: "Medium quality description",
			Published:   time.Now(),
		},
	}

	classifications := []*domain.Classification{
		{
			GUID:        "high-quality-1",
			Score:       9.0,
			Explanation: "Excellent article",
			Topics:      []string{"quality", "excellent"},
		},
		{
			GUID:        "high-quality-2",
			Score:       8.5,
			Explanation: "Very good article",
			Topics:      []string{"quality", "good"},
		},
		{
			GUID:        "low-quality-1",
			Score:       3.0,
			Explanation: "Poor article",
			Topics:      []string{"poor"},
		},
		{
			GUID:        "medium-quality-1",
			Score:       7.0,
			Explanation: "Decent article",
			Topics:      []string{"decent"},
		},
	}

	// create items and classifications
	for i := range items {
		err := repos.Item.CreateItem(context.Background(), &items[i])
		require.NoError(t, err)
		err = repos.Item.UpdateItemClassification(context.Background(), items[i].ID, classifications[i])
		require.NoError(t, err)
	}

	t.Run("get active feeds with high score threshold", func(t *testing.T) {
		feedNames, err := repos.Feed.GetActiveFeedNames(context.Background(), 8.0)
		require.NoError(t, err)

		// should return only feed1 (has articles with scores >= 8.0)
		assert.Len(t, feedNames, 1)
		assert.Contains(t, feedNames, "High Quality Feed")
	})

	t.Run("get active feeds with medium score threshold", func(t *testing.T) {
		feedNames, err := repos.Feed.GetActiveFeedNames(context.Background(), 6.0)
		require.NoError(t, err)

		// should return feed1 and feed3 (feed2 has max score 3.0)
		assert.Len(t, feedNames, 2)
		assert.Contains(t, feedNames, "High Quality Feed")
		assert.Contains(t, feedNames, "noname.comfeed3.xml") // URL with slashes removed for feed3
	})

	t.Run("get active feeds with low score threshold", func(t *testing.T) {
		feedNames, err := repos.Feed.GetActiveFeedNames(context.Background(), 2.0)
		require.NoError(t, err)

		// should return all three feeds
		assert.Len(t, feedNames, 3)
		assert.Contains(t, feedNames, "High Quality Feed")
		assert.Contains(t, feedNames, "Mixed Quality Feed")
		assert.Contains(t, feedNames, "noname.comfeed3.xml")
	})

	t.Run("get active feeds with very high threshold", func(t *testing.T) {
		feedNames, err := repos.Feed.GetActiveFeedNames(context.Background(), 10.0)
		require.NoError(t, err)

		// no feeds should meet this threshold
		assert.Empty(t, feedNames)
	})
}
