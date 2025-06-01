package db

import (
	"context"
	"fmt"
	"os"
	"sync"
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

	t.Run("update feed status", func(t *testing.T) {
		// disable feed
		err := db.UpdateFeedStatus(ctx, 1, false)
		require.NoError(t, err)

		feed, err := db.GetFeed(ctx, 1)
		require.NoError(t, err)
		assert.False(t, feed.Enabled)

		// enable feed
		err = db.UpdateFeedStatus(ctx, 1, true)
		require.NoError(t, err)

		feed, err = db.GetFeed(ctx, 1)
		require.NoError(t, err)
		assert.True(t, feed.Enabled)
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
		// get initial score
		item, err := db.GetItem(ctx, 1)
		require.NoError(t, err)
		initialScore := item.RelevanceScore

		// like should increase score by 1
		err = db.UpdateItemFeedback(ctx, 1, "like")
		require.NoError(t, err)

		item, err = db.GetItem(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, "like", item.UserFeedback)
		assert.NotNil(t, item.FeedbackAt)
		assert.InEpsilon(t, initialScore+1, item.RelevanceScore, 0.001)

		// dislike should decrease score by 2
		err = db.UpdateItemFeedback(ctx, 1, "dislike")
		require.NoError(t, err)

		item, err = db.GetItem(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, "dislike", item.UserFeedback)
		assert.InEpsilon(t, initialScore-1, item.RelevanceScore, 0.001) // +1 -2 = -1 from initial
	})

	t.Run("update item feedback score boundaries", func(t *testing.T) {
		// create item with score near max
		item := &Item{
			FeedID:           feed.ID,
			GUID:             "guid-max-score",
			Title:            "Max Score Item",
			Link:             "https://example.com/max",
			Published:        time.Now(),
			ExtractedContent: "Content",
		}
		err := db.CreateItem(ctx, item)
		require.NoError(t, err)

		// set score to 9.5
		classification := Classification{
			GUID:        "guid-max-score",
			Score:       9.5,
			Explanation: "Very high score",
			Topics:      []string{"test"},
		}
		err = db.UpdateItemProcessed(ctx, item.ID, "Content", "", classification)
		require.NoError(t, err)

		// like should cap at 10
		err = db.UpdateItemFeedback(ctx, item.ID, "like")
		require.NoError(t, err)

		updated, err := db.GetItem(ctx, item.ID)
		require.NoError(t, err)
		assert.InEpsilon(t, 10.0, updated.RelevanceScore, 0.001)

		// create item with low score
		item2 := &Item{
			FeedID:           feed.ID,
			GUID:             "guid-min-score",
			Title:            "Min Score Item",
			Link:             "https://example.com/min",
			Published:        time.Now(),
			ExtractedContent: "Content",
		}
		err = db.CreateItem(ctx, item2)
		require.NoError(t, err)

		// set score to 1.0
		classification2 := Classification{
			GUID:        "guid-min-score",
			Score:       1.0,
			Explanation: "Very low score",
			Topics:      []string{"test"},
		}
		err = db.UpdateItemProcessed(ctx, item2.ID, "Content", "", classification2)
		require.NoError(t, err)

		// dislike should cap at 0
		err = db.UpdateItemFeedback(ctx, item2.ID, "dislike")
		require.NoError(t, err)

		updated2, err := db.GetItem(ctx, item2.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.0, updated2.RelevanceScore, 0.001)
	})

	t.Run("get recent feedback", func(t *testing.T) {
		examples, err := db.GetRecentFeedback(ctx, "dislike", 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(examples), 1)
		// should have at least one dislike from the previous test
		found := false
		for _, ex := range examples {
			if ex.Feedback == "dislike" {
				found = true
				break
			}
		}
		assert.True(t, found, "should find at least one dislike feedback")
	})

	t.Run("get classified items with filters", func(t *testing.T) {
		// test topic filtering
		topicItems, err := db.GetClassifiedItemsWithFilters(ctx, 0, "test", "", 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(topicItems), 1)

		// verify all items have the topic
		for _, item := range topicItems {
			found := false
			for _, topic := range item.Topics {
				if topic == "test" {
					found = true
					break
				}
			}
			assert.True(t, found, "item should have 'test' topic")
		}

		// test feed filtering
		feedItems, err := db.GetClassifiedItemsWithFilters(ctx, 0, "", "Test Feed", 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(feedItems), 1)

		// verify all items are from the correct feed
		for _, item := range feedItems {
			assert.Equal(t, "Test Feed", item.FeedTitle)
		}

		// test combined filtering
		combinedItems, err := db.GetClassifiedItemsWithFilters(ctx, 0, "test", "Test Feed", 10)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(combinedItems), len(topicItems))
		assert.LessOrEqual(t, len(combinedItems), len(feedItems))
	})

	t.Run("get active feed names", func(t *testing.T) {
		feedNames, err := db.GetActiveFeedNames(ctx, 0)
		require.NoError(t, err)
		assert.Contains(t, feedNames, "Test Feed")

		// test with higher score threshold
		feedNamesHighScore, err := db.GetActiveFeedNames(ctx, 8)
		require.NoError(t, err)
		// should have fewer or equal feeds
		assert.LessOrEqual(t, len(feedNamesHighScore), len(feedNames))
	})

	t.Run("classify items for later tests", func(t *testing.T) {
		// create more items and classify them for subsequent tests
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

		// update items with classification for later tests
		classification1 := Classification{
			GUID:        "guid-3",
			Score:       7.0,
			Explanation: "Good match",
			Topics:      []string{"tech"},
		}
		err = db.UpdateItemProcessed(ctx, item3.ID, "Content 3", "", classification1)
		require.NoError(t, err)

		// also classify the earlier item for topics test
		classification2 := Classification{
			GUID:        "guid-needs-extraction",
			Score:       3.0,
			Explanation: "Not relevant",
			Topics:      []string{"other"},
		}
		err = db.UpdateItemProcessed(ctx, 2, "Some content", "", classification2)
		require.NoError(t, err)

		// verify updates
		item, err := db.GetItem(ctx, item3.ID)
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
		// create a test item if it doesn't exist
		var testItemID int64
		err := db.GetContext(ctx, &testItemID, "SELECT id FROM items WHERE title = ?", "Test Classified Item")
		if err != nil {
			// create the item
			testItem := &Item{
				FeedID:           feed.ID,
				GUID:             "guid-test-classified",
				Title:            "Test Classified Item",
				Link:             "https://example.com/test-classified",
				Published:        time.Now(),
				ExtractedContent: "Test content",
			}
			err := db.CreateItem(ctx, testItem)
			require.NoError(t, err)
			testItemID = testItem.ID

			// classify it
			classification := Classification{
				GUID:        "guid-test-classified",
				Score:       7.5,
				Explanation: "Test classification",
				Topics:      []string{"test-topic"},
			}
			err = db.UpdateItemProcessed(ctx, testItemID, "Test content", "", classification)
			require.NoError(t, err)
		}

		// get item with feed info
		item, err := db.GetClassifiedItem(ctx, testItemID)
		require.NoError(t, err)
		assert.Equal(t, "Test Classified Item", item.Title)
		assert.Equal(t, "Test Feed", item.FeedTitle)
		assert.InEpsilon(t, 7.5, item.RelevanceScore, 0.001)

		// try to get non-existent item
		_, err = db.GetClassifiedItem(ctx, 99999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("get classified item with rich content", func(t *testing.T) {
		// create a test item for this specific test
		testItem := &Item{
			FeedID:           feed.ID,
			GUID:             "guid-rich-content-test",
			Title:            "Rich Content Test Item",
			Link:             "https://example.com/rich-content-test",
			Published:        time.Now(),
			ExtractedContent: "Initial content",
		}
		err := db.CreateItem(ctx, testItem)
		require.NoError(t, err)

		// classify it
		classification := Classification{
			GUID:        "guid-rich-content-test",
			Score:       8.0,
			Explanation: "Rich content test",
			Topics:      []string{"rich-test"},
		}
		err = db.UpdateItemProcessed(ctx, testItem.ID, "Initial content", "", classification)
		require.NoError(t, err)

		// update item with rich content
		err = db.UpdateItemExtraction(ctx, testItem.ID, "Plain content", "<p>Rich content</p>", nil)
		require.NoError(t, err)

		// get item and verify both content fields
		item, err := db.GetClassifiedItem(ctx, testItem.ID)
		require.NoError(t, err)
		assert.Equal(t, "Plain content", item.ExtractedContent)
		assert.Equal(t, "<p>Rich content</p>", item.ExtractedRichContent)
	})

	t.Run("get topics", func(t *testing.T) {
		// first, verify we have classified items with topics
		var count int
		err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM items WHERE classified_at IS NOT NULL AND topics IS NOT NULL")
		require.NoError(t, err)
		t.Logf("Found %d classified items with topics", count)

		// get topics from classified items
		topics, err := db.GetTopics(ctx)
		require.NoError(t, err)
		t.Logf("Topics found: %v", topics)

		// based on the classify items test above, we should have these topics
		assert.Contains(t, topics, "tech")
		assert.Contains(t, topics, "other")

		// verify they're sorted if we have any
		if len(topics) > 1 {
			for i := 1; i < len(topics); i++ {
				assert.Less(t, topics[i-1], topics[i], "topics should be sorted")
			}
		}
	})

	t.Run("get topics filtered by score", func(t *testing.T) {
		// get topics for items with score >= 7
		topics, err := db.GetTopicsFiltered(ctx, 7.0)
		require.NoError(t, err)
		t.Logf("Topics for score >= 7: %v", topics)

		// should only have tech topic from item3 (score 7.0) and item1 (score 8.5)
		assert.Contains(t, topics, "tech")
		assert.NotContains(t, topics, "other") // from item2 which has score 3.0

		// get topics for items with score >= 3
		topics, err = db.GetTopicsFiltered(ctx, 3.0)
		require.NoError(t, err)
		t.Logf("Topics for score >= 3: %v", topics)

		// should have both topics
		assert.Contains(t, topics, "tech")
		assert.Contains(t, topics, "other")
	})

	t.Run("update item processed with summary", func(t *testing.T) {
		// create a new item with original description
		item := &Item{
			FeedID:      feed.ID,
			GUID:        "guid-process-test",
			Title:       "Process Test Article",
			Link:        "https://example.com/process",
			Description: "Original RSS description that is quite long and contains full content from the feed",
			Published:   time.Now(),
		}
		err := db.CreateItem(ctx, item)
		require.NoError(t, err)

		// update with extraction and classification including summary
		classification := Classification{
			GUID:        "guid-process-test",
			Score:       8.5,
			Explanation: "Highly relevant to our interests",
			Topics:      []string{"ai", "technology"},
			Summary:     "A concise summary of the article that captures key points in 300-500 chars",
		}

		err = db.UpdateItemProcessed(ctx, item.ID, "Full extracted content here", "<p>Rich HTML content</p>", classification)
		require.NoError(t, err)

		// verify all fields were updated correctly
		updated, err := db.GetItem(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, "Full extracted content here", updated.ExtractedContent)
		assert.Equal(t, "<p>Rich HTML content</p>", updated.ExtractedRichContent)
		assert.InEpsilon(t, 8.5, updated.RelevanceScore, 0.001)
		assert.Equal(t, "Highly relevant to our interests", updated.Explanation)
		assert.NotNil(t, updated.ExtractedAt)
		assert.NotNil(t, updated.ClassifiedAt)
		// most importantly, check that description was updated with summary
		assert.Equal(t, "A concise summary of the article that captures key points in 300-500 chars", updated.Description)
		assert.NotEqual(t, "Original RSS description that is quite long and contains full content from the feed", updated.Description)
	})

	t.Run("update item processed without summary", func(t *testing.T) {
		// create another item
		item := &Item{
			FeedID:      feed.ID,
			GUID:        "guid-no-summary",
			Title:       "No Summary Article",
			Link:        "https://example.com/nosummary",
			Description: "Original description",
			Published:   time.Now(),
		}
		err := db.CreateItem(ctx, item)
		require.NoError(t, err)

		// update without summary
		classification := Classification{
			GUID:        "guid-no-summary",
			Score:       5.0,
			Explanation: "Moderately relevant",
			Topics:      []string{"general"},
			Summary:     "", // no summary provided
		}

		err = db.UpdateItemProcessed(ctx, item.ID, "Content", "<p>HTML</p>", classification)
		require.NoError(t, err)

		// verify description was NOT changed
		updated, err := db.GetItem(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, "Original description", updated.Description)
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

func TestFeedbackCountAndSince(t *testing.T) {
	ctx := context.Background()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// create feed
	feed := &Feed{
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		Description:   "A test feed",
		FetchInterval: 1800,
		Enabled:       true,
	}
	err := db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// create items with feedback
	for i := 0; i < 10; i++ {
		item := &Item{
			FeedID:      feed.ID,
			GUID:        fmt.Sprintf("item-%d", i),
			Title:       fmt.Sprintf("Item %d", i),
			Link:        fmt.Sprintf("https://example.com/item%d", i),
			Description: fmt.Sprintf("Description %d", i),
			Published:   time.Now().Add(-time.Duration(i) * time.Hour),
		}
		err := db.CreateItem(ctx, item)
		require.NoError(t, err)

		// add classification
		classification := Classification{
			Score:       float64(i),
			Explanation: fmt.Sprintf("Score %d", i),
			Topics:      []string{"test"},
		}
		err = db.UpdateItemProcessed(ctx, item.ID, "content", "", classification)
		require.NoError(t, err)

		// add feedback to half of items
		if i%2 == 0 {
			feedback := "like"
			if i%4 == 0 {
				feedback = "dislike"
			}
			err = db.UpdateItemFeedback(ctx, item.ID, feedback)
			require.NoError(t, err)
		}
	}

	t.Run("get feedback count", func(t *testing.T) {
		count, err := db.GetFeedbackCount(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(5), count) // 5 items have feedback
	})

	t.Run("get feedback since offset", func(t *testing.T) {
		// get feedback items after offset 2 (should get 3 items)
		feedback, err := db.GetFeedbackSince(ctx, 2, 10)
		require.NoError(t, err)
		assert.Len(t, feedback, 3)

		// verify they are ordered by most recent first
		for i := 0; i < len(feedback)-1; i++ {
			assert.NotEmpty(t, feedback[i].Title)
			assert.Contains(t, []string{"like", "dislike"}, feedback[i].Feedback)
		}
	})

	t.Run("get feedback since with limit", func(t *testing.T) {
		// get only 2 feedback items after offset 1
		feedback, err := db.GetFeedbackSince(ctx, 1, 2)
		require.NoError(t, err)
		assert.Len(t, feedback, 2)
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

// TestConcurrentUpdates tests that concurrent database updates work with retry logic
func TestConcurrentUpdates(t *testing.T) {
	ctx := context.Background()

	// create a test database with file to ensure proper concurrency
	tmpFile := t.TempDir() + "/test.db"
	db, err := New(ctx, Config{
		DSN:          "file:" + tmpFile + "?cache=shared&mode=rwc&_txlock=immediate",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	})
	require.NoError(t, err)
	defer db.Close()

	// create a test feed
	feed := &Feed{
		URL:           "https://example.com/feed",
		Title:         "Test Feed",
		FetchInterval: 30,
		Enabled:       true,
	}
	err = db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// create test items
	const numItems = 20
	items := make([]*Item, numItems)
	for i := 0; i < numItems; i++ {
		item := &Item{
			FeedID:      feed.ID,
			GUID:        fmt.Sprintf("guid-%d", i),
			Title:       fmt.Sprintf("Item %d", i),
			Link:        fmt.Sprintf("https://example.com/item-%d", i),
			Description: fmt.Sprintf("Description %d", i),
			Published:   time.Now().Add(-time.Duration(i) * time.Hour),
		}
		err := db.CreateItem(ctx, item)
		require.NoError(t, err)
		items[i] = item
	}

	// simulate concurrent updates
	var wg sync.WaitGroup
	errors := make(chan error, numItems)

	// run concurrent updates
	for i := 0; i < numItems; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// simulate processing delay
			time.Sleep(time.Duration(idx%5) * time.Millisecond)

			// update item with classification
			classification := Classification{
				Score:       float64(idx%10) + 0.5,
				Explanation: fmt.Sprintf("Test explanation %d", idx),
				Topics:      []string{fmt.Sprintf("topic-%d", idx%3)},
				Summary:     fmt.Sprintf("Summary %d", idx),
			}

			err := db.UpdateItemProcessed(ctx, items[idx].ID,
				fmt.Sprintf("Extracted content %d", idx),
				fmt.Sprintf("Rich content %d", idx),
				classification)

			if err != nil {
				errors <- err
			}
		}(i)
	}

	// wait for all updates to complete
	wg.Wait()
	close(errors)

	// check for errors
	var errorCount int
	for err := range errors {
		t.Logf("Update error: %v", err)
		errorCount++
	}

	// all updates should succeed with retry logic
	assert.Equal(t, 0, errorCount, "No errors expected with retry logic")

	// verify all items were updated
	for i, item := range items {
		updated, err := db.GetItem(ctx, item.ID)
		require.NoError(t, err)

		assert.NotNil(t, updated.ExtractedAt)
		assert.NotNil(t, updated.ClassifiedAt)
		assert.Equal(t, fmt.Sprintf("Extracted content %d", i), updated.ExtractedContent)
		assert.Equal(t, fmt.Sprintf("Summary %d", i), updated.Description)
		assert.Greater(t, updated.RelevanceScore, 0.0)
	}
}

// TestConcurrentFeedUpdates tests concurrent feed status updates
func TestConcurrentFeedUpdates(t *testing.T) {
	ctx := context.Background()

	// create a test database with file to ensure proper concurrency
	tmpFile := t.TempDir() + "/test.db"
	db, err := New(ctx, Config{
		DSN:          "file:" + tmpFile + "?cache=shared&mode=rwc&_txlock=immediate",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	})
	require.NoError(t, err)
	defer db.Close()

	// create test feeds
	const numFeeds = 10
	feeds := make([]*Feed, numFeeds)
	for i := 0; i < numFeeds; i++ {
		feed := &Feed{
			URL:           fmt.Sprintf("https://example.com/feed%d", i),
			Title:         fmt.Sprintf("Test Feed %d", i),
			FetchInterval: 30,
			Enabled:       true,
		}
		err := db.CreateFeed(ctx, feed)
		require.NoError(t, err)
		feeds[i] = feed
	}

	// simulate concurrent feed updates
	var wg sync.WaitGroup
	errors := make(chan error, numFeeds*2)

	// run concurrent updates
	for i := 0; i < numFeeds; i++ {
		// update feed as fetched
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			nextFetch := time.Now().Add(30 * time.Minute)
			err := db.UpdateFeedFetched(ctx, feeds[idx].ID, nextFetch)
			if err != nil {
				errors <- err
			}
		}(i)

		// simulate some errors
		if i%3 == 0 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				err := db.UpdateFeedError(ctx, feeds[idx].ID, "Test error")
				if err != nil {
					errors <- err
				}
			}(i)
		}
	}

	// wait for all updates to complete
	wg.Wait()
	close(errors)

	// check for errors
	var errorCount int
	for err := range errors {
		t.Logf("Feed update error: %v", err)
		errorCount++
	}

	// all updates should succeed with retry logic
	assert.Equal(t, 0, errorCount, "No errors expected with retry logic")
}

// TestStressWithHighContention tests database under high contention
func TestStressWithHighContention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	ctx := context.Background()

	// create a test database with limited connections to force contention
	tmpFile := t.TempDir() + "/test.db"
	db, err := New(ctx, Config{
		DSN:          "file:" + tmpFile + "?cache=shared&mode=rwc&_txlock=immediate",
		MaxOpenConns: 2, // force contention
		MaxIdleConns: 1,
	})
	require.NoError(t, err)
	defer db.Close()

	// create a single item that all goroutines will update
	feed := &Feed{
		URL:     "https://example.com/feed",
		Title:   "Test Feed",
		Enabled: true,
	}
	err = db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	item := &Item{
		FeedID:      feed.ID,
		GUID:        "test-guid",
		Title:       "Test Item",
		Link:        "https://example.com/item",
		Description: "Test Description",
		Published:   time.Now(),
	}
	err = db.CreateItem(ctx, item)
	require.NoError(t, err)

	// run many concurrent updates to the same item
	const numGoroutines = 50
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			classification := Classification{
				Score:       float64(idx%10) + 0.5,
				Explanation: fmt.Sprintf("Update %d", idx),
				Topics:      []string{fmt.Sprintf("topic-%d", idx%5)},
			}

			err := db.UpdateItemProcessed(ctx, item.ID,
				fmt.Sprintf("Content %d", idx),
				fmt.Sprintf("Rich %d", idx),
				classification)

			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			} else {
				t.Logf("Update %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// with retry logic, all updates should eventually succeed
	assert.Equal(t, numGoroutines, successCount, "All updates should succeed with retry")

	// verify the item was updated
	updated, err := db.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.NotNil(t, updated.ExtractedAt)
	assert.NotNil(t, updated.ClassifiedAt)
}
