package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/llm"
	"github.com/umputun/newscope/pkg/scheduler/mocks"
)

func TestFeedProcessor_UpdateFeedNow(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	retryFunc := func(ctx context.Context, op func() error) error {
		return op()
	}

	fp := NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
		MaxWorkers:            1,
		RetryFunc:             retryFunc,
	})

	testFeed := &domain.Feed{
		ID:            1,
		URL:           "https://example.com/feed.xml",
		Title:         "Test Feed",
		FetchInterval: 3600,
	}

	testParsedFeed := &domain.ParsedFeed{
		Title: "Test Feed",
		Items: []domain.ParsedItem{
			{
				GUID:        "item1",
				Title:       "Test Item",
				Link:        "https://example.com/item1",
				Description: "Test description",
				Published:   time.Now(),
			},
		},
	}

	// setup expectations
	feedManager.GetFeedFunc = func(ctx context.Context, id int64) (*domain.Feed, error) {
		assert.Equal(t, int64(1), id)
		return testFeed, nil
	}

	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		assert.Equal(t, testFeed.URL, url)
		return testParsedFeed, nil
	}

	itemManager.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		assert.Equal(t, testFeed.ID, feedID)
		assert.Equal(t, "item1", guid)
		return false, nil
	}

	itemManager.ItemExistsByTitleOrURLFunc = func(ctx context.Context, title, url string) (bool, error) {
		assert.Equal(t, "Test Item", title)
		assert.Equal(t, "https://example.com/item1", url)
		return false, nil
	}

	itemManager.CreateItemFunc = func(ctx context.Context, item *domain.Item) error {
		assert.Equal(t, testFeed.ID, item.FeedID)
		assert.Equal(t, "item1", item.GUID)
		assert.Equal(t, "Test Item", item.Title)
		item.ID = 123 // simulate database assigning ID
		return nil
	}

	feedManager.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		assert.Equal(t, testFeed.ID, feedID)
		// nextFetch should be in the future, but allow some timing slack
		assert.True(t, nextFetch.After(time.Now().Add(-time.Second)))
		return nil
	}

	// setup mocks for background processing
	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{
			Content:     "extracted content",
			RichContent: "<p>rich content</p>",
		}, nil
	}

	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		return []domain.FeedbackExample{}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech"}, nil
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		return "", nil
	}

	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		return []domain.Classification{{
			GUID:        req.Articles[0].GUID,
			Score:       7.5,
			Explanation: "test classification",
			Topics:      []string{"tech"},
			Summary:     "test summary",
		}}, nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		return nil
	}

	// execute
	err := fp.UpdateFeedNow(context.Background(), 1)

	// wait for background processing to complete
	time.Sleep(100 * time.Millisecond)

	// verify
	require.NoError(t, err)
	assert.Len(t, feedManager.GetFeedCalls(), 1)
	assert.Len(t, parser.ParseCalls(), 1)
	assert.Len(t, itemManager.ItemExistsCalls(), 1)
	assert.Len(t, itemManager.ItemExistsByTitleOrURLCalls(), 1)
	assert.Len(t, itemManager.CreateItemCalls(), 1)
	assert.Len(t, feedManager.UpdateFeedFetchedCalls(), 1)
}

func TestFeedProcessor_ExtractContentNow(t *testing.T) {
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	retryFunc := func(ctx context.Context, op func() error) error {
		return op()
	}

	fp := NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           &mocks.FeedManagerMock{},
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                &mocks.ParserMock{},
		Extractor:             extractor,
		Classifier:            classifier,
		MaxWorkers:            1,
		RetryFunc:             retryFunc,
	})

	testItem := &domain.Item{
		ID:    1,
		GUID:  "test-guid",
		Link:  "https://example.com/item1",
		Title: "Test Item",
	}

	extractResult := &content.ExtractResult{
		Content:     "Extracted content",
		RichContent: "<p>Rich content</p>",
	}

	classification := &domain.Classification{
		GUID:        testItem.GUID,
		Score:       8.5,
		Explanation: "Test classification",
		Topics:      []string{"tech"},
		Summary:     "Test summary",
	}

	// setup expectations
	itemManager.GetItemFunc = func(ctx context.Context, id int64) (*domain.Item, error) {
		assert.Equal(t, int64(1), id)
		return testItem, nil
	}

	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		assert.Equal(t, testItem.Link, url)
		return extractResult, nil
	}

	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		assert.Empty(t, feedbackType)
		assert.Equal(t, 50, limit)
		return []domain.FeedbackExample{}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech", "news"}, nil
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		switch key {
		case "preference_summary":
			return "", nil
		case domain.SettingPreferredTopics:
			return "", nil
		case domain.SettingAvoidedTopics:
			return "", nil
		default:
			t.Fatalf("unexpected setting key: %s", key)
			return "", nil
		}
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		assert.Equal(t, testItem.ID, itemID)
		assert.Equal(t, extractResult.Content, extraction.PlainText)
		assert.Equal(t, extractResult.RichContent, extraction.RichHTML)
		assert.False(t, extraction.ExtractedAt.IsZero())
		return nil
	}

	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		assert.Len(t, req.Articles, 1)
		assert.Equal(t, extractResult.Content, req.Articles[0].Content)
		assert.Empty(t, req.Feedbacks)
		assert.Equal(t, []string{"tech", "news"}, req.CanonicalTopics)
		assert.Empty(t, req.PreferenceSummary)
		return []domain.Classification{*classification}, nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, class *domain.Classification) error {
		assert.Equal(t, testItem.ID, itemID)
		assert.Nil(t, extraction) // extraction is nil in batch processing since it's saved separately
		assert.Equal(t, classification.GUID, class.GUID)
		assert.InEpsilon(t, classification.Score, class.Score, 0.001)
		assert.Equal(t, classification.Explanation, class.Explanation)
		assert.Equal(t, classification.Topics, class.Topics)
		assert.Equal(t, classification.Summary, class.Summary)
		assert.False(t, class.ClassifiedAt.IsZero())
		return nil
	}

	// execute
	err := fp.ExtractContentNow(context.Background(), 1)

	// verify
	require.NoError(t, err)
	assert.Len(t, itemManager.GetItemCalls(), 1)
	assert.Len(t, extractor.ExtractCalls(), 1)
	assert.Len(t, classificationManager.GetRecentFeedbackCalls(), 1)
	assert.Len(t, classificationManager.GetTopicsCalls(), 1)
	assert.Len(t, settingManager.GetSettingCalls(), 3)
	assert.Len(t, classifier.ClassifyItemsCalls(), 1)
	assert.Len(t, itemManager.UpdateItemProcessedCalls(), 1)
}

func TestFeedProcessor_UpdateFeed_ParseError(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	parser := &mocks.ParserMock{}

	retryFunc := func(ctx context.Context, op func() error) error {
		return op()
	}

	fp := NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           feedManager,
		ItemManager:           &mocks.ItemManagerMock{},
		ClassificationManager: &mocks.ClassificationManagerMock{},
		SettingManager:        &mocks.SettingManagerMock{},
		Parser:                parser,
		Extractor:             &mocks.ExtractorMock{},
		Classifier:            &mocks.ClassifierMock{},
		MaxWorkers:            1,
		RetryFunc:             retryFunc,
	})

	testFeed := &domain.Feed{
		ID:  1,
		URL: "https://example.com/feed.xml",
	}

	// setup feed manager
	feedManager.GetFeedFunc = func(ctx context.Context, id int64) (*domain.Feed, error) {
		return testFeed, nil
	}

	// setup parser to fail
	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		return nil, assert.AnError
	}

	// setup feed manager to expect error update
	feedManager.UpdateFeedErrorFunc = func(ctx context.Context, feedID int64, errMsg string) error {
		assert.Equal(t, testFeed.ID, feedID)
		assert.NotEmpty(t, errMsg)
		return nil
	}

	// execute
	err := fp.UpdateFeedNow(context.Background(), 1)

	// verify - should not return error but should call UpdateFeedError
	require.NoError(t, err)
	assert.Len(t, parser.ParseCalls(), 1)
	assert.Len(t, feedManager.UpdateFeedErrorCalls(), 1)
}

func TestFeedProcessor_ProcessItem_ExtractionError(t *testing.T) {
	itemManager := &mocks.ItemManagerMock{}
	extractor := &mocks.ExtractorMock{}

	retryFunc := func(ctx context.Context, op func() error) error {
		return op()
	}

	fp := NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           &mocks.FeedManagerMock{},
		ItemManager:           itemManager,
		ClassificationManager: &mocks.ClassificationManagerMock{},
		SettingManager:        &mocks.SettingManagerMock{},
		Parser:                &mocks.ParserMock{},
		Extractor:             extractor,
		Classifier:            &mocks.ClassifierMock{},
		MaxWorkers:            1,
		RetryFunc:             retryFunc,
	})

	testItem := &domain.Item{
		ID:   1,
		GUID: "test-guid",
		Link: "https://example.com/item1",
	}

	// setup extraction to fail
	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return nil, assert.AnError
	}

	// setup item manager to expect extraction error update
	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		assert.Equal(t, testItem.ID, itemID)
		assert.NotEmpty(t, extraction.Error)
		assert.False(t, extraction.ExtractedAt.IsZero())
		return nil
	}

	// setup GetItem
	itemManager.GetItemFunc = func(ctx context.Context, id int64) (*domain.Item, error) {
		return testItem, nil
	}

	err := fp.ExtractContentNow(context.Background(), 1)

	// verify - should not return error but should call UpdateItemExtraction
	require.NoError(t, err)
	assert.Len(t, extractor.ExtractCalls(), 1)
	assert.Len(t, itemManager.UpdateItemExtractionCalls(), 1)
}

func TestFeedProcessor_UpdateFeed_DuplicateItems(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	retryFunc := func(ctx context.Context, op func() error) error {
		return op()
	}

	fp := NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
		MaxWorkers:            1,
		RetryFunc:             retryFunc,
	})

	testFeed := &domain.Feed{
		ID:            1,
		URL:           "https://example.com/feed.xml",
		FetchInterval: 3600,
	}

	testParsedFeed := &domain.ParsedFeed{
		Items: []domain.ParsedItem{
			{
				GUID:  "existing-item",
				Title: "Existing Item",
				Link:  "https://example.com/existing",
			},
			{
				GUID:  "new-item",
				Title: "New Item",
				Link:  "https://example.com/new",
			},
		},
	}

	// setup feed and parser
	feedManager.GetFeedFunc = func(ctx context.Context, id int64) (*domain.Feed, error) {
		return testFeed, nil
	}

	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		return testParsedFeed, nil
	}

	// setup item existence checks
	itemManager.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		if guid == "existing-item" {
			return true, nil // already exists
		}
		return false, nil // new item
	}

	itemManager.ItemExistsByTitleOrURLFunc = func(ctx context.Context, title, url string) (bool, error) {
		return false, nil // no duplicates by title/url
	}

	// setup item creation
	itemManager.CreateItemFunc = func(ctx context.Context, item *domain.Item) error {
		assert.Equal(t, "new-item", item.GUID) // should only create new item
		return nil
	}

	// setup feed update
	feedManager.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	// setup mocks for background processing
	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{
			Content:     "extracted content",
			RichContent: "<p>rich content</p>",
		}, nil
	}

	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		return []domain.FeedbackExample{}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech"}, nil
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		return "", nil
	}

	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		return []domain.Classification{{
			GUID:        req.Articles[0].GUID,
			Score:       7.5,
			Explanation: "test classification",
			Topics:      []string{"tech"},
			Summary:     "test summary",
		}}, nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		return nil
	}

	// execute
	err := fp.UpdateFeedNow(context.Background(), 1)

	// wait for background processing
	time.Sleep(100 * time.Millisecond)

	// verify
	require.NoError(t, err)
	assert.Len(t, itemManager.ItemExistsCalls(), 2)             // checked both items
	assert.Len(t, itemManager.ItemExistsByTitleOrURLCalls(), 1) // only for new item
	assert.Len(t, itemManager.CreateItemCalls(), 1)             // only created new item
	assert.Len(t, feedManager.UpdateFeedFetchedCalls(), 1)      // updated feed timestamp
}

func TestFeedProcessor_UpdateFeed_ItemCreationError(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	// retry function that retries up to 5 times
	retryFunc := func(ctx context.Context, op func() error) error {
		for i := 0; i < 5; i++ {
			if err := op(); err != nil {
				if i < 4 {
					continue
				}
				return err
			}
			return nil
		}
		return nil
	}

	fp := NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
		MaxWorkers:            1,
		RetryFunc:             retryFunc,
	})

	testFeed := &domain.Feed{
		ID:            1,
		URL:           "https://example.com/feed.xml",
		FetchInterval: 3600,
	}

	testParsedFeed := &domain.ParsedFeed{
		Items: []domain.ParsedItem{
			{GUID: "item1", Title: "Item 1", Link: "https://example.com/item1"},
		},
	}

	// setup feed and parser
	feedManager.GetFeedFunc = func(ctx context.Context, id int64) (*domain.Feed, error) {
		return testFeed, nil
	}

	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		return testParsedFeed, nil
	}

	// setup item checks to pass
	itemManager.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		return false, nil
	}

	itemManager.ItemExistsByTitleOrURLFunc = func(ctx context.Context, title, url string) (bool, error) {
		return false, nil
	}

	// setup item creation to fail
	itemManager.CreateItemFunc = func(ctx context.Context, item *domain.Item) error {
		return assert.AnError
	}

	// setup feed update to still succeed
	feedManager.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	// setup mocks for background processing (though items won't be created due to error)
	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{}, nil
	}

	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		return []domain.FeedbackExample{}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{}, nil
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		return "", nil
	}

	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		return []domain.Classification{}, nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		return nil
	}

	// execute
	err := fp.UpdateFeedNow(context.Background(), 1)

	// verify - should not return error but should still update feed
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(itemManager.CreateItemCalls()), 5) // at least 5 attempts due to retry logic
	assert.Len(t, feedManager.UpdateFeedFetchedCalls(), 1)          // should still update feed timestamp
}

func TestFeedProcessor_UpdateFeed_ItemCreationWithLockError(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	// setup retry function that actually retries on lock errors
	retryFunc := func(ctx context.Context, op func() error) error {
		for i := 0; i < 5; i++ {
			if err := op(); err != nil {
				if i < 4 && isLockError(err) {
					time.Sleep(10 * time.Millisecond) // simulate backoff
					continue
				}
				return err
			}
			return nil
		}
		return fmt.Errorf("max retries exceeded")
	}

	fp := NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
		MaxWorkers:            1,
		RetryFunc:             retryFunc,
	})

	testFeed := &domain.Feed{
		ID:            1,
		URL:           "https://example.com/feed.xml",
		FetchInterval: 3600,
	}

	testParsedFeed := &domain.ParsedFeed{
		Items: []domain.ParsedItem{
			{GUID: "item1", Title: "Item 1", Link: "https://example.com/item1"},
		},
	}

	// setup feed and parser
	feedManager.GetFeedFunc = func(ctx context.Context, id int64) (*domain.Feed, error) {
		return testFeed, nil
	}

	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		return testParsedFeed, nil
	}

	// setup item checks to pass
	itemManager.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		return false, nil
	}

	itemManager.ItemExistsByTitleOrURLFunc = func(ctx context.Context, title, url string) (bool, error) {
		return false, nil
	}

	// setup item creation to fail with lock error initially, then succeed
	itemCreateCount := 0
	itemManager.CreateItemFunc = func(ctx context.Context, item *domain.Item) error {
		itemCreateCount++
		if itemCreateCount < 5 {
			return fmt.Errorf("SQLITE_BUSY: database is locked")
		}
		return nil
	}

	// setup feed update to succeed
	feedManager.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	// setup mocks for background processing
	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{
			Content:     "extracted content",
			RichContent: "<p>rich content</p>",
		}, nil
	}

	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		return []domain.FeedbackExample{}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech"}, nil
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		return "", nil
	}

	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		return []domain.Classification{
			{GUID: "item1", Score: 8, Explanation: "Good", Topics: []string{"tech"}},
		}, nil
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		return nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	// execute
	err := fp.UpdateFeedNow(context.Background(), 1)

	// wait for background processing
	time.Sleep(100 * time.Millisecond)

	// verify - should succeed after retries
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(itemManager.CreateItemCalls()), 5) // should be called at least 5 times
	assert.Len(t, feedManager.UpdateFeedFetchedCalls(), 1)
}

func TestFeedProcessor_ProcessItem_NonHTMLContent(t *testing.T) {
	itemManager := &mocks.ItemManagerMock{}
	extractor := &mocks.ExtractorMock{}

	retryFunc := func(ctx context.Context, op func() error) error {
		return op()
	}

	fp := NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           &mocks.FeedManagerMock{},
		ItemManager:           itemManager,
		ClassificationManager: &mocks.ClassificationManagerMock{},
		SettingManager:        &mocks.SettingManagerMock{},
		Parser:                &mocks.ParserMock{},
		Extractor:             extractor,
		Classifier:            &mocks.ClassifierMock{},
		MaxWorkers:            1,
		RetryFunc:             retryFunc,
	})

	testItem := &domain.Item{
		ID:    1,
		GUID:  "test-guid",
		Link:  "https://example.com/document.pdf",
		Title: "PDF Document",
	}

	// setup extraction to fail with unsupported content type error
	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return nil, fmt.Errorf("unsupported content type: application/pdf")
	}

	// setup item manager to expect extraction error update with specific binary content message
	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		assert.Equal(t, testItem.ID, itemID)
		assert.Equal(t, "Binary content (PDF, image, or other non-HTML format)", extraction.Error)
		assert.False(t, extraction.ExtractedAt.IsZero())
		return nil
	}

	// setup GetItem
	itemManager.GetItemFunc = func(ctx context.Context, id int64) (*domain.Item, error) {
		return testItem, nil
	}

	err := fp.ExtractContentNow(context.Background(), 1)

	// verify - should not return error but should call UpdateItemExtraction with binary content message
	require.NoError(t, err)
	assert.Len(t, extractor.ExtractCalls(), 1)
	assert.Len(t, itemManager.UpdateItemExtractionCalls(), 1)
}

func TestFeedProcessor_ExtractionDisabled(t *testing.T) {
	retryFunc := func(ctx context.Context, op func() error) error {
		return op()
	}

	settingManager := &mocks.SettingManagerMock{
		GetSettingFunc: func(ctx context.Context, key string) (string, error) { return "", nil },
	}

	newProcessor := func(itemManager *mocks.ItemManagerMock, classificationManager *mocks.ClassificationManagerMock,
		classifier *mocks.ClassifierMock) *FeedProcessor {
		return NewFeedProcessor(FeedProcessorConfig{
			FeedManager:           &mocks.FeedManagerMock{},
			ItemManager:           itemManager,
			ClassificationManager: classificationManager,
			SettingManager:        settingManager,
			Parser:                &mocks.ParserMock{},
			Extractor:             nil, // extraction disabled
			Classifier:            classifier,
			MaxWorkers:            1,
			RetryFunc:             retryFunc,
		})
	}

	t.Run("classifies item with feed content", func(t *testing.T) {
		itemManager := &mocks.ItemManagerMock{}
		classificationManager := &mocks.ClassificationManagerMock{}
		classifier := &mocks.ClassifierMock{}

		classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
			return []domain.FeedbackExample{}, nil
		}
		classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
			return []string{}, nil
		}
		classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
			require.Len(t, req.Articles, 1)
			assert.Equal(t, "content from the feed", req.Articles[0].Content)
			return []domain.Classification{{GUID: "test-guid", Score: 7.5, Summary: "summary"}}, nil
		}
		itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64,
			extraction *domain.ExtractedContent, classification *domain.Classification) error {
			assert.Equal(t, int64(1), itemID)
			assert.Nil(t, extraction)
			return nil
		}

		fp := newProcessor(itemManager, classificationManager, classifier)
		item := &domain.Item{ID: 1, GUID: "test-guid", Link: "https://example.com/item1", Content: "content from the feed"}
		fp.ProcessItem(context.Background(), item)

		assert.Len(t, classifier.ClassifyItemsCalls(), 1, "item should be classified from feed content")
		assert.Empty(t, itemManager.UpdateItemExtractionCalls(), "no extraction result should be stored")
	})

	t.Run("classifies description-only item", func(t *testing.T) {
		itemManager := &mocks.ItemManagerMock{}
		classificationManager := &mocks.ClassificationManagerMock{}
		classifier := &mocks.ClassifierMock{}

		classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
			return []domain.FeedbackExample{}, nil
		}
		classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
			return []string{}, nil
		}
		classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
			require.Len(t, req.Articles, 1)
			assert.Equal(t, "description from the feed", req.Articles[0].Description)
			return []domain.Classification{{GUID: "test-guid", Score: 6}}, nil
		}
		itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64,
			extraction *domain.ExtractedContent, classification *domain.Classification) error {
			return nil
		}

		fp := newProcessor(itemManager, classificationManager, classifier)
		item := &domain.Item{ID: 1, GUID: "test-guid", Link: "https://example.com/item1", Description: "description from the feed"}
		fp.ProcessItem(context.Background(), item)

		assert.Len(t, classifier.ClassifyItemsCalls(), 1, "description is enough to classify without extraction")
	})

	t.Run("skips item without content or description", func(t *testing.T) {
		itemManager := &mocks.ItemManagerMock{}
		classifier := &mocks.ClassifierMock{}

		fp := newProcessor(itemManager, &mocks.ClassificationManagerMock{}, classifier)
		item := &domain.Item{ID: 1, GUID: "test-guid", Link: "https://example.com/item1"}
		fp.ProcessItem(context.Background(), item)

		assert.Empty(t, classifier.ClassifyItemsCalls())
	})

	t.Run("extract content now reports extraction disabled", func(t *testing.T) {
		itemManager := &mocks.ItemManagerMock{}
		fp := newProcessor(itemManager, &mocks.ClassificationManagerMock{}, &mocks.ClassifierMock{})

		err := fp.ExtractContentNow(context.Background(), 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
		assert.Empty(t, itemManager.GetItemCalls(), "item should not be loaded at all")
	})

	t.Run("processing worker classifies without extraction", func(t *testing.T) {
		itemManager := &mocks.ItemManagerMock{}
		classificationManager := &mocks.ClassificationManagerMock{}
		classifier := &mocks.ClassifierMock{}

		classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
			return []domain.FeedbackExample{}, nil
		}
		classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
			return []string{}, nil
		}
		classified := make(chan struct{})
		classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
			defer close(classified)
			return []domain.Classification{{GUID: "test-guid", Score: 5}}, nil
		}
		itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64,
			extraction *domain.ExtractedContent, classification *domain.Classification) error {
			return nil
		}

		fp := newProcessor(itemManager, classificationManager, classifier)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		items := make(chan domain.Item, 1)
		items <- domain.Item{ID: 1, GUID: "test-guid", Link: "https://example.com/item1", Content: "content from the feed"}
		close(items)

		done := make(chan struct{})
		go func() {
			defer close(done)
			fp.ProcessingWorker(ctx, items)
		}()

		select {
		case <-classified:
		case <-ctx.Done():
			t.Fatal("classification never happened")
		}
		<-done
	})
}
