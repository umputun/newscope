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

func TestClassificationRepository_GetClassifiedItems_Sorting(t *testing.T) {
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

	// create test feed
	testFeed := &domain.Feed{
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		Description:   "A test feed",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test items with different scores and published dates
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	testItems := []*domain.Item{
		{
			FeedID:      testFeed.ID,
			GUID:        "item-1",
			Title:       "High Score Old Item",
			Link:        "https://example.com/item1",
			Description: "Item with high score but older date",
			Published:   baseTime, // oldest
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "item-2",
			Title:       "Low Score New Item",
			Link:        "https://example.com/item2",
			Description: "Item with low score but newer date",
			Published:   baseTime.Add(2 * time.Hour), // newest
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "item-3",
			Title:       "Medium Score Medium Item",
			Link:        "https://example.com/item3",
			Description: "Item with medium score and medium date",
			Published:   baseTime.Add(1 * time.Hour), // middle
		},
	}

	// create items
	for _, item := range testItems {
		err = repos.Item.CreateItem(context.Background(), item)
		require.NoError(t, err)
	}

	// add classifications with different scores
	classifications := []*domain.Classification{
		{
			GUID:        "item-1",
			Score:       9.0, // highest score
			Explanation: "High relevance",
			Topics:      []string{"important"},
		},
		{
			GUID:        "item-2",
			Score:       5.0, // lowest score
			Explanation: "Low relevance",
			Topics:      []string{"general"},
		},
		{
			GUID:        "item-3",
			Score:       7.0, // medium score
			Explanation: "Medium relevance",
			Topics:      []string{"moderate"},
		},
	}

	for i, classification := range classifications {
		err = repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classification)
		require.NoError(t, err)
	}

	t.Run("sort by score descending", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "score",
			Limit:    10,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 3)

		// should be ordered by score descending: 9.0, 7.0, 5.0
		assert.Equal(t, "item-1", items[0].GUID) // score 9.0
		assert.Equal(t, "item-3", items[1].GUID) // score 7.0
		assert.Equal(t, "item-2", items[2].GUID) // score 5.0

		assert.InDelta(t, 9.0, items[0].Classification.Score, 0.001)
		assert.InDelta(t, 7.0, items[1].Classification.Score, 0.001)
		assert.InDelta(t, 5.0, items[2].Classification.Score, 0.001)
	})

	t.Run("sort by published date descending", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "published",
			Limit:    10,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 3)

		// should be ordered by published date descending: newest first
		assert.Equal(t, "item-2", items[0].GUID) // newest (baseTime + 2h)
		assert.Equal(t, "item-3", items[1].GUID) // middle (baseTime + 1h)
		assert.Equal(t, "item-1", items[2].GUID) // oldest (baseTime)

		// verify the dates are actually in descending order
		assert.True(t, items[0].Published.After(items[1].Published))
		assert.True(t, items[1].Published.After(items[2].Published))
	})

	t.Run("default sort behavior", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "", // empty should default to published
			Limit:    10,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 3)

		// should default to published date descending
		assert.Equal(t, "item-2", items[0].GUID) // newest
		assert.Equal(t, "item-3", items[1].GUID) // middle
		assert.Equal(t, "item-1", items[2].GUID) // oldest
	})

	t.Run("score sort with secondary published sort", func(t *testing.T) {
		// create additional item with same score as item-3 but different published date
		additionalItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        "item-4",
			Title:       "Same Score Newer Item",
			Link:        "https://example.com/item4",
			Description: "Same score as item-3 but newer",
			Published:   baseTime.Add(90 * time.Minute), // newer than item-3
		}
		err = repos.Item.CreateItem(context.Background(), additionalItem)
		require.NoError(t, err)

		// give it the same score as item-3
		classification := &domain.Classification{
			GUID:        "item-4",
			Score:       7.0, // same as item-3
			Explanation: "Same score as item-3",
			Topics:      []string{"moderate"},
		}
		err = repos.Item.UpdateItemClassification(context.Background(), additionalItem.ID, classification)
		require.NoError(t, err)

		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "score",
			Limit:    10,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 4)

		// first should be highest score
		assert.Equal(t, "item-1", items[0].GUID) // score 9.0
		assert.InDelta(t, 9.0, items[0].Classification.Score, 0.001)

		// next two should be score 7.0, but ordered by published date desc (item-4 then item-3)
		assert.InDelta(t, 7.0, items[1].Classification.Score, 0.001)
		assert.InDelta(t, 7.0, items[2].Classification.Score, 0.001)
		// item-4 should come before item-3 because it's newer
		assert.Equal(t, "item-4", items[1].GUID) // newer item with score 7.0
		assert.Equal(t, "item-3", items[2].GUID) // older item with score 7.0

		// last should be lowest score
		assert.Equal(t, "item-2", items[3].GUID) // score 5.0
		assert.InDelta(t, 5.0, items[3].Classification.Score, 0.001)
	})

	t.Run("sort by source+date", func(t *testing.T) {
		// create second feed for testing source-based sorting
		secondFeed := &domain.Feed{
			URL:           "https://feed2.example.com/feed.xml",
			Title:         "Alpha Feed", // alphabetically before "Test Feed"
			Description:   "Another test feed",
			FetchInterval: 3600,
			Enabled:       true,
		}
		err = repos.Feed.CreateFeed(context.Background(), secondFeed)
		require.NoError(t, err)

		// create items in second feed
		secondFeedItems := []*domain.Item{
			{
				FeedID:      secondFeed.ID,
				GUID:        "alpha-item-1",
				Title:       "Alpha Feed Old Item",
				Link:        "https://feed2.example.com/item1",
				Description: "Old item from alpha feed",
				Published:   baseTime.Add(30 * time.Minute), // older than item-2, newer than item-1
			},
			{
				FeedID:      secondFeed.ID,
				GUID:        "alpha-item-2",
				Title:       "Alpha Feed New Item",
				Link:        "https://feed2.example.com/item2",
				Description: "New item from alpha feed",
				Published:   baseTime.Add(3 * time.Hour), // newest overall
			},
		}

		// create items in second feed
		for _, item := range secondFeedItems {
			err = repos.Item.CreateItem(context.Background(), item)
			require.NoError(t, err)
		}

		// add classifications
		secondFeedClassifications := []*domain.Classification{
			{
				GUID:        "alpha-item-1",
				Score:       6.0,
				Explanation: "Alpha feed old item",
				Topics:      []string{"alpha"},
			},
			{
				GUID:        "alpha-item-2",
				Score:       8.0,
				Explanation: "Alpha feed new item",
				Topics:      []string{"alpha"},
			},
		}

		for i, classification := range secondFeedClassifications {
			err = repos.Item.UpdateItemClassification(context.Background(), secondFeedItems[i].ID, classification)
			require.NoError(t, err)
		}

		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "source+date",
			Limit:    10,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 6) // 4 from testFeed + 2 from secondFeed

		// should be grouped by feed title (Alpha Feed comes before Test Feed alphabetically)
		// within each feed, sorted by published date descending
		assert.Equal(t, "Alpha Feed", items[0].FeedName) // alpha-item-2 (newest in Alpha Feed)
		assert.Equal(t, "alpha-item-2", items[0].GUID)
		assert.Equal(t, "Alpha Feed", items[1].FeedName) // alpha-item-1 (older in Alpha Feed)
		assert.Equal(t, "alpha-item-1", items[1].GUID)
		assert.Equal(t, "Test Feed", items[2].FeedName) // item-2 (newest in Test Feed)
		assert.Equal(t, "item-2", items[2].GUID)

		// verify dates are descending within each feed group
		assert.True(t, items[0].Published.After(items[1].Published)) // within Alpha Feed
		assert.True(t, items[2].Published.After(items[3].Published)) // within Test Feed
	})

	t.Run("sort by source+score", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "source+score",
			Limit:    10,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 6) // 4 from testFeed + 2 from secondFeed

		// should be grouped by feed title (Alpha Feed comes before Test Feed alphabetically)
		// within each feed, sorted by score descending, then by published date descending
		assert.Equal(t, "Alpha Feed", items[0].FeedName) // alpha-item-2 (score 8.0)
		assert.Equal(t, "alpha-item-2", items[0].GUID)
		assert.InDelta(t, 8.0, items[0].Classification.Score, 0.001)

		assert.Equal(t, "Alpha Feed", items[1].FeedName) // alpha-item-1 (score 6.0)
		assert.Equal(t, "alpha-item-1", items[1].GUID)
		assert.InDelta(t, 6.0, items[1].Classification.Score, 0.001)

		assert.Equal(t, "Test Feed", items[2].FeedName) // item-1 (score 9.0, highest in Test Feed)
		assert.Equal(t, "item-1", items[2].GUID)
		assert.InDelta(t, 9.0, items[2].Classification.Score, 0.001)

		// verify scores are descending within each feed group
		assert.GreaterOrEqual(t, items[0].Classification.Score, items[1].Classification.Score) // within Alpha Feed
		assert.GreaterOrEqual(t, items[2].Classification.Score, items[3].Classification.Score) // within Test Feed
	})
}
