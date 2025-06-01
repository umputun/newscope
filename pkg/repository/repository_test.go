package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/domain"
)

func TestRepositories_Integration(t *testing.T) {
	// setup test database
	cfg := Config{
		DSN:             ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 30 * time.Second,
	}

	repos, err := NewRepositories(context.Background(), cfg)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, repos.Close())
	}()

	// test ping
	require.NoError(t, repos.Ping(context.Background()))

	// test feed operations
	t.Run("feed operations", func(t *testing.T) {
		testFeed := &domain.Feed{
			URL:           "https://example.com/feed.xml",
			Title:         "Test Feed",
			Description:   "A test feed",
			FetchInterval: 3600,
			Enabled:       true,
		}

		// create feed
		err := repos.Feed.CreateFeed(context.Background(), testFeed)
		require.NoError(t, err)
		assert.NotZero(t, testFeed.ID)

		// get feed
		retrievedFeed, err := repos.Feed.GetFeed(context.Background(), testFeed.ID)
		require.NoError(t, err)
		assert.Equal(t, testFeed.URL, retrievedFeed.URL)
		assert.Equal(t, testFeed.Title, retrievedFeed.Title)

		// get all feeds
		feeds, err := repos.Feed.GetFeeds(context.Background(), false)
		require.NoError(t, err)
		assert.Len(t, feeds, 1)
		assert.Equal(t, testFeed.ID, feeds[0].ID)

		// update feed status
		err = repos.Feed.UpdateFeedStatus(context.Background(), testFeed.ID, false)
		require.NoError(t, err)

		// verify status update
		updatedFeed, err := repos.Feed.GetFeed(context.Background(), testFeed.ID)
		require.NoError(t, err)
		assert.False(t, updatedFeed.Enabled)

		// test item operations with the feed
		testItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        "test-guid-1",
			Title:       "Test Item",
			Link:        "https://example.com/item1",
			Description: "Test item description",
			Content:     "Test content",
			Author:      "Test Author",
			Published:   time.Now(),
		}

		// create item
		err = repos.Item.CreateItem(context.Background(), testItem)
		require.NoError(t, err)
		assert.NotZero(t, testItem.ID)

		// check item exists
		exists, err := repos.Item.ItemExists(context.Background(), testFeed.ID, testItem.GUID)
		require.NoError(t, err)
		assert.True(t, exists)

		// get item
		retrievedItem, err := repos.Item.GetItem(context.Background(), testItem.ID)
		require.NoError(t, err)
		assert.Equal(t, testItem.Title, retrievedItem.Title)
		assert.Equal(t, testItem.GUID, retrievedItem.GUID)

		// test classification operations
		classification := &domain.Classification{
			GUID:        testItem.GUID,
			Score:       8.5,
			Explanation: "Test classification",
			Topics:      []string{"tech", "news"},
			Summary:     "Test summary",
		}

		// update item with classification
		err = repos.Item.UpdateItemClassification(context.Background(), testItem.ID, classification)
		require.NoError(t, err)

		// test settings
		err = repos.Setting.SetSetting(context.Background(), "test_key", "test_value")
		require.NoError(t, err)

		value, err := repos.Setting.GetSetting(context.Background(), "test_key")
		require.NoError(t, err)
		assert.Equal(t, "test_value", value)

		// delete feed (should cascade to items)
		err = repos.Feed.DeleteFeed(context.Background(), testFeed.ID)
		require.NoError(t, err)

		// verify feed is deleted
		_, err = repos.Feed.GetFeed(context.Background(), testFeed.ID)
		assert.Error(t, err)
	})
}

func TestNewRepositories_InvalidDSN(t *testing.T) {
	cfg := Config{
		DSN: "invalid://database/url",
	}

	_, err := NewRepositories(context.Background(), cfg)
	assert.Error(t, err)
}

func TestRepositories_Close(t *testing.T) {
	cfg := Config{
		DSN:             ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 30 * time.Second,
	}

	repos, err := NewRepositories(context.Background(), cfg)
	require.NoError(t, err)

	// close should not error
	assert.NoError(t, repos.Close())

	// second close should not error
	assert.NoError(t, repos.Close())
}
