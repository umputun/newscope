package scheduler

//go:generate moq -out mocks/db.go -pkg mocks -skip-ensure -fmt goimports . Database
//go:generate moq -out mocks/parser.go -pkg mocks -skip-ensure -fmt goimports . Parser
//go:generate moq -out mocks/extractor.go -pkg mocks -skip-ensure -fmt goimports . Extractor

import (
	"context"
	"errors"
	"fmt"
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

func TestNewScheduler(t *testing.T) {
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	t.Run("with defaults", func(t *testing.T) {
		cfg := Config{}
		s := NewScheduler(mockDB, mockParser, mockExtractor, cfg)

		assert.Equal(t, 30*time.Minute, s.updateInterval)
		assert.Equal(t, 5*time.Minute, s.extractInterval)
		assert.Equal(t, 5, s.maxWorkers)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := Config{
			UpdateInterval:  1 * time.Hour,
			ExtractInterval: 10 * time.Minute,
			MaxWorkers:      10,
		}
		s := NewScheduler(mockDB, mockParser, mockExtractor, cfg)

		assert.Equal(t, 1*time.Hour, s.updateInterval)
		assert.Equal(t, 10*time.Minute, s.extractInterval)
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
			ID:    1,
			URL:   "http://example.com/feed.xml",
			Title: "Test Feed",
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

		updateFeedCalled := false
		mockDB.UpdateFeedFunc = func(ctx context.Context, feed *db.Feed) error {
			updateFeedCalled = true
			assert.Equal(t, parsedFeed.Title, feed.Title)
			assert.Equal(t, parsedFeed.Description, feed.Description.String)
			assert.True(t, feed.Description.Valid)
			return nil
		}

		createItemCount := 0
		mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
			createItemCount++
			item.ID = int64(createItemCount) // simulate successful creation
			return nil
		}

		mockDB.UpdateFeedLastFetchedFunc = func(ctx context.Context, feedID int64, lastFetched time.Time) error {
			assert.Equal(t, testFeed.ID, feedID)
			return nil
		}

		s := NewScheduler(mockDB, mockParser, mockExtractor, Config{})
		s.updateFeed(ctx, testFeed)

		assert.True(t, updateFeedCalled)
		assert.Equal(t, 2, createItemCount)
	})

	t.Run("parse error", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}

		testFeed := db.Feed{
			ID:  1,
			URL: "http://example.com/feed.xml",
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

		s := NewScheduler(mockDB, mockParser, mockExtractor, Config{})
		s.updateFeed(ctx, testFeed)

		assert.True(t, errorUpdated)
	})
}

func TestScheduler_ExtractItemContent(t *testing.T) {
	ctx := context.Background()

	t.Run("successful extraction", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}

		testItem := db.Item{
			ID:    1,
			Title: "Test Item",
			Link:  "http://example.com/article",
		}

		extractResult := &content.ExtractResult{
			Content: "This is the extracted content",
			Title:   "Test Item",
			URL:     "http://example.com/article",
		}

		mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
			assert.Equal(t, testItem.Link, url)
			return extractResult, nil
		}

		contentCreated := false
		mockDB.CreateContentFunc = func(ctx context.Context, content *db.Content) error {
			contentCreated = true
			assert.Equal(t, testItem.ID, content.ItemID)
			assert.Equal(t, extractResult.Content, content.FullContent)
			assert.False(t, content.ExtractionError.Valid)
			return nil
		}

		s := NewScheduler(mockDB, mockParser, mockExtractor, Config{})
		s.extractItemContent(ctx, testItem)

		assert.True(t, contentCreated)
	})

	t.Run("extraction error", func(t *testing.T) {
		mockDB := &mocks.DatabaseMock{}
		mockParser := &mocks.ParserMock{}
		mockExtractor := &mocks.ExtractorMock{}

		testItem := db.Item{
			ID:   1,
			Link: "http://example.com/article",
		}

		extractErr := errors.New("extraction failed")
		mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
			return nil, extractErr
		}

		errorStored := false
		mockDB.CreateContentFunc = func(ctx context.Context, content *db.Content) error {
			errorStored = true
			assert.Equal(t, testItem.ID, content.ItemID)
			assert.True(t, content.ExtractionError.Valid)
			assert.Equal(t, extractErr.Error(), content.ExtractionError.String)
			return nil
		}

		s := NewScheduler(mockDB, mockParser, mockExtractor, Config{})
		s.extractItemContent(ctx, testItem)

		assert.True(t, errorStored)
	})
}

func TestScheduler_UpdateFeedNow(t *testing.T) {
	ctx := context.Background()
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	testFeed := &db.Feed{
		ID:  1,
		URL: "http://example.com/feed.xml",
	}

	parsedFeed := &types.Feed{
		Title: "Test Feed",
		Items: []types.Item{},
	}

	mockDB.GetFeedFunc = func(ctx context.Context, id int64) (*db.Feed, error) {
		assert.Equal(t, int64(1), id)
		return testFeed, nil
	}

	mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
		assert.Equal(t, testFeed.URL, url)
		return parsedFeed, nil
	}

	mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
		return nil
	}

	mockDB.UpdateFeedFunc = func(ctx context.Context, feed *db.Feed) error {
		return nil
	}

	mockDB.UpdateFeedLastFetchedFunc = func(ctx context.Context, feedID int64, lastFetched time.Time) error {
		assert.Equal(t, testFeed.ID, feedID)
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, Config{})
	err := s.UpdateFeedNow(ctx, 1)
	require.NoError(t, err)
}

func TestScheduler_ExtractContentNow(t *testing.T) {
	ctx := context.Background()
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	testItem := &db.Item{
		ID:   1,
		Link: "http://example.com/article",
	}

	extractResult := &content.ExtractResult{
		Content: "Extracted content",
	}

	mockDB.GetItemFunc = func(ctx context.Context, id int64) (*db.Item, error) {
		assert.Equal(t, int64(1), id)
		return testItem, nil
	}

	mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		assert.Equal(t, testItem.Link, url)
		return extractResult, nil
	}

	mockDB.CreateContentFunc = func(ctx context.Context, content *db.Content) error {
		assert.Equal(t, testItem.ID, content.ItemID)
		assert.Equal(t, extractResult.Content, content.FullContent)
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, Config{})
	err := s.ExtractContentNow(ctx, 1)
	require.NoError(t, err)
}

func TestScheduler_StartStop(t *testing.T) {
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	// mock expectations for the initial feed update
	mockDB.GetEnabledFeedsFunc = func(ctx context.Context) ([]db.Feed, error) {
		return []db.Feed{}, nil
	}
	mockDB.GetItemsForExtractionFunc = func(ctx context.Context, limit int) ([]db.Item, error) {
		return []db.Item{}, nil
	}

	cfg := Config{
		UpdateInterval:  100 * time.Millisecond,
		ExtractInterval: 100 * time.Millisecond,
	}
	s := NewScheduler(mockDB, mockParser, mockExtractor, cfg)

	ctx := context.Background()
	s.Start(ctx)

	// let it run briefly
	time.Sleep(50 * time.Millisecond)

	// stop should complete without hanging
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() timed out")
	}
}

func TestScheduler_extractPendingContent(t *testing.T) {
	ctx := context.Background()
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	pendingItems := []db.Item{
		{
			ID:    1,
			Title: "Item 1",
			Link:  "http://example.com/1",
		},
		{
			ID:    2,
			Title: "Item 2",
			Link:  "http://example.com/2",
		},
	}

	mockDB.GetItemsForExtractionFunc = func(ctx context.Context, limit int) ([]db.Item, error) {
		assert.Equal(t, 5, limit) // default max workers
		return pendingItems, nil
	}

	extractedContent := &content.ExtractResult{
		Content: "Extracted content",
		Title:   "Article Title",
	}

	extractCallCount := 0
	mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		extractCallCount++
		if url == "http://example.com/2" {
			return nil, fmt.Errorf("extraction failed")
		}
		return extractedContent, nil
	}

	createContentCallCount := 0
	mockDB.CreateContentFunc = func(ctx context.Context, content *db.Content) error {
		createContentCallCount++
		switch content.ItemID {
		case 1:
			assert.Equal(t, "Extracted content", content.FullContent)
			assert.False(t, content.ExtractionError.Valid)
		case 2:
			assert.Empty(t, content.FullContent)
			assert.True(t, content.ExtractionError.Valid)
			assert.Contains(t, content.ExtractionError.String, "extraction failed")
		}
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, Config{})
	s.extractPendingContent(ctx)

	assert.Equal(t, 2, extractCallCount)
	assert.Equal(t, 2, createContentCallCount)
}

func TestScheduler_updateAllFeeds(t *testing.T) {
	ctx := context.Background()
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	testFeeds := []db.Feed{
		{
			ID:    1,
			URL:   "http://example.com/feed1.xml",
			Title: "Feed 1",
		},
		{
			ID:    2,
			URL:   "http://example.com/feed2.xml",
			Title: "Feed 2",
		},
	}

	mockDB.GetEnabledFeedsFunc = func(ctx context.Context) ([]db.Feed, error) {
		return testFeeds, nil
	}

	parsedFeed := &types.Feed{
		Title:       "Updated Feed",
		Description: "Updated Description",
		Items: []types.Item{
			{
				GUID:      "item1",
				Title:     "New Item",
				Link:      "http://example.com/new",
				Published: time.Now(),
			},
		},
	}

	parseCallCount := 0
	mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
		parseCallCount++
		if url == "http://example.com/feed2.xml" {
			return nil, fmt.Errorf("parse error")
		}
		return parsedFeed, nil
	}

	mockDB.UpdateFeedFunc = func(ctx context.Context, feed *db.Feed) error {
		return nil
	}

	mockDB.UpdateFeedErrorFunc = func(ctx context.Context, feedID int64, errMsg string) error {
		assert.Equal(t, int64(2), feedID)
		assert.Contains(t, errMsg, "parse error")
		return nil
	}

	mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
		item.ID = 1 // simulate successful creation
		return nil
	}

	mockDB.UpdateFeedLastFetchedFunc = func(ctx context.Context, feedID int64, lastFetched time.Time) error {
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, Config{MaxWorkers: 2})
	s.updateAllFeeds(ctx)

	assert.Equal(t, 2, parseCallCount)
}

func TestScheduler_periodicUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	updateCount := 0
	var mu sync.Mutex
	mockDB.GetEnabledFeedsFunc = func(ctx context.Context) ([]db.Feed, error) {
		mu.Lock()
		updateCount++
		mu.Unlock()
		return []db.Feed{}, nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, Config{
		UpdateInterval:  50 * time.Millisecond,
		ExtractInterval: 1 * time.Hour, // don't run extraction
	})

	// start the scheduler
	go s.Start(ctx)

	// wait for at least 2 updates
	time.Sleep(150 * time.Millisecond)
	cancel()

	// wait for graceful shutdown
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.GreaterOrEqual(t, updateCount, 2)
	mu.Unlock()
}
