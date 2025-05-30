package scheduler

//go:generate moq -out mocks/db.go -pkg mocks -skip-ensure -fmt goimports . Database
//go:generate moq -out mocks/parser.go -pkg mocks -skip-ensure -fmt goimports . Parser
//go:generate moq -out mocks/extractor.go -pkg mocks -skip-ensure -fmt goimports . Extractor
//go:generate moq -out mocks/classifier.go -pkg mocks -skip-ensure -fmt goimports . Classifier

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
	"github.com/umputun/newscope/pkg/scheduler/mocks"
)

// setupTestDB sets up a database mock with default implementations
func setupTestDB() *mocks.DatabaseMock {
	mockDB := &mocks.DatabaseMock{}

	// default implementations that tests can override
	mockDB.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		return false, nil
	}

	mockDB.ItemExistsByTitleOrURLFunc = func(ctx context.Context, title, url string) (bool, error) {
		return false, nil
	}

	return mockDB
}

func TestNewScheduler(t *testing.T) {
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}
	mockClassifier := &mocks.ClassifierMock{}

	t.Run("with defaults", func(t *testing.T) {
		cfg := Config{}
		s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, cfg)

		assert.Equal(t, 30*time.Minute, s.updateInterval)
		assert.Equal(t, 5, s.maxWorkers)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := Config{
			UpdateInterval: 1 * time.Hour,
			MaxWorkers:     10,
		}
		s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, cfg)

		assert.Equal(t, 1*time.Hour, s.updateInterval)
		assert.Equal(t, 10, s.maxWorkers)
	})
}

func TestScheduler_UpdateFeed(t *testing.T) {
	ctx := context.Background()

	t.Run("successful update with new items", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}

		testFeed := db.Feed{
			ID:            1,
			URL:           "http://example.com/feed.xml",
			Title:         "Test Feed",
			FetchInterval: 1800,
		}

		parsedFeed := &types.Feed{
			Title:       "Test Feed Updated",
			Description: "Updated description",
			Items: []types.Item{
				{
					GUID:        "item1",
					Title:       "Item 1",
					Link:        "http://example.com/1",
					Description: "Description 1",
					Published:   time.Now(),
				},
				{
					GUID:        "item2",
					Title:       "Item 2",
					Link:        "http://example.com/2",
					Description: "Description 2",
					Published:   time.Now(),
				},
			},
		}

		mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
			assert.Equal(t, testFeed.URL, url)
			return parsedFeed, nil
		}

		// mock ItemExists to return false for all items
		mockDB.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
			return false, nil
		}

		// mock ItemExistsByTitleOrURL to return false for all items
		mockDB.ItemExistsByTitleOrURLFunc = func(ctx context.Context, title, url string) (bool, error) {
			return false, nil
		}

		createItemCount := 0
		mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
			createItemCount++
			item.ID = int64(createItemCount) // simulate successful creation
			return nil
		}

		mockDB.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
			assert.Equal(t, testFeed.ID, feedID)
			return nil
		}

		// create a channel to capture items
		processCh := make(chan db.Item, 10)
		defer close(processCh)

		s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
		s.updateFeed(ctx, testFeed, processCh)

		assert.Equal(t, 2, createItemCount)
		// check that items were sent to channel
		assert.Len(t, processCh, 2)
	})

	t.Run("parse error", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}

		testFeed := db.Feed{
			ID:            1,
			URL:           "http://example.com/feed.xml",
			FetchInterval: 1800,
		}

		parseErr := errors.New("parse error")
		mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
			return nil, parseErr
		}

		errorUpdated := false
		mockDB.UpdateFeedErrorFunc = func(ctx context.Context, feedID int64, errMsg string) error {
			errorUpdated = true
			assert.Equal(t, testFeed.ID, feedID)
			assert.Equal(t, parseErr.Error(), errMsg)
			return nil
		}

		processCh := make(chan db.Item, 10)
		defer close(processCh)

		s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
		s.updateFeed(ctx, testFeed, processCh)

		assert.True(t, errorUpdated)
		// no items should be sent to channel
		assert.Empty(t, processCh)
	})

	t.Run("duplicate item by title or URL", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}

		testFeed := db.Feed{
			ID:            1,
			URL:           "http://example.com/feed.xml",
			FetchInterval: 1800,
		}

		parsedFeed := &types.Feed{
			Title: "Test Feed",
			Items: []types.Item{
				{GUID: "item1", Title: "Duplicate Article", Link: "http://example.com/1"},
				{GUID: "item2", Title: "New Article", Link: "http://example.com/2"},
			},
		}

		mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
			return parsedFeed, nil
		}

		// mock ItemExists to return false for all items
		mockDB.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
			return false, nil
		}

		// mock ItemExistsByTitleOrURL to return true for first item (duplicate)
		mockDB.ItemExistsByTitleOrURLFunc = func(ctx context.Context, title, url string) (bool, error) {
			if title == "Duplicate Article" || url == "http://example.com/1" {
				return true, nil
			}
			return false, nil
		}

		createItemCount := 0
		mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
			createItemCount++
			item.ID = int64(createItemCount)
			return nil
		}

		mockDB.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
			return nil
		}

		processCh := make(chan db.Item, 10)
		defer close(processCh)

		s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
		s.updateFeed(ctx, testFeed, processCh)

		// only one item should be created (the non-duplicate)
		assert.Equal(t, 1, createItemCount)
		assert.Len(t, processCh, 1)
	})
}

func TestScheduler_ProcessItem(t *testing.T) {
	ctx := context.Background()

	t.Run("successful extraction and classification", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}
		mockClassifier := &mocks.ClassifierMock{}

		testItem := db.Item{
			ID:    1,
			Title: "Test Item",
			Link:  "http://example.com/article",
		}

		extractResult := &content.ExtractResult{
			Content:     "This is the extracted content",
			RichContent: "<p>This is the extracted content</p>",
			Title:       "Test Item",
			URL:         "http://example.com/article",
		}

		mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
			assert.Equal(t, testItem.Link, url)
			return extractResult, nil
		}

		classification := db.Classification{
			GUID:        testItem.GUID,
			Score:       8.5,
			Explanation: "Highly relevant",
			Topics:      []string{"tech", "news"},
		}

		mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string) ([]db.Classification, error) {
			assert.Len(t, articles, 1)
			assert.Equal(t, extractResult.Content, articles[0].ExtractedContent)
			return []db.Classification{classification}, nil
		}

		processedUpdated := false
		mockDB.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, content, richContent string, c db.Classification) error {
			processedUpdated = true
			assert.Equal(t, testItem.ID, itemID)
			assert.Equal(t, extractResult.Content, content)
			assert.Equal(t, extractResult.RichContent, richContent)
			assert.InEpsilon(t, classification.Score, c.Score, 0.001)
			assert.Equal(t, classification.Explanation, c.Explanation)
			assert.Equal(t, classification.Topics, c.Topics)
			return nil
		}

		feedbacks := []db.FeedbackExample{}
		topics := []string{"tech", "news", "programming"}
		s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, Config{})
		s.processItem(ctx, testItem, feedbacks, topics)

		assert.True(t, processedUpdated)
	})

	t.Run("extraction error", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}
		mockClassifier := &mocks.ClassifierMock{}

		testItem := db.Item{
			ID:   1,
			Link: "http://example.com/article",
		}

		extractErr := errors.New("extraction failed")
		mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
			return nil, extractErr
		}

		// should save extraction error to database
		mockDB.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, content, richContent string, err error) error {
			assert.Equal(t, int64(1), itemID)
			assert.Empty(t, content)
			assert.Empty(t, richContent)
			assert.Equal(t, extractErr, err)
			return nil
		}

		// should not call classifier or update database
		mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string) ([]db.Classification, error) {
			t.Fatal("classifier should not be called on extraction error")
			return nil, nil
		}

		mockDB.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, content, richContent string, classification db.Classification) error {
			t.Fatal("database should not be updated on extraction error")
			return nil
		}

		feedbacks := []db.FeedbackExample{}
		topics := []string{"tech", "news", "programming"}
		s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, Config{})
		s.processItem(ctx, testItem, feedbacks, topics)
	})

	t.Run("classification error", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}
		mockClassifier := &mocks.ClassifierMock{}

		testItem := db.Item{
			ID:   1,
			Link: "http://example.com/article",
		}

		extractResult := &content.ExtractResult{
			Content:     "This is the extracted content",
			RichContent: "<p>This is the extracted content</p>",
		}

		mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
			return extractResult, nil
		}

		classifyErr := errors.New("classification failed")
		mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string) ([]db.Classification, error) {
			return nil, classifyErr
		}

		// should not update database on classification error
		mockDB.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, content, richContent string, classification db.Classification) error {
			t.Fatal("database should not be updated on classification error")
			return nil
		}

		feedbacks := []db.FeedbackExample{}
		topics := []string{"tech", "news", "programming"}
		s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, Config{})
		s.processItem(ctx, testItem, feedbacks, topics)
	})
}

func TestScheduler_UpdateFeedNow(t *testing.T) {
	ctx := context.Background()

	mockDB := setupTestDB()
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}
	mockClassifier := &mocks.ClassifierMock{}

	feedID := int64(123)
	testFeed := &db.Feed{
		ID:    feedID,
		URL:   "http://example.com/feed.xml",
		Title: "Test Feed",
	}

	mockDB.GetFeedFunc = func(ctx context.Context, id int64) (*db.Feed, error) {
		assert.Equal(t, feedID, id)
		return testFeed, nil
	}

	parsedFeed := &types.Feed{
		Items: []types.Item{{
			GUID:  "item1",
			Title: "New Item",
			Link:  "http://example.com/new",
		}},
	}

	mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
		return parsedFeed, nil
	}

	mockDB.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		return false, nil
	}

	mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
		item.ID = 1
		return nil
	}

	mockDB.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	// mock processing - add extractor and classifier mocks
	mockDB.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error) {
		return []db.FeedbackExample{}, nil
	}

	mockDB.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech", "news", "programming"}, nil
	}

	mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{
			Content:     "Test content",
			RichContent: "<p>Test content</p>",
		}, nil
	}

	mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string) ([]db.Classification, error) {
		return []db.Classification{{
			Score:       7.0,
			Explanation: "Test",
			Topics:      []string{"test"},
		}}, nil
	}

	mockDB.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, content, richContent string, c db.Classification) error {
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, Config{})
	err := s.UpdateFeedNow(ctx, feedID)
	require.NoError(t, err)
}

func TestScheduler_ExtractContentNow(t *testing.T) {
	ctx := context.Background()

	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}
	mockClassifier := &mocks.ClassifierMock{}

	itemID := int64(456)
	testItem := &db.Item{
		ID:    itemID,
		Title: "Test Item",
		Link:  "http://example.com/article",
	}

	mockDB.GetItemFunc = func(ctx context.Context, id int64) (*db.Item, error) {
		assert.Equal(t, itemID, id)
		return testItem, nil
	}

	mockDB.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error) {
		return []db.FeedbackExample{}, nil
	}

	mockDB.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech", "news", "programming"}, nil
	}

	extractResult := &content.ExtractResult{
		Content:     "Extracted content",
		RichContent: "<p>Extracted content</p>",
	}

	mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return extractResult, nil
	}

	classification := db.Classification{
		Score:       7.5,
		Explanation: "Relevant",
		Topics:      []string{"tech"},
	}

	mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string) ([]db.Classification, error) {
		return []db.Classification{classification}, nil
	}

	mockDB.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, content, richContent string, c db.Classification) error {
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, Config{})
	err := s.ExtractContentNow(ctx, itemID)
	require.NoError(t, err)
}

func TestScheduler_StartStop(t *testing.T) {
	// setup mocks
	mockDB := setupTestDB()
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}
	mockClassifier := &mocks.ClassifierMock{}

	// track what happened
	var mu sync.Mutex
	feedsFetched := 0
	itemsCreated := 0
	itemsProcessed := 0

	// mock feed list
	mockDB.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]db.Feed, error) {
		return []db.Feed{
			{ID: 1, URL: "http://feed1.com/rss", Title: "Feed 1", Enabled: true, FetchInterval: 1800},
			{ID: 2, URL: "http://feed2.com/rss", Title: "Feed 2", Enabled: true, FetchInterval: 1800},
		}, nil
	}

	// mock feed parsing
	mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
		mu.Lock()
		feedsFetched++
		mu.Unlock()

		switch url {
		case "http://feed1.com/rss":
			return &types.Feed{
				Title: "Feed 1",
				Items: []types.Item{
					{GUID: "item1", Title: "Article 1", Link: "http://feed1.com/1"},
					{GUID: "item2", Title: "Article 2", Link: "http://feed1.com/2"},
				},
			}, nil
		case "http://feed2.com/rss":
			return &types.Feed{
				Title: "Feed 2",
				Items: []types.Item{
					{GUID: "item3", Title: "Article 3", Link: "http://feed2.com/3"},
				},
			}, nil
		}
		return nil, assert.AnError
	}

	// mock item existence check - track what we've seen
	seenItems := make(map[string]bool)
	mockDB.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		if seenItems[guid] {
			return true, nil
		}
		seenItems[guid] = true
		return false, nil
	}

	// mock item creation
	createdItems := make([]db.Item, 0)
	mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
		mu.Lock()
		itemsCreated++
		item.ID = int64(itemsCreated)
		createdItems = append(createdItems, *item)
		mu.Unlock()
		return nil
	}

	// mock feed update
	mockDB.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	// mock feedback
	mockDB.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error) {
		return []db.FeedbackExample{
			{Title: "Good article", Feedback: "like", Topics: []string{"tech"}},
		}, nil
	}

	mockDB.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech", "news", "programming"}, nil
	}

	// mock extraction
	mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{
			Content:     "Extracted content for " + url,
			RichContent: "<p>Extracted content for " + url + "</p>",
		}, nil
	}

	// mock classification
	mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string) ([]db.Classification, error) {
		return []db.Classification{{
			GUID:        articles[0].GUID,
			Score:       8.5,
			Explanation: "Highly relevant",
			Topics:      []string{"tech", "news"},
		}}, nil
	}

	// mock processing update
	processedItems := make(map[int64]bool)
	mockDB.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, content, richContent string, classification db.Classification) error {
		mu.Lock()
		itemsProcessed++
		processedItems[itemID] = true
		mu.Unlock()
		return nil
	}

	// create scheduler with short update interval for testing
	cfg := Config{
		UpdateInterval: 100 * time.Millisecond,
		MaxWorkers:     2,
	}
	scheduler := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, cfg)

	// start scheduler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)

	// wait for initial feed update and processing
	time.Sleep(150 * time.Millisecond)

	// verify initial processing
	mu.Lock()
	assert.GreaterOrEqual(t, feedsFetched, 2, "should fetch both feeds at least once")
	assert.Equal(t, 3, itemsCreated, "should create 3 items")
	assert.Equal(t, 3, itemsProcessed, "should process all 3 items")
	mu.Unlock()

	// stop scheduler
	scheduler.Stop()

	// verify all created items were processed
	for _, item := range createdItems {
		assert.True(t, processedItems[item.ID], "item %d should be processed", item.ID)
	}
}

func TestScheduler_GracefulShutdown(t *testing.T) {
	mockDB := setupTestDB()
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}
	mockClassifier := &mocks.ClassifierMock{}

	processingStarted := make(chan struct{})
	processingBlocked := make(chan struct{})
	processingComplete := false

	// setup slow processing to test graceful shutdown
	mockDB.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]db.Feed, error) {
		return []db.Feed{{ID: 1, URL: "http://test.com", Enabled: true}}, nil
	}

	mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
		return &types.Feed{
			Items: []types.Item{{GUID: "item1", Title: "Test", Link: "http://test.com/1"}},
		}, nil
	}

	mockDB.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		return false, nil
	}

	mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
		item.ID = 1
		return nil
	}

	mockDB.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	mockDB.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error) {
		return []db.FeedbackExample{}, nil
	}

	mockDB.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech", "news", "programming"}, nil
	}

	// slow extraction to simulate work in progress
	mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		select {
		case <-processingStarted:
			// already signaled
		default:
			close(processingStarted)
		}

		<-processingBlocked // block until test releases
		return &content.ExtractResult{Content: "Test"}, nil
	}

	mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string) ([]db.Classification, error) {
		return []db.Classification{{Score: 5.0}}, nil
	}

	mockDB.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, content, richContent string, c db.Classification) error {
		processingComplete = true
		return nil
	}

	scheduler := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, Config{
		UpdateInterval: 50 * time.Millisecond,
		MaxWorkers:     1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// wait for processing to start
	<-processingStarted

	// cancel context (simulate shutdown)
	cancel()

	// release the blocked processing
	close(processingBlocked)

	// stop should wait for in-flight work
	scheduler.Stop()

	// verify processing completed despite shutdown
	assert.True(t, processingComplete, "in-flight processing should complete")
}
