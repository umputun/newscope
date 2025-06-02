package repository

import (
	"context"
	"fmt"
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
	testItems := []domain.Item{
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
	for i := range testItems {
		err = repos.Item.CreateItem(context.Background(), &testItems[i])
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
		secondFeedItems := []domain.Item{
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
		for i := range secondFeedItems {
			err = repos.Item.CreateItem(context.Background(), &secondFeedItems[i])
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

func TestClassificationRepository_Pagination(t *testing.T) {
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
		Title:         "Pagination Test Feed",
		Description:   "Feed for testing pagination",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create multiple test items for pagination testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	testItems := make([]domain.Item, 15) // create 15 items for pagination testing

	for i := 0; i < 15; i++ {
		testItems[i] = domain.Item{
			FeedID:      testFeed.ID,
			GUID:        fmt.Sprintf("pagination-item-%d", i+1),
			Title:       fmt.Sprintf("Pagination Test Item %d", i+1),
			Link:        fmt.Sprintf("https://example.com/item%d", i+1),
			Description: fmt.Sprintf("Description for test item %d", i+1),
			Published:   baseTime.Add(time.Duration(i) * time.Hour), // incrementing times
		}
	}

	// create items
	for i := range testItems {
		err = repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)
	}

	// add classifications to all items
	for i := range testItems {
		classification := &domain.Classification{
			GUID:        testItems[i].GUID,
			Score:       float64(5 + i%6), // scores from 5-10
			Explanation: fmt.Sprintf("Classification for item %d", i+1),
			Topics:      []string{"pagination", "test"},
		}
		err = repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classification)
		require.NoError(t, err)
	}

	t.Run("get total count", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "published",
			Limit:    10,
			Offset:   0,
		}

		count, err := repos.Classification.GetClassifiedItemsCount(context.Background(), filter)
		require.NoError(t, err)
		assert.Equal(t, 15, count)
	})

	t.Run("first page pagination", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "published",
			Limit:    5,
			Offset:   0,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 5)

		// should be newest items (items with highest index since they have later times)
		assert.Equal(t, "pagination-item-15", items[0].GUID) // newest
		assert.Equal(t, "pagination-item-14", items[1].GUID)
		assert.Equal(t, "pagination-item-13", items[2].GUID)
		assert.Equal(t, "pagination-item-12", items[3].GUID)
		assert.Equal(t, "pagination-item-11", items[4].GUID)
	})

	t.Run("second page pagination", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "published",
			Limit:    5,
			Offset:   5,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 5)

		// should be the next 5 items
		assert.Equal(t, "pagination-item-10", items[0].GUID)
		assert.Equal(t, "pagination-item-9", items[1].GUID)
		assert.Equal(t, "pagination-item-8", items[2].GUID)
		assert.Equal(t, "pagination-item-7", items[3].GUID)
		assert.Equal(t, "pagination-item-6", items[4].GUID)
	})

	t.Run("third page pagination", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "published",
			Limit:    5,
			Offset:   10,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, items, 5)

		// should be the last 5 items
		assert.Equal(t, "pagination-item-5", items[0].GUID)
		assert.Equal(t, "pagination-item-4", items[1].GUID)
		assert.Equal(t, "pagination-item-3", items[2].GUID)
		assert.Equal(t, "pagination-item-2", items[3].GUID)
		assert.Equal(t, "pagination-item-1", items[4].GUID) // oldest
	})

	t.Run("pagination with filters", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 7.0, // should filter out some items
			SortBy:   "published",
			Limit:    3,
			Offset:   0,
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)

		// count should be fewer due to score filter
		count, err := repos.Classification.GetClassifiedItemsCount(context.Background(), filter)
		require.NoError(t, err)
		assert.Less(t, count, 15)            // should be less than total
		assert.LessOrEqual(t, len(items), 3) // should respect limit

		// all returned items should have score >= 7.0
		for _, item := range items {
			assert.GreaterOrEqual(t, item.Classification.Score, 7.0)
		}
	})

	t.Run("empty page beyond available items", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore: 0.0,
			SortBy:   "published",
			Limit:    5,
			Offset:   20, // beyond available items
		}

		items, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		assert.Empty(t, items) // should return empty slice
	})
}

func TestClassificationRepository_GetClassifiedItem(t *testing.T) {
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
		Description:   "Feed for testing GetClassifiedItem",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test item
	now := time.Now()
	testItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "test-item-1",
		Title:       "Test Article",
		Link:        "https://example.com/article1",
		Description: "Test description",
		Published:   now,
	}
	err = repos.Item.CreateItem(context.Background(), testItem)
	require.NoError(t, err)

	// add classification to item
	classification := &domain.Classification{
		GUID:        testItem.GUID,
		Score:       8.5,
		Explanation: "Test explanation",
		Topics:      []string{"tech", "news"},
	}
	err = repos.Item.UpdateItemClassification(context.Background(), testItem.ID, classification)
	require.NoError(t, err)

	t.Run("get existing classified item", func(t *testing.T) {
		item, err := repos.Classification.GetClassifiedItem(context.Background(), testItem.ID)
		require.NoError(t, err)
		require.NotNil(t, item)

		assert.Equal(t, testItem.ID, item.ID)
		assert.Equal(t, testFeed.ID, item.FeedID)
		assert.Equal(t, "Test Feed", item.FeedName)
		assert.Equal(t, "test-item-1", item.GUID)
		assert.Equal(t, "Test Article", item.Title)
		assert.Equal(t, "https://example.com/article1", item.Link)
		assert.Equal(t, "Test description", item.Description)

		require.NotNil(t, item.Classification)
		assert.InDelta(t, 8.5, item.Classification.Score, 0.01)
		assert.Equal(t, "Test explanation", item.Classification.Explanation)
		assert.Equal(t, []string{"tech", "news"}, item.Classification.Topics)
	})

	t.Run("get non-existent item", func(t *testing.T) {
		item, err := repos.Classification.GetClassifiedItem(context.Background(), 99999)
		require.Error(t, err)
		assert.Nil(t, item)
	})
}

func TestClassificationRepository_GetTopics(t *testing.T) {
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
		Description:   "Feed for testing GetTopics",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test items with different topics
	testItems := []domain.Item{
		{
			FeedID:      testFeed.ID,
			GUID:        "item-1",
			Title:       "Tech Article",
			Link:        "https://example.com/tech",
			Description: "Tech description",
			Published:   time.Now(),
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "item-2",
			Title:       "Science Article",
			Link:        "https://example.com/science",
			Description: "Science description",
			Published:   time.Now(),
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "item-3",
			Title:       "Mixed Article",
			Link:        "https://example.com/mixed",
			Description: "Mixed description",
			Published:   time.Now(),
		},
	}

	classifications := []*domain.Classification{
		{
			GUID:        "item-1",
			Score:       8.0,
			Explanation: "Tech explanation",
			Topics:      []string{"technology", "programming"},
		},
		{
			GUID:        "item-2",
			Score:       7.5,
			Explanation: "Science explanation",
			Topics:      []string{"science", "research"},
		},
		{
			GUID:        "item-3",
			Score:       9.0,
			Explanation: "Mixed explanation",
			Topics:      []string{"technology", "science"},
		},
	}

	// create items and add classifications
	for i := range testItems {
		err = repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)
		err = repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classifications[i])
		require.NoError(t, err)
	}

	t.Run("get all topics", func(t *testing.T) {
		topics, err := repos.Classification.GetTopics(context.Background())
		require.NoError(t, err)

		// should contain all unique topics
		expectedTopics := []string{"programming", "research", "science", "technology"}
		assert.ElementsMatch(t, expectedTopics, topics)
	})

	t.Run("empty database", func(t *testing.T) {
		// create a new database for this test
		emptyRepos, err := NewRepositories(context.Background(), Config{DSN: ":memory:"})
		require.NoError(t, err)
		defer emptyRepos.Close()

		topics, err := emptyRepos.Classification.GetTopics(context.Background())
		require.NoError(t, err)
		assert.Empty(t, topics)
	})
}

func TestClassificationRepository_GetTopicsFiltered(t *testing.T) {
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
		Description:   "Feed for testing GetTopicsFiltered",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test items with different scores
	testItems := []domain.Item{
		{
			FeedID:      testFeed.ID,
			GUID:        "high-score-item",
			Title:       "High Score Article",
			Link:        "https://example.com/high",
			Description: "High score description",
			Published:   time.Now(),
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "low-score-item",
			Title:       "Low Score Article",
			Link:        "https://example.com/low",
			Description: "Low score description",
			Published:   time.Now(),
		},
	}

	classifications := []*domain.Classification{
		{
			GUID:        "high-score-item",
			Score:       9.0,
			Explanation: "High score explanation",
			Topics:      []string{"important", "featured"},
		},
		{
			GUID:        "low-score-item",
			Score:       3.0,
			Explanation: "Low score explanation",
			Topics:      []string{"basic", "general"},
		},
	}

	// create items and add classifications
	for i := range testItems {
		err = repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)
		err = repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classifications[i])
		require.NoError(t, err)
	}

	t.Run("filter high score topics", func(t *testing.T) {
		topics, err := repos.Classification.GetTopicsFiltered(context.Background(), 8.0)
		require.NoError(t, err)

		// should only contain topics from high-score items
		expectedTopics := []string{"featured", "important"}
		assert.ElementsMatch(t, expectedTopics, topics)
	})

	t.Run("filter low score topics", func(t *testing.T) {
		topics, err := repos.Classification.GetTopicsFiltered(context.Background(), 2.0)
		require.NoError(t, err)

		// should contain all topics since both items meet the threshold
		expectedTopics := []string{"basic", "featured", "general", "important"}
		assert.ElementsMatch(t, expectedTopics, topics)
	})

	t.Run("filter very high score", func(t *testing.T) {
		topics, err := repos.Classification.GetTopicsFiltered(context.Background(), 10.0)
		require.NoError(t, err)

		// no items should meet this threshold
		assert.Empty(t, topics)
	})
}

func TestClassificationRepository_UpdateItemFeedback(t *testing.T) {
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
		Description:   "Feed for testing UpdateItemFeedback",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test item
	testItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "feedback-item",
		Title:       "Feedback Article",
		Link:        "https://example.com/feedback",
		Description: "Feedback description",
		Published:   time.Now(),
	}
	err = repos.Item.CreateItem(context.Background(), testItem)
	require.NoError(t, err)

	t.Run("update feedback like", func(t *testing.T) {
		feedback := &domain.Feedback{
			Type: domain.FeedbackLike,
		}

		err := repos.Classification.UpdateItemFeedback(context.Background(), testItem.ID, feedback)
		require.NoError(t, err)

		// verify feedback was stored by getting the item
		item, err := repos.Classification.GetClassifiedItem(context.Background(), testItem.ID)
		require.NoError(t, err)
		require.NotNil(t, item.UserFeedback)
		assert.Equal(t, domain.FeedbackLike, item.UserFeedback.Type)
	})

	t.Run("update feedback dislike", func(t *testing.T) {
		feedback := &domain.Feedback{
			Type: domain.FeedbackDislike,
		}

		err := repos.Classification.UpdateItemFeedback(context.Background(), testItem.ID, feedback)
		require.NoError(t, err)

		// verify feedback was updated
		item, err := repos.Classification.GetClassifiedItem(context.Background(), testItem.ID)
		require.NoError(t, err)
		require.NotNil(t, item.UserFeedback)
		assert.Equal(t, domain.FeedbackDislike, item.UserFeedback.Type)
	})

	t.Run("update feedback non-existent item", func(t *testing.T) {
		feedback := &domain.Feedback{
			Type: domain.FeedbackLike,
		}

		err := repos.Classification.UpdateItemFeedback(context.Background(), 99999, feedback)
		// sQLite may not error on UPDATE with no matches, depending on implementation
		// this tests the function doesn't panic/crash
		_ = err
	})
}

func TestItemRepository_GetItems(t *testing.T) {
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
		Description:   "Feed for testing GetItems",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test items with different scores
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	testItems := make([]domain.Item, 5)
	for i := 0; i < 5; i++ {
		testItems[i] = domain.Item{
			FeedID:      testFeed.ID,
			GUID:        fmt.Sprintf("item-%d", i+1),
			Title:       fmt.Sprintf("Test Article %d", i+1),
			Link:        fmt.Sprintf("https://example.com/item%d", i+1),
			Description: fmt.Sprintf("Description for item %d", i+1),
			Published:   baseTime.Add(time.Duration(i) * time.Hour),
		}
	}

	// create items and add varying classifications
	for i := range testItems {
		err = repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)

		if i < 3 { // classify first 3 items
			classification := &domain.Classification{
				GUID:        testItems[i].GUID,
				Score:       float64(5 + i*2), // scores: 5, 7, 9
				Explanation: fmt.Sprintf("Classification for item %d", i+1),
				Topics:      []string{"test", "sample"},
			}
			err = repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classification)
			require.NoError(t, err)
		}
	}

	t.Run("get items with score filter", func(t *testing.T) {
		items, err := repos.Item.GetItems(context.Background(), 10, 6.0)
		require.NoError(t, err)

		// should return items with score >= 6.0 (items 2 and 3)
		assert.Len(t, items, 2)

		// items should be ordered by published date desc
		assert.Equal(t, "item-3", items[0].GUID)
		assert.Equal(t, "item-2", items[1].GUID)
	})

	t.Run("get items with high score threshold", func(t *testing.T) {
		items, err := repos.Item.GetItems(context.Background(), 10, 10.0)
		require.NoError(t, err)

		// no items should meet this threshold
		assert.Empty(t, items)
	})

	t.Run("get items with limit", func(t *testing.T) {
		items, err := repos.Item.GetItems(context.Background(), 1, 0.0)
		require.NoError(t, err)

		// should return only 1 item (the most recent)
		assert.Len(t, items, 1)
		// GetItems returns items by published DESC, so item-5 should be first
		assert.Equal(t, "item-5", items[0].GUID)
	})

	t.Run("get items returns all items with score filter", func(t *testing.T) {
		items, err := repos.Item.GetItems(context.Background(), 10, 0.0)
		require.NoError(t, err)

		// GetItems returns all items with relevance_score >= threshold
		// since default score is 0, all items are returned
		assert.Len(t, items, 5)

		// should be ordered by published DESC
		assert.Equal(t, "item-5", items[0].GUID)
		assert.Equal(t, "item-4", items[1].GUID)
		assert.Equal(t, "item-3", items[2].GUID)
		assert.Equal(t, "item-2", items[3].GUID)
		assert.Equal(t, "item-1", items[4].GUID)
	})
}

func TestItemRepository_GetUnclassifiedItems(t *testing.T) {
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
		Description:   "Feed for testing GetUnclassifiedItems",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test items
	classifiedItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "classified-item",
		Title:       "Classified Article",
		Link:        "https://example.com/classified",
		Description: "Classified description",
		Published:   time.Now(),
	}

	unclassifiedItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "unclassified-item",
		Title:       "Unclassified Article",
		Link:        "https://example.com/unclassified",
		Description: "Unclassified description",
		Published:   time.Now(),
	}

	// create items
	err = repos.Item.CreateItem(context.Background(), classifiedItem)
	require.NoError(t, err)
	err = repos.Item.CreateItem(context.Background(), unclassifiedItem)
	require.NoError(t, err)

	// add extraction to both items (GetUnclassifiedItems requires extracted_content != '')
	extraction := &domain.ExtractedContent{
		PlainText: "Extracted content for testing",
		RichHTML:  "<p>Extracted content for testing</p>",
		Error:     "",
	}
	err = repos.Item.UpdateItemExtraction(context.Background(), classifiedItem.ID, extraction)
	require.NoError(t, err)
	err = repos.Item.UpdateItemExtraction(context.Background(), unclassifiedItem.ID, extraction)
	require.NoError(t, err)

	// classify only one item
	classification := &domain.Classification{
		GUID:        classifiedItem.GUID,
		Score:       8.0,
		Explanation: "Test classification",
		Topics:      []string{"test"},
	}
	err = repos.Item.UpdateItemClassification(context.Background(), classifiedItem.ID, classification)
	require.NoError(t, err)

	t.Run("get unclassified items", func(t *testing.T) {
		items, err := repos.Item.GetUnclassifiedItems(context.Background(), 10)
		require.NoError(t, err)

		// should return only the unclassified item
		assert.Len(t, items, 1)
		assert.Equal(t, "unclassified-item", items[0].GUID)
	})

	t.Run("get unclassified items with limit", func(t *testing.T) {
		// add another unclassified item
		anotherItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        "another-unclassified",
			Title:       "Another Unclassified",
			Link:        "https://example.com/another",
			Description: "Another description",
			Published:   time.Now(),
		}
		err = repos.Item.CreateItem(context.Background(), anotherItem)
		require.NoError(t, err)

		items, err := repos.Item.GetUnclassifiedItems(context.Background(), 1)
		require.NoError(t, err)

		// should return only 1 item due to limit
		assert.Len(t, items, 1)
	})
}

func TestItemRepository_GetItemsNeedingExtraction(t *testing.T) {
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
		Description:   "Feed for testing GetItemsNeedingExtraction",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create items - one needs extraction, one already extracted
	needsExtractionItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "needs-extraction",
		Title:       "Needs Extraction",
		Link:        "https://example.com/needs",
		Description: "Needs extraction description",
		Published:   time.Now(),
	}

	extractedItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "already-extracted",
		Title:       "Already Extracted",
		Link:        "https://example.com/extracted",
		Description: "Already extracted description",
		Published:   time.Now(),
	}

	// create items
	err = repos.Item.CreateItem(context.Background(), needsExtractionItem)
	require.NoError(t, err)
	err = repos.Item.CreateItem(context.Background(), extractedItem)
	require.NoError(t, err)

	// mark one item as extracted
	extraction := &domain.ExtractedContent{
		PlainText: "Extracted text",
		RichHTML:  "<p>Extracted HTML</p>",
		Error:     "",
	}
	err = repos.Item.UpdateItemExtraction(context.Background(), extractedItem.ID, extraction)
	require.NoError(t, err)

	t.Run("get items needing extraction", func(t *testing.T) {
		items, err := repos.Item.GetItemsNeedingExtraction(context.Background(), 10)
		require.NoError(t, err)

		// should return only the item that needs extraction
		assert.Len(t, items, 1)
		assert.Equal(t, "needs-extraction", items[0].GUID)
	})

	t.Run("get items needing extraction with limit", func(t *testing.T) {
		// add another item needing extraction
		anotherItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        "another-needs-extraction",
			Title:       "Another Needs Extraction",
			Link:        "https://example.com/another-needs",
			Description: "Another needs extraction",
			Published:   time.Now(),
		}
		err = repos.Item.CreateItem(context.Background(), anotherItem)
		require.NoError(t, err)

		items, err := repos.Item.GetItemsNeedingExtraction(context.Background(), 1)
		require.NoError(t, err)

		// should return only 1 item due to limit
		assert.Len(t, items, 1)
	})
}

func TestItemRepository_UpdateItemExtraction(t *testing.T) {
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

	// create test feed and item
	testFeed := &domain.Feed{
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		Description:   "Feed for testing UpdateItemExtraction",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	testItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "extraction-item",
		Title:       "Extraction Article",
		Link:        "https://example.com/extraction",
		Description: "Extraction description",
		Published:   time.Now(),
	}
	err = repos.Item.CreateItem(context.Background(), testItem)
	require.NoError(t, err)

	t.Run("update extraction success", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "This is the extracted plain text content.",
			RichHTML:  "<p>This is the <strong>extracted</strong> rich HTML content.</p>",
			Error:     "",
		}

		err := repos.Item.UpdateItemExtraction(context.Background(), testItem.ID, extraction)
		require.NoError(t, err)

		// this test verifies the update doesn't error - extraction verification
		// would require checking via GetClassifiedItem or database query
	})

	t.Run("update extraction error", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "",
			RichHTML:  "",
			Error:     "Failed to extract content from URL",
		}

		err := repos.Item.UpdateItemExtraction(context.Background(), testItem.ID, extraction)
		require.NoError(t, err)
	})

	t.Run("update extraction non-existent item", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "Some text",
			RichHTML:  "<p>Some HTML</p>",
			Error:     "",
		}

		err := repos.Item.UpdateItemExtraction(context.Background(), 99999, extraction)
		assert.NoError(t, err) // sQLite doesn't error on UPDATE with no matches
	})
}

func TestFeedRepository_GetFeedsToFetch(t *testing.T) {
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
		err = repos.Feed.CreateFeed(context.Background(), feed)
		require.NoError(t, err)
	}

	// set fetch times after creation using UpdateFeedFetched
	err = repos.Feed.UpdateFeedFetched(context.Background(), recentFeed.ID, future)
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
		Description:   "Feed for testing UpdateFeedFetched",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
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
		Description:   "Feed for testing UpdateFeedError",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
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

func TestFeedRepository_GetActiveFeedNames(t *testing.T) {
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
		err = repos.Feed.CreateFeed(context.Background(), feed)
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
		err = repos.Item.CreateItem(context.Background(), &items[i])
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

func TestClassificationRepository_GetRecentFeedback(t *testing.T) {
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
		Description:   "Feed for testing GetRecentFeedback",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test items with different feedback
	now := time.Now()
	testItems := []domain.Item{
		{
			FeedID:      testFeed.ID,
			GUID:        "feedback-item-1",
			Title:       "Liked Article",
			Link:        "https://example.com/liked",
			Description: "Article that user liked",
			Published:   now.Add(-2 * time.Hour),
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "feedback-item-2",
			Title:       "Disliked Article",
			Link:        "https://example.com/disliked",
			Description: "Article that user disliked",
			Published:   now.Add(-1 * time.Hour),
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "feedback-item-3",
			Title:       "Another Liked Article",
			Link:        "https://example.com/liked2",
			Description: "Another article that user liked",
			Published:   now,
		},
	}

	// create items and add extraction content
	for i := range testItems {
		err = repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)

		// add extraction content (required for feedback functions)
		extraction := &domain.ExtractedContent{
			PlainText: "Extracted content for " + testItems[i].Title,
			RichHTML:  "<p>Extracted content for " + testItems[i].Title + "</p>",
			Error:     "",
		}
		err = repos.Item.UpdateItemExtraction(context.Background(), testItems[i].ID, extraction)
		require.NoError(t, err)
	}

	// add feedback to items
	feedback1 := &domain.Feedback{Type: domain.FeedbackLike}
	err = repos.Classification.UpdateItemFeedback(context.Background(), testItems[0].ID, feedback1)
	require.NoError(t, err)

	feedback2 := &domain.Feedback{Type: domain.FeedbackDislike}
	err = repos.Classification.UpdateItemFeedback(context.Background(), testItems[1].ID, feedback2)
	require.NoError(t, err)

	feedback3 := &domain.Feedback{Type: domain.FeedbackLike}
	err = repos.Classification.UpdateItemFeedback(context.Background(), testItems[2].ID, feedback3)
	require.NoError(t, err)

	t.Run("get all recent feedback", func(t *testing.T) {
		examples, err := repos.Classification.GetRecentFeedback(context.Background(), "", 10)
		require.NoError(t, err)

		// should return all feedback items (likes and dislikes)
		assert.Len(t, examples, 3)

		// verify feedback types are included
		feedbackTypes := make(map[domain.FeedbackType]int)
		for _, example := range examples {
			feedbackTypes[example.Feedback]++
		}
		assert.Equal(t, 2, feedbackTypes[domain.FeedbackLike])
		assert.Equal(t, 1, feedbackTypes[domain.FeedbackDislike])
	})

	t.Run("get recent likes only", func(t *testing.T) {
		examples, err := repos.Classification.GetRecentFeedback(context.Background(), "like", 10)
		require.NoError(t, err)

		// should return only liked items
		assert.Len(t, examples, 2)
		for _, example := range examples {
			assert.Equal(t, domain.FeedbackLike, example.Feedback)
		}
	})

	t.Run("get recent dislikes only", func(t *testing.T) {
		examples, err := repos.Classification.GetRecentFeedback(context.Background(), "dislike", 10)
		require.NoError(t, err)

		// should return only disliked items
		assert.Len(t, examples, 1)
		assert.Equal(t, domain.FeedbackDislike, examples[0].Feedback)
		assert.Equal(t, "Disliked Article", examples[0].Title)
	})

	t.Run("get recent feedback with limit", func(t *testing.T) {
		examples, err := repos.Classification.GetRecentFeedback(context.Background(), "", 2)
		require.NoError(t, err)

		// should respect limit
		assert.Len(t, examples, 2)
	})

	t.Run("get recent feedback with no feedback", func(t *testing.T) {
		// create new database for this test
		emptyRepos, err := NewRepositories(context.Background(), Config{DSN: ":memory:"})
		require.NoError(t, err)
		defer emptyRepos.Close()

		examples, err := emptyRepos.Classification.GetRecentFeedback(context.Background(), "", 10)
		require.NoError(t, err)
		assert.Empty(t, examples)
	})
}

func TestClassificationRepository_GetFeedbackCount(t *testing.T) {
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

	t.Run("count feedback on empty database", func(t *testing.T) {
		count, err := repos.Classification.GetFeedbackCount(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	// create test feed and items with feedback
	testFeed := &domain.Feed{
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		Description:   "Feed for testing GetFeedbackCount",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create items with feedback
	for i := 0; i < 5; i++ {
		item := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        fmt.Sprintf("feedback-item-%d", i+1),
			Title:       fmt.Sprintf("Article %d", i+1),
			Link:        fmt.Sprintf("https://example.com/article%d", i+1),
			Description: fmt.Sprintf("Description for article %d", i+1),
			Published:   time.Now(),
		}
		err = repos.Item.CreateItem(context.Background(), item)
		require.NoError(t, err)

		// add feedback to first 3 items only
		if i < 3 {
			feedbackType := domain.FeedbackLike
			if i == 1 {
				feedbackType = domain.FeedbackDislike
			}
			feedback := &domain.Feedback{Type: feedbackType}
			err = repos.Classification.UpdateItemFeedback(context.Background(), item.ID, feedback)
			require.NoError(t, err)
		}
	}

	t.Run("count feedback with items", func(t *testing.T) {
		count, err := repos.Classification.GetFeedbackCount(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(3), count) // only 3 items have feedback
	})
}

func TestClassificationRepository_GetFeedbackSince(t *testing.T) {
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
		Description:   "Feed for testing GetFeedbackSince",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create items with feedback
	for i := 0; i < 8; i++ {
		item := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        fmt.Sprintf("feedback-item-%d", i+1),
			Title:       fmt.Sprintf("Article %d", i+1),
			Link:        fmt.Sprintf("https://example.com/article%d", i+1),
			Description: fmt.Sprintf("Description for article %d", i+1),
			Published:   time.Now(),
		}
		err = repos.Item.CreateItem(context.Background(), item)
		require.NoError(t, err)

		// add extraction content (required for feedback functions)
		extraction := &domain.ExtractedContent{
			PlainText: fmt.Sprintf("Extracted content for article %d", i+1),
			RichHTML:  fmt.Sprintf("<p>Extracted content for article %d</p>", i+1),
			Error:     "",
		}
		err = repos.Item.UpdateItemExtraction(context.Background(), item.ID, extraction)
		require.NoError(t, err)

		// add feedback (alternating likes and dislikes)
		feedbackType := domain.FeedbackLike
		if i%2 == 1 {
			feedbackType = domain.FeedbackDislike
		}
		feedback := &domain.Feedback{Type: feedbackType}
		err = repos.Classification.UpdateItemFeedback(context.Background(), item.ID, feedback)
		require.NoError(t, err)
	}

	t.Run("get feedback with offset 0", func(t *testing.T) {
		examples, err := repos.Classification.GetFeedbackSince(context.Background(), 0, 5)
		require.NoError(t, err)

		// should return first 5 items
		assert.Len(t, examples, 5)
	})

	t.Run("get feedback with offset 3", func(t *testing.T) {
		examples, err := repos.Classification.GetFeedbackSince(context.Background(), 3, 5)
		require.NoError(t, err)

		// should return next 5 items starting from offset 3
		assert.Len(t, examples, 5)
	})

	t.Run("get feedback with high offset", func(t *testing.T) {
		examples, err := repos.Classification.GetFeedbackSince(context.Background(), 10, 5)
		require.NoError(t, err)

		// should return empty since offset is beyond available items
		assert.Empty(t, examples)
	})

	t.Run("get feedback with limit larger than available", func(t *testing.T) {
		examples, err := repos.Classification.GetFeedbackSince(context.Background(), 0, 20)
		require.NoError(t, err)

		// should return all 8 items
		assert.Len(t, examples, 8)
	})
}

func TestItemRepository_UpdateItemProcessed(t *testing.T) {
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

	// create test feed and item
	testFeed := &domain.Feed{
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		Description:   "Feed for testing UpdateItemProcessed",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	testItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "processed-item",
		Title:       "Test Article",
		Link:        "https://example.com/article",
		Description: "Test description",
		Published:   time.Now(),
	}
	err = repos.Item.CreateItem(context.Background(), testItem)
	require.NoError(t, err)

	t.Run("update item with extraction and classification", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "This is the extracted plain text content.",
			RichHTML:  "<p>This is the <strong>extracted</strong> rich HTML content.</p>",
			Error:     "",
		}

		classification := &domain.Classification{
			GUID:        testItem.GUID,
			Score:       8.5,
			Explanation: "High quality technical article",
			Topics:      []string{"technology", "programming"},
			Summary:     "Updated summary from processing",
		}

		err := repos.Item.UpdateItemProcessed(context.Background(), testItem.ID, extraction, classification)
		require.NoError(t, err)

		// verify the update by getting the item and checking fields were set
		// UpdateItemProcessed updates multiple fields atomically
	})

	t.Run("update item without summary", func(t *testing.T) {
		// create another item for this test
		anotherItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        "processed-item-2",
			Title:       "Another Test Article",
			Link:        "https://example.com/article2",
			Description: "Another test description",
			Published:   time.Now(),
		}
		err = repos.Item.CreateItem(context.Background(), anotherItem)
		require.NoError(t, err)

		extraction := &domain.ExtractedContent{
			PlainText: "Plain text without summary update.",
			RichHTML:  "<p>HTML without summary update.</p>",
			Error:     "",
		}

		classification := &domain.Classification{
			GUID:        anotherItem.GUID,
			Score:       7.0,
			Explanation: "Decent article",
			Topics:      []string{"general"},
			Summary:     "", // empty summary - shouldn't update description
		}

		err := repos.Item.UpdateItemProcessed(context.Background(), anotherItem.ID, extraction, classification)
		require.NoError(t, err)
	})

	t.Run("update non-existent item", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "Some text",
			RichHTML:  "<p>Some HTML</p>",
			Error:     "",
		}

		classification := &domain.Classification{
			GUID:        "non-existent",
			Score:       5.0,
			Explanation: "Test",
			Topics:      []string{"test"},
		}

		err := repos.Item.UpdateItemProcessed(context.Background(), 99999, extraction, classification)
		require.NoError(t, err) // function should not error on non-existent items
	})
}

func TestItemRepository_ItemExistsByTitleOrURL(t *testing.T) {
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
		Description:   "Feed for testing ItemExistsByTitleOrURL",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err = repos.Feed.CreateFeed(context.Background(), testFeed)
	require.NoError(t, err)

	// create test items
	existingItems := []domain.Item{
		{
			FeedID:      testFeed.ID,
			GUID:        "existing-item-1",
			Title:       "Unique Test Article",
			Link:        "https://example.com/unique-article",
			Description: "Description for unique article",
			Published:   time.Now(),
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "existing-item-2",
			Title:       "Another Article",
			Link:        "https://different.com/another-article",
			Description: "Description for another article",
			Published:   time.Now(),
		},
	}

	for i := range existingItems {
		err = repos.Item.CreateItem(context.Background(), &existingItems[i])
		require.NoError(t, err)
	}

	t.Run("item exists by title", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Unique Test Article", "https://some-other-url.com")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("item exists by URL", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Some Other Title", "https://example.com/unique-article")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("item exists by both title and URL", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Unique Test Article", "https://example.com/unique-article")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("item does not exist", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Non-existent Title", "https://non-existent.com/url")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("empty title and URL", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "", "")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("check against second item", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Another Article", "https://random.com/url")
		require.NoError(t, err)
		assert.True(t, exists) // should find by title match
	})
}

func TestCriticalError(t *testing.T) {
	originalErr := fmt.Errorf("test error message")
	critErr := &criticalError{err: originalErr}

	assert.Equal(t, "test error message", critErr.Error())
	assert.Equal(t, originalErr.Error(), critErr.Error())
}

func TestIsLockError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		assert.False(t, isLockError(nil))
	})

	t.Run("sqlite busy error", func(t *testing.T) {
		err := fmt.Errorf("SQLITE_BUSY: database is busy")
		assert.True(t, isLockError(err))
	})

	t.Run("database locked error", func(t *testing.T) {
		err := fmt.Errorf("database is locked")
		assert.True(t, isLockError(err))
	})

	t.Run("table locked error", func(t *testing.T) {
		err := fmt.Errorf("database table is locked")
		assert.True(t, isLockError(err))
	})

	t.Run("non-lock error", func(t *testing.T) {
		err := fmt.Errorf("syntax error")
		assert.False(t, isLockError(err))
	})

	t.Run("empty error message", func(t *testing.T) {
		err := fmt.Errorf("")
		assert.False(t, isLockError(err))
	})
}

func TestClassificationSQL_Value(t *testing.T) {
	t.Run("nil classification", func(t *testing.T) {
		var c classificationSQL
		value, err := c.Value()
		require.NoError(t, err)
		assert.Equal(t, "[]", value)
	})

	t.Run("empty classification", func(t *testing.T) {
		c := classificationSQL{}
		value, err := c.Value()
		require.NoError(t, err)
		expectedJSON := "[]"
		assert.JSONEq(t, expectedJSON, string(value.([]byte)))
	})

	t.Run("non-empty classification", func(t *testing.T) {
		c := classificationSQL{"topic1", "topic2", "topic3"}
		value, err := c.Value()
		require.NoError(t, err)
		expectedJSON := `["topic1","topic2","topic3"]`
		assert.JSONEq(t, expectedJSON, string(value.([]byte)))
	})

	t.Run("single topic classification", func(t *testing.T) {
		c := classificationSQL{"single-topic"}
		value, err := c.Value()
		require.NoError(t, err)
		expectedJSON := `["single-topic"]`
		assert.JSONEq(t, expectedJSON, string(value.([]byte)))
	})
}
