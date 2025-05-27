package server

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
)

func TestDBAdapter_ConvertFeeds(t *testing.T) {
	// test data
	dbFeeds := []db.Feed{
		{
			ID:          1,
			URL:         "http://example.com/feed1",
			Title:       "Test Feed 1",
			Description: "Description 1",
		},
		{
			ID:          2,
			URL:         "http://example.com/feed2",
			Title:       "Test Feed 2",
			Description: "",
		},
	}

	// test the conversion logic that DBAdapter.GetFeeds would do
	feeds := make([]types.Feed, len(dbFeeds))
	for i, f := range dbFeeds {
		feeds[i] = types.Feed{
			Title:       f.Title,
			Description: f.Description,
			Link:        f.URL,
		}
	}

	require.Len(t, feeds, 2)

	assert.Equal(t, "Test Feed 1", feeds[0].Title)
	assert.Equal(t, "Description 1", feeds[0].Description)
	assert.Equal(t, "http://example.com/feed1", feeds[0].Link)

	assert.Equal(t, "Test Feed 2", feeds[1].Title)
	assert.Empty(t, feeds[1].Description)
	assert.Equal(t, "http://example.com/feed2", feeds[1].Link)
}

func TestDBAdapter_ConvertItems(t *testing.T) {
	publishTime := time.Now()

	// test data
	dbItems := []db.Item{
		{
			ID:          1,
			FeedID:      1,
			GUID:        "guid1",
			Title:       "Item 1",
			Link:        "http://example.com/item1",
			Description: "Description 1",
			Author:      "Author 1",
			Published:   publishTime,
		},
		{
			ID:          2,
			FeedID:      1,
			GUID:        "guid2",
			Title:       "Item 2",
			Link:        "http://example.com/item2",
			Description: "",
			Author:      "",
			Published:   time.Time{},
		},
	}

	// test the conversion logic that DBAdapter.GetItems would do
	items := make([]types.Item, len(dbItems))
	for i, item := range dbItems {
		items[i] = types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Author:      item.Author,
			Published:   item.Published,
		}
	}

	require.Len(t, items, 2)

	assert.Equal(t, "guid1", items[0].GUID)
	assert.Equal(t, "Item 1", items[0].Title)
	assert.Equal(t, "http://example.com/item1", items[0].Link)
	assert.Equal(t, "Description 1", items[0].Description)
	assert.Equal(t, "Author 1", items[0].Author)
	assert.Equal(t, publishTime, items[0].Published)

	assert.Equal(t, "guid2", items[1].GUID)
	assert.Equal(t, "Item 2", items[1].Title)
	assert.Empty(t, items[1].Description)
	assert.Empty(t, items[1].Author)
	assert.True(t, items[1].Published.IsZero())
}

func TestDBAdapter_GetFeeds(t *testing.T) {
	// create test DB
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// insert test feeds
	feed1 := &db.Feed{
		URL:         "http://example.com/feed1",
		Title:       "Test Feed 1",
		Description: "Description 1",
		Enabled:     true,
	}
	err := testDB.CreateFeed(ctx, feed1)
	require.NoError(t, err)

	feed2 := &db.Feed{
		URL:         "http://example.com/feed2",
		Title:       "Test Feed 2",
		Description: "",
		Enabled:     true,
	}
	err = testDB.CreateFeed(ctx, feed2)
	require.NoError(t, err)

	// create adapter and test GetFeeds
	adapter := &DBAdapter{DB: testDB}
	feeds, err := adapter.GetFeeds(ctx)
	require.NoError(t, err)
	require.Len(t, feeds, 2)

	assert.Equal(t, "Test Feed 1", feeds[0].Title)
	assert.Equal(t, "Description 1", feeds[0].Description)
	assert.Equal(t, "http://example.com/feed1", feeds[0].Link)

	assert.Equal(t, "Test Feed 2", feeds[1].Title)
	assert.Empty(t, feeds[1].Description)
	assert.Equal(t, "http://example.com/feed2", feeds[1].Link)
}

func TestDBAdapter_GetItems(t *testing.T) {
	// create test DB
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create a feed first
	feed := &db.Feed{
		URL:     "http://example.com/feed",
		Title:   "Test Feed",
		Enabled: true,
	}
	err := testDB.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// insert test items
	item1 := &db.Item{
		FeedID:         feed.ID,
		GUID:           "guid1",
		Title:          "Item 1",
		Link:           "http://example.com/item1",
		Description:    "Description 1",
		Author:         "Author 1",
		Published:      time.Now(),
		RelevanceScore: 7.5,
	}
	err = testDB.CreateItem(ctx, item1)
	require.NoError(t, err)

	item2 := &db.Item{
		FeedID:         feed.ID,
		GUID:           "guid2",
		Title:          "Item 2",
		Link:           "http://example.com/item2",
		Description:    "",
		Author:         "",
		Published:      time.Time{},
		RelevanceScore: 8.0,
	}
	err = testDB.CreateItem(ctx, item2)
	require.NoError(t, err)

	// create adapter and test GetItems
	adapter := &DBAdapter{DB: testDB}
	items, err := adapter.GetItems(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "guid1", items[0].GUID)
	assert.Equal(t, "Item 1", items[0].Title)
	assert.Equal(t, "Description 1", items[0].Description)
	assert.Equal(t, "Author 1", items[0].Author)

	assert.Equal(t, "guid2", items[1].GUID)
	assert.Equal(t, "Item 2", items[1].Title)
	assert.Empty(t, items[1].Description)
	assert.Empty(t, items[1].Author)
}

// setupTestDB creates a test database
func setupTestDB(t *testing.T) (testDB *db.DB, cleanup func()) {
	t.Helper()

	tmpfile, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	tmpfile.Close()

	cfg := db.Config{
		DSN:          tmpfile.Name(),
		MaxOpenConns: 1,
	}

	ctx := context.Background()
	testDB, err = db.New(ctx, cfg)
	require.NoError(t, err)

	cleanup = func() {
		testDB.Close()
		os.Remove(tmpfile.Name())
	}

	return testDB, cleanup
}

func TestDBAdapter_GetClassifiedItems(t *testing.T) {
	// create test DB
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create a feed first
	feed := &db.Feed{
		URL:     "http://example.com/feed",
		Title:   "Test Feed",
		Enabled: true,
	}
	err := testDB.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// insert test items with classification
	now := time.Now()
	item1 := &db.Item{
		FeedID:         feed.ID,
		GUID:           "guid1",
		Title:          "Tech Article",
		Link:           "http://example.com/item1",
		Description:    "About AI",
		Published:      now,
		RelevanceScore: 8.5,
		Explanation:    "Very relevant to AI",
		Topics:         db.Topics{"ai", "tech"},
	}
	err = testDB.CreateItem(ctx, item1)
	require.NoError(t, err)
	// mark as classified
	err = testDB.UpdateItemClassification(ctx, item1.ID, 8.5, "Very relevant to AI", []string{"ai", "tech"})
	require.NoError(t, err)

	item2 := &db.Item{
		FeedID:         feed.ID,
		GUID:           "guid2",
		Title:          "Science Article",
		Link:           "http://example.com/item2",
		Description:    "About biology",
		Published:      now.Add(-1 * time.Hour),
		RelevanceScore: 3.0,
		Explanation:    "Not very relevant",
		Topics:         db.Topics{"science", "biology"},
	}
	err = testDB.CreateItem(ctx, item2)
	require.NoError(t, err)
	err = testDB.UpdateItemClassification(ctx, item2.ID, 3.0, "Not very relevant", []string{"science", "biology"})
	require.NoError(t, err)

	// create adapter and test GetClassifiedItems
	adapter := &DBAdapter{DB: testDB}

	// test with min score filter
	items, err := adapter.GetClassifiedItems(ctx, 5.0, "", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Tech Article", items[0].Title)
	assert.Equal(t, "Test Feed", items[0].FeedName)
	assert.InEpsilon(t, 8.5, items[0].RelevanceScore, 0.01)

	// test with topic filter
	items, err = adapter.GetClassifiedItems(ctx, 0.0, "science", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Science Article", items[0].Title)

	// test with both filters
	items, err = adapter.GetClassifiedItems(ctx, 5.0, "ai", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Tech Article", items[0].Title)
}

func TestDBAdapter_GetClassifiedItem(t *testing.T) {
	// create test DB
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create a feed
	feed := &db.Feed{
		URL:     "http://example.com/feed",
		Title:   "Test Feed",
		Enabled: true,
	}
	err := testDB.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// insert test item
	now := time.Now()
	item := &db.Item{
		FeedID:      feed.ID,
		GUID:        "guid1",
		Title:       "Test Article",
		Link:        "http://example.com/item1",
		Description: "Test description",
		Published:   now,
	}
	err = testDB.CreateItem(ctx, item)
	require.NoError(t, err)

	// add extracted content
	err = testDB.UpdateItemExtraction(ctx, item.ID, "Full article content", nil)
	require.NoError(t, err)

	// classify it
	err = testDB.UpdateItemClassification(ctx, item.ID, 7.5, "Relevant article", []string{"test", "demo"})
	require.NoError(t, err)

	// create adapter and test GetClassifiedItem
	adapter := &DBAdapter{DB: testDB}

	result, err := adapter.GetClassifiedItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, item.ID, result.ID)
	assert.Equal(t, "Test Article", result.Title)
	assert.Equal(t, "Test Feed", result.FeedName)
	assert.InEpsilon(t, 7.5, result.RelevanceScore, 0.01)
	assert.Equal(t, "Relevant article", result.Explanation)
	assert.Equal(t, []string{"test", "demo"}, result.Topics)
	assert.Equal(t, "Full article content", result.ExtractedContent)
}

func TestDBAdapter_UpdateItemFeedback(t *testing.T) {
	// create test DB
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create a feed and item
	feed := &db.Feed{
		URL:     "http://example.com/feed",
		Title:   "Test Feed",
		Enabled: true,
	}
	err := testDB.CreateFeed(ctx, feed)
	require.NoError(t, err)

	item := &db.Item{
		FeedID:    feed.ID,
		GUID:      "guid1",
		Title:     "Test Article",
		Link:      "http://example.com/item1",
		Published: time.Now(),
	}
	err = testDB.CreateItem(ctx, item)
	require.NoError(t, err)

	// create adapter and test UpdateItemFeedback
	adapter := &DBAdapter{DB: testDB}

	err = adapter.UpdateItemFeedback(ctx, item.ID, "like")
	require.NoError(t, err)

	// verify feedback was updated
	updatedItem, err := testDB.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "like", updatedItem.UserFeedback)
	assert.NotNil(t, updatedItem.FeedbackAt)
}

func TestDBAdapter_GetTopics(t *testing.T) {
	// create test DB
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create a feed
	feed := &db.Feed{
		URL:     "http://example.com/feed",
		Title:   "Test Feed",
		Enabled: true,
	}
	err := testDB.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// create items with various topics
	// note: we need to classify items for topics to be populated properly
	for i, data := range []struct {
		title  string
		topics []string
	}{
		{"Item 1", []string{"tech", "ai"}},
		{"Item 2", []string{"science", "ai"}},
		{"Item 3", []string{"tech", "web"}},
	} {
		item := &db.Item{
			FeedID:    feed.ID,
			GUID:      fmt.Sprintf("guid%d", i),
			Title:     data.title,
			Link:      fmt.Sprintf("http://example.com/item%d", i),
			Published: time.Now(),
		}
		err = testDB.CreateItem(ctx, item)
		require.NoError(t, err)

		// classify to set topics
		err = testDB.UpdateItemClassification(ctx, item.ID, 5.0, "test", data.topics)
		require.NoError(t, err)
	}

	// create adapter and test GetTopics
	adapter := &DBAdapter{DB: testDB}

	topics, err := adapter.GetTopics(ctx)
	require.NoError(t, err)

	// should have unique topics
	assert.Len(t, topics, 4) // tech, ai, science, web

	// verify all expected topics are present
	topicMap := make(map[string]bool)
	for _, topic := range topics {
		topicMap[topic] = true
	}

	assert.True(t, topicMap["tech"])
	assert.True(t, topicMap["ai"])
	assert.True(t, topicMap["science"])
	assert.True(t, topicMap["web"])
}
