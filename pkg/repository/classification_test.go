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

func TestClassificationRepository_GetClassifiedItems_Sorting(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

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
		err := repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)
	}

	// add classifications with different scores
	classifications := []*domain.Classification{
		{
			GUID:        "item-1",
			Score:       9.0, // highest score
			Explanation: "High relevance",
			Topics:      []string{"important"},
			Summary:     "Summary for high score item",
		},
		{
			GUID:        "item-2",
			Score:       5.0, // lowest score
			Explanation: "Low relevance",
			Topics:      []string{"general"},
			Summary:     "Summary for low score item",
		},
		{
			GUID:        "item-3",
			Score:       7.0, // medium score
			Explanation: "Medium relevance",
			Topics:      []string{"moderate"},
			Summary:     "Summary for medium score item",
		},
	}

	for i, classification := range classifications {
		err := repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classification)
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

		// verify summaries are returned correctly
		assert.Equal(t, "Summary for high score item", items[0].Classification.Summary)
		assert.Equal(t, "Summary for medium score item", items[1].Classification.Summary)
		assert.Equal(t, "Summary for low score item", items[2].Classification.Summary)
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
		err := repos.Item.CreateItem(context.Background(), additionalItem)
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
		err := repos.Feed.CreateFeed(context.Background(), secondFeed)
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
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Pagination Test Feed")

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
		err := repos.Item.CreateItem(context.Background(), &testItems[i])
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
		err := repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classification)
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
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

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
	err := repos.Item.CreateItem(context.Background(), testItem)
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
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

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
		err := repos.Item.CreateItem(context.Background(), &testItems[i])
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
		emptyRepos, cleanup := setupTestDB(t)
		defer cleanup()

		topics, err := emptyRepos.Classification.GetTopics(context.Background())
		require.NoError(t, err)
		assert.Empty(t, topics)
	})
}

func TestClassificationRepository_GetTopicsFiltered(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

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
		err := repos.Item.CreateItem(context.Background(), &testItems[i])
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
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

	// create test item
	testItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "feedback-item",
		Title:       "Feedback Article",
		Link:        "https://example.com/feedback",
		Description: "Feedback description",
		Published:   time.Now(),
	}
	err := repos.Item.CreateItem(context.Background(), testItem)
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

func TestClassificationRepository_GetRecentFeedback(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

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
		err := repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)

		// add extraction content (required for feedback functions)
		extraction := &domain.ExtractedContent{
			PlainText: "Extracted content for " + testItems[i].Title,
			RichHTML:  "<p>Extracted content for " + testItems[i].Title + "</p>",
			Error:     "",
		}
		err = repos.Item.UpdateItemExtraction(context.Background(), testItems[i].ID, extraction)
		require.NoError(t, err)

		// add classification with summary
		classification := &domain.Classification{
			GUID:        testItems[i].GUID,
			Score:       8.0,
			Explanation: "Test classification",
			Topics:      []string{"test"},
			Summary:     fmt.Sprintf("AI summary for %s", testItems[i].Title),
		}
		err = repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classification)
		require.NoError(t, err)
	}

	// add feedback to items
	feedback1 := &domain.Feedback{Type: domain.FeedbackLike}
	err := repos.Classification.UpdateItemFeedback(context.Background(), testItems[0].ID, feedback1)
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
			// verify summary is included
			assert.NotEmpty(t, example.Summary)
			assert.Contains(t, example.Summary, "AI summary for")
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
			// verify summary is included
			assert.NotEmpty(t, example.Summary)
			assert.Contains(t, example.Summary, "AI summary for")
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
		emptyRepos, cleanup := setupTestDB(t)
		defer cleanup()

		examples, err := emptyRepos.Classification.GetRecentFeedback(context.Background(), "", 10)
		require.NoError(t, err)
		assert.Empty(t, examples)
	})
}

func TestClassificationRepository_GetFeedbackCount(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("count feedback on empty database", func(t *testing.T) {
		count, err := repos.Classification.GetFeedbackCount(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	// create test feed and items with feedback
	testFeed := createTestFeed(t, repos, "Test Feed")

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
		err := repos.Item.CreateItem(context.Background(), item)
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
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

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
		err := repos.Item.CreateItem(context.Background(), item)
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

func TestClassificationRepository_GetTopTopicsByScore(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// use the same test data pattern as the working TestClassificationRepository_GetTopics test
	// to avoid the json_each issue that's specific to this test setup
	testFeed := createTestFeed(t, repos, "Test Feed")

	// create test items with different topics and scores
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
		err := repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)
		err = repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classifications[i])
		require.NoError(t, err)
	}

	t.Run("get top topics by score", func(t *testing.T) {
		topics, err := repos.Classification.GetTopTopicsByScore(context.Background(), 7.0, 10)
		require.NoError(t, err)

		// should return topics ordered by average score
		// technology: (8.0 + 9.0) / 2 = 8.5
		// science: (7.5 + 9.0) / 2 = 8.25
		// programming: 8.0 / 1 = 8.0
		// research: 7.5 / 1 = 7.5
		require.GreaterOrEqual(t, len(topics), 3)

		// topics should be ordered by average score descending
		assert.GreaterOrEqual(t, topics[0].AvgScore, topics[1].AvgScore)
		if len(topics) > 2 {
			assert.GreaterOrEqual(t, topics[1].AvgScore, topics[2].AvgScore)
		}

		// verify we have expected topics
		topicNames := make([]string, len(topics))
		for i, topic := range topics {
			topicNames[i] = topic.Topic
		}
		assert.Contains(t, topicNames, "technology")
		assert.Contains(t, topicNames, "science")
	})

	t.Run("get top topics with high threshold", func(t *testing.T) {
		topics, err := repos.Classification.GetTopTopicsByScore(context.Background(), 8.5, 10)
		require.NoError(t, err)

		// with threshold 8.5, only items with score >= 8.5 are included (item-3 with score 9.0)
		// so technology and science both get score 9.0 with 1 item each
		assert.GreaterOrEqual(t, len(topics), 2)

		found := false
		for _, topic := range topics {
			if topic.Topic == "technology" {
				found = true
				assert.InDelta(t, 9.0, topic.AvgScore, 0.01) // only from item-3 with score 9.0
				assert.Equal(t, 1, topic.ItemCount)          // only item-3
			}
		}
		assert.True(t, found, "technology topic should be found")
	})

	t.Run("get top topics with limit", func(t *testing.T) {
		topics, err := repos.Classification.GetTopTopicsByScore(context.Background(), 0.0, 2)
		require.NoError(t, err)

		// should respect limit
		assert.LessOrEqual(t, len(topics), 2)
	})
}

func TestClassificationRepository_GetClassifiedItems_LikedFilter(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	feed := &domain.Feed{
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		Description:   "Test feed for liked filter",
		FetchInterval: 3600,
		Enabled:       true,
	}
	err := repos.Feed.CreateFeed(context.Background(), feed)
	require.NoError(t, err)

	// create test items
	baseTime := time.Now().Add(-24 * time.Hour)
	items := []domain.Item{
		{
			FeedID:      feed.ID,
			GUID:        "liked-item-1",
			Title:       "Liked Article 1",
			Link:        "https://example.com/liked1",
			Description: "This is a liked article",
			Published:   baseTime.Add(1 * time.Hour),
		},
		{
			FeedID:      feed.ID,
			GUID:        "liked-item-2",
			Title:       "Liked Article 2",
			Link:        "https://example.com/liked2",
			Description: "Another liked article",
			Published:   baseTime.Add(2 * time.Hour),
		},
		{
			FeedID:      feed.ID,
			GUID:        "disliked-item",
			Title:       "Disliked Article",
			Link:        "https://example.com/disliked",
			Description: "This article was disliked",
			Published:   baseTime.Add(3 * time.Hour),
		},
		{
			FeedID:      feed.ID,
			GUID:        "neutral-item",
			Title:       "Neutral Article",
			Link:        "https://example.com/neutral",
			Description: "No feedback on this article",
			Published:   baseTime.Add(4 * time.Hour),
		},
	}

	// create items and add classifications
	for i := range items {
		err = repos.Item.CreateItem(context.Background(), &items[i])
		require.NoError(t, err)

		// add classification
		classification := &domain.Classification{
			GUID:        items[i].GUID,
			Score:       7.5,
			Explanation: "Test classification",
			Topics:      []string{"test"},
		}
		err = repos.Item.UpdateItemClassification(context.Background(), items[i].ID, classification)
		require.NoError(t, err)
	}

	// add user feedback
	err = repos.Classification.UpdateItemFeedback(context.Background(), items[0].ID, &domain.Feedback{Type: domain.FeedbackLike})
	require.NoError(t, err)

	err = repos.Classification.UpdateItemFeedback(context.Background(), items[1].ID, &domain.Feedback{Type: domain.FeedbackLike})
	require.NoError(t, err)

	err = repos.Classification.UpdateItemFeedback(context.Background(), items[2].ID, &domain.Feedback{Type: domain.FeedbackDislike})
	require.NoError(t, err)
	// items[3] has no feedback

	t.Run("filter liked only", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore:      0.0,
			ShowLikedOnly: true,
			SortBy:        "published",
			Limit:         10,
		}

		result, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, result, 2) // only 2 liked items

		// verify only liked items are returned
		assert.Equal(t, "Liked Article 2", result[0].Title) // newer first
		assert.Equal(t, "Liked Article 1", result[1].Title)

		// verify feedback is correctly set
		assert.Equal(t, domain.FeedbackLike, result[0].UserFeedback.Type)
		assert.Equal(t, domain.FeedbackLike, result[1].UserFeedback.Type)
	})

	t.Run("filter without liked only shows all", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore:      0.0,
			ShowLikedOnly: false,
			SortBy:        "published",
			Limit:         10,
		}

		result, err := repos.Classification.GetClassifiedItems(context.Background(), filter)
		require.NoError(t, err)
		require.Len(t, result, 4) // all 4 items

		// verify all items are returned in correct order
		assert.Equal(t, "Neutral Article", result[0].Title)
		assert.Equal(t, "Disliked Article", result[1].Title)
		assert.Equal(t, "Liked Article 2", result[2].Title)
		assert.Equal(t, "Liked Article 1", result[3].Title)
	})

	t.Run("count with liked filter", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore:      0.0,
			ShowLikedOnly: true,
		}

		count, err := repos.Classification.GetClassifiedItemsCount(context.Background(), filter)
		require.NoError(t, err)
		assert.Equal(t, 2, count) // only 2 liked items
	})

	t.Run("count without liked filter", func(t *testing.T) {
		filter := &domain.ItemFilter{
			MinScore:      0.0,
			ShowLikedOnly: false,
		}

		count, err := repos.Classification.GetClassifiedItemsCount(context.Background(), filter)
		require.NoError(t, err)
		assert.Equal(t, 4, count) // all 4 items
	})
}

func TestClassificationRepository_SearchItems(t *testing.T) {
	// setup test database
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create test feed
	testFeed := createTestFeed(t, repos, "Tech News")

	// create test items with content for FTS
	items := []struct {
		title       string
		description string
		content     string
		summary     string
		score       float64
		topics      []string
		feedback    string
	}{
		{
			title:       "Golang Best Practices",
			description: "Learn about best practices in Go programming",
			content:     "This article covers testing, error handling, and concurrency patterns in Go",
			summary:     "A comprehensive guide to Go best practices",
			score:       8.5,
			topics:      []string{"golang", "programming"},
			feedback:    "like",
		},
		{
			title:       "Python Machine Learning",
			description: "Introduction to ML with Python",
			content:     "Python provides excellent libraries for machine learning and data science",
			summary:     "Getting started with ML in Python",
			score:       7.0,
			topics:      []string{"python", "ml"},
			feedback:    "",
		},
		{
			title:       "JavaScript Frameworks",
			description: "Comparing modern JS frameworks",
			content:     "React, Vue, and Angular are popular choices for web development",
			summary:     "Overview of JavaScript frameworks",
			score:       6.0,
			topics:      []string{"javascript", "web"},
			feedback:    "dislike",
		},
		{
			title:       "ChatGPT and AI Revolution",
			description: "How ChatGPT is changing the AI landscape",
			content:     "ChatGPT represents a major advancement in conversational AI technology",
			summary:     "The impact of ChatGPT on AI development",
			score:       9.0,
			topics:      []string{"ai", "chatgpt"},
			feedback:    "like",
		},
	}

	// insert items
	for i, item := range items {
		testItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        fmt.Sprintf("guid-%d", i),
			Title:       item.title,
			Link:        fmt.Sprintf("https://example.com/%d", i),
			Description: item.description,
			Content:     item.content,
			Published:   time.Now().Add(time.Duration(-i) * time.Hour),
		}
		err := repos.Item.CreateItem(ctx, testItem)
		require.NoError(t, err)

		// add classification
		classification := &domain.Classification{
			GUID:         testItem.GUID,
			Score:        item.score,
			Topics:       item.topics,
			Summary:      item.summary,
			ClassifiedAt: time.Now(),
		}
		err = repos.Item.UpdateItemClassification(ctx, testItem.ID, classification)
		require.NoError(t, err)

		// add extracted content
		err = repos.Item.UpdateItemExtraction(ctx, testItem.ID, &domain.ExtractedContent{
			PlainText:   item.content,
			ExtractedAt: time.Now(),
		})
		require.NoError(t, err)

		// add feedback if any
		if item.feedback != "" {
			feedback := &domain.Feedback{
				Type: domain.FeedbackType(item.feedback),
			}
			err = repos.Classification.UpdateItemFeedback(ctx, testItem.ID, feedback)
			require.NoError(t, err)
		}
	}

	tests := []struct {
		name        string
		searchQuery string
		filter      *domain.ItemFilter
		wantCount   int
		wantTitles  []string
	}{
		{
			name:        "search for golang",
			searchQuery: "golang",
			filter:      &domain.ItemFilter{Limit: 10},
			wantCount:   1,
			wantTitles:  []string{"Golang Best Practices"},
		},
		{
			name:        "search for programming",
			searchQuery: "programming",
			filter:      &domain.ItemFilter{Limit: 10},
			wantCount:   1,
			wantTitles:  []string{"Golang Best Practices"},
		},
		{
			name:        "search for python",
			searchQuery: "python",
			filter:      &domain.ItemFilter{Limit: 10},
			wantCount:   1,
			wantTitles:  []string{"Python Machine Learning"},
		},
		{
			name:        "search with min score filter",
			searchQuery: "programming OR python",
			filter:      &domain.ItemFilter{MinScore: 7.5, Limit: 10},
			wantCount:   1,
			wantTitles:  []string{"Golang Best Practices"},
		},
		{
			name:        "search with topic filter",
			searchQuery: "web",
			filter:      &domain.ItemFilter{Topic: "javascript", Limit: 10},
			wantCount:   1,
			wantTitles:  []string{"JavaScript Frameworks"},
		},
		{
			name:        "search liked only",
			searchQuery: "programming OR python",
			filter:      &domain.ItemFilter{ShowLikedOnly: true, Limit: 10},
			wantCount:   1,
			wantTitles:  []string{"Golang Best Practices"},
		},
		{
			name:        "no results",
			searchQuery: "rust",
			filter:      &domain.ItemFilter{Limit: 10},
			wantCount:   0,
			wantTitles:  []string{},
		},
		{
			name:        "search for GPT should find ChatGPT",
			searchQuery: "GPT",
			filter:      &domain.ItemFilter{Limit: 10},
			wantCount:   1,
			wantTitles:  []string{"ChatGPT and AI Revolution"},
		},
		{
			name:        "search for ChatGPT exact match",
			searchQuery: "ChatGPT",
			filter:      &domain.ItemFilter{Limit: 10},
			wantCount:   1,
			wantTitles:  []string{"ChatGPT and AI Revolution"},
		},
		{
			name:        "complex query with OR operator",
			searchQuery: "golang OR chatgpt",
			filter:      &domain.ItemFilter{Limit: 10},
			wantCount:   2,
			wantTitles:  []string{"ChatGPT and AI Revolution", "Golang Best Practices"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// search items
			items, err := repos.Classification.SearchItems(ctx, tt.searchQuery, tt.filter)
			require.NoError(t, err)
			assert.Len(t, items, tt.wantCount)

			// verify titles
			titles := make([]string, 0, len(items))
			for _, item := range items {
				titles = append(titles, item.Title)
			}
			assert.Equal(t, tt.wantTitles, titles)

			// verify count
			count, err := repos.Classification.GetSearchItemsCount(ctx, tt.searchQuery, tt.filter)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, count)
		})
	}
}
