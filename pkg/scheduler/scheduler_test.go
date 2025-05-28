package scheduler

//go:generate moq -out mocks/db.go -pkg mocks -skip-ensure -fmt goimports . Database
//go:generate moq -out mocks/parser.go -pkg mocks -skip-ensure -fmt goimports . Parser
//go:generate moq -out mocks/extractor.go -pkg mocks -skip-ensure -fmt goimports . Extractor
//go:generate moq -out mocks/classifier.go -pkg mocks -skip-ensure -fmt goimports . Classifier

import (
	"context"
	"errors"
	"fmt"
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
	mockClassifier := &mocks.ClassifierMock{}

	t.Run("with defaults", func(t *testing.T) {
		cfg := Config{}
		s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, cfg)

		assert.Equal(t, 30*time.Minute, s.updateInterval)
		assert.Equal(t, 5*time.Minute, s.extractInterval)
		assert.Equal(t, 10*time.Minute, s.classifyInterval)
		assert.Equal(t, 5, s.maxWorkers)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := Config{
			UpdateInterval:   1 * time.Hour,
			ExtractInterval:  10 * time.Minute,
			ClassifyInterval: 15 * time.Minute,
			MaxWorkers:       10,
		}
		s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, cfg)

		assert.Equal(t, 1*time.Hour, s.updateInterval)
		assert.Equal(t, 10*time.Minute, s.extractInterval)
		assert.Equal(t, 15*time.Minute, s.classifyInterval)
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

		s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
		s.updateFeed(ctx, testFeed)

		assert.Equal(t, 2, createItemCount)
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

		s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
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
			Content:     "This is the extracted content",
			RichContent: "<p>This is the extracted content</p>",
			Title:       "Test Item",
			URL:         "http://example.com/article",
		}

		mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
			assert.Equal(t, testItem.Link, url)
			return extractResult, nil
		}

		contentUpdated := false
		mockDB.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, content, richContent string, err error) error {
			contentUpdated = true
			assert.Equal(t, testItem.ID, itemID)
			assert.Equal(t, extractResult.Content, content)
			assert.Equal(t, extractResult.RichContent, richContent)
			assert.NoError(t, err)
			return nil
		}

		s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
		s.extractItemContent(ctx, testItem)

		assert.True(t, contentUpdated)
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
		mockDB.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, content, richContent string, err error) error {
			errorStored = true
			assert.Equal(t, testItem.ID, itemID)
			assert.Empty(t, content)
			assert.Empty(t, richContent)
			assert.Equal(t, extractErr, err)
			return nil
		}

		s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
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
		ID:            1,
		URL:           "http://example.com/feed.xml",
		FetchInterval: 300, // 5 minutes in seconds
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

	mockDB.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		return false, nil
	}

	mockDB.CreateItemFunc = func(ctx context.Context, item *db.Item) error {
		return nil
	}

	mockDB.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		assert.Equal(t, testFeed.ID, feedID)
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
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
		Content:     "Extracted content",
		RichContent: "<p>Extracted content</p>",
	}

	mockDB.GetItemFunc = func(ctx context.Context, id int64) (*db.Item, error) {
		assert.Equal(t, int64(1), id)
		return testItem, nil
	}

	mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		assert.Equal(t, testItem.Link, url)
		return extractResult, nil
	}

	mockDB.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, content, richContent string, err error) error {
		assert.Equal(t, testItem.ID, itemID)
		assert.Equal(t, extractResult.Content, content)
		assert.Equal(t, extractResult.RichContent, richContent)
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
	err := s.ExtractContentNow(ctx, 1)
	require.NoError(t, err)
}

func TestScheduler_StartStop(t *testing.T) {
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	// mock expectations for the initial feed update
	mockDB.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]db.Feed, error) {
		return []db.Feed{}, nil
	}
	mockDB.GetItemsNeedingExtractionFunc = func(ctx context.Context, limit int) ([]db.Item, error) {
		return []db.Item{}, nil
	}

	cfg := Config{
		UpdateInterval:  100 * time.Millisecond,
		ExtractInterval: 100 * time.Millisecond,
	}
	s := NewScheduler(mockDB, mockParser, mockExtractor, nil, cfg)

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

	mockDB.GetItemsNeedingExtractionFunc = func(ctx context.Context, limit int) ([]db.Item, error) {
		assert.Equal(t, 5, limit) // default max workers
		return pendingItems, nil
	}

	extractedContent := &content.ExtractResult{
		Content:     "Extracted content",
		RichContent: "<p>Rich content</p>",
		Title:       "Article Title",
	}

	mockExtractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		if url == "http://example.com/2" {
			return nil, fmt.Errorf("extraction failed")
		}
		return extractedContent, nil
	}

	mockDB.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, content, richContent string, err error) error {
		switch itemID {
		case 1:
			assert.Equal(t, "Extracted content", content)
			assert.Equal(t, "<p>Rich content</p>", richContent)
			assert.NoError(t, err) //nolint:testifylint // inside mock function
		case 2:
			assert.Empty(t, content)
			assert.Empty(t, richContent)
			assert.NotNil(t, err) //nolint:testifylint // inside mock function
			assert.Contains(t, err.Error(), "extraction failed")
		}
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{})
	s.extractPendingContent(ctx)

	assert.Len(t, mockExtractor.ExtractCalls(), 2)
	assert.Len(t, mockDB.UpdateItemExtractionCalls(), 2)
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

	mockDB.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]db.Feed, error) {
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

	mockParser.ParseFunc = func(ctx context.Context, url string) (*types.Feed, error) {
		if url == "http://example.com/feed2.xml" {
			return nil, fmt.Errorf("parse error")
		}
		return parsedFeed, nil
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

	mockDB.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		return false, nil
	}

	mockDB.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{MaxWorkers: 2})
	s.updateAllFeeds(ctx)

	assert.Len(t, mockParser.ParseCalls(), 2)
}

func TestScheduler_periodicUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}

	mockDB.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]db.Feed, error) {
		return []db.Feed{}, nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, nil, Config{
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

	assert.GreaterOrEqual(t, len(mockDB.GetFeedsCalls()), 2)
}

func TestScheduler_classifyPendingItems(t *testing.T) {
	ctx := context.Background()
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}
	mockClassifier := &mocks.ClassifierMock{}

	testItems := []db.Item{
		{
			ID:               1,
			GUID:             "guid1",
			Title:            "Article 1",
			ExtractedContent: "Content 1",
		},
		{
			ID:               2,
			GUID:             "guid2",
			Title:            "Article 2",
			ExtractedContent: "Content 2",
		},
	}

	mockDB.GetUnclassifiedItemsFunc = func(ctx context.Context, limit int) ([]db.Item, error) {
		return testItems, nil
	}

	mockDB.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error) {
		return []db.FeedbackExample{
			{
				Title:    "Previous Article",
				Feedback: "like",
				Topics:   []string{"tech"},
			},
		}, nil
	}

	mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample) ([]db.Classification, error) {
		assert.Len(t, articles, 2)
		assert.Len(t, feedbacks, 1)

		return []db.Classification{
			{
				GUID:        "guid1",
				Score:       8.5,
				Explanation: "Relevant article",
				Topics:      []string{"tech", "programming"},
			},
			{
				GUID:        "guid2",
				Score:       3.0,
				Explanation: "Not relevant",
				Topics:      []string{"sports"},
			},
		}, nil
	}

	mockDB.UpdateClassificationsFunc = func(ctx context.Context, classifications []db.Classification, itemsByGUID map[string]int64) error {
		assert.Len(t, classifications, 2)
		assert.Len(t, itemsByGUID, 2)
		assert.Equal(t, int64(1), itemsByGUID["guid1"])
		assert.Equal(t, int64(2), itemsByGUID["guid2"])
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, Config{})
	s.classifyPendingItems(ctx)

	assert.Len(t, mockClassifier.ClassifyArticlesCalls(), 1)
	assert.Len(t, mockDB.UpdateClassificationsCalls(), 1)
}

func TestScheduler_ClassifyNow(t *testing.T) {
	ctx := context.Background()
	mockDB := &mocks.DatabaseMock{}
	mockParser := &mocks.ParserMock{}
	mockExtractor := &mocks.ExtractorMock{}
	mockClassifier := &mocks.ClassifierMock{}

	// set up expectations
	mockDB.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error) {
		return []db.FeedbackExample{}, nil
	}

	mockDB.GetUnclassifiedItemsFunc = func(ctx context.Context, limit int) ([]db.Item, error) {
		return []db.Item{
			{
				ID:    1,
				GUID:  "guid1",
				Title: "Test Article",
			},
		}, nil
	}

	mockClassifier.ClassifyArticlesFunc = func(ctx context.Context, items []db.Item, feedbacks []db.FeedbackExample) ([]db.Classification, error) {
		return []db.Classification{
			{
				GUID:        "guid1",
				Score:       7.5,
				Explanation: "Relevant",
				Topics:      []string{"tech"},
			},
		}, nil
	}

	mockDB.UpdateClassificationsFunc = func(ctx context.Context, classifications []db.Classification, itemsByGUID map[string]int64) error {
		return nil
	}

	s := NewScheduler(mockDB, mockParser, mockExtractor, mockClassifier, Config{})

	// test ClassifyNow
	err := s.ClassifyNow(ctx)
	require.NoError(t, err)

	// verify classification was triggered
	assert.Len(t, mockDB.GetUnclassifiedItemsCalls(), 1)
	assert.Len(t, mockClassifier.ClassifyArticlesCalls(), 1)
	assert.Len(t, mockDB.UpdateClassificationsCalls(), 1)
}
