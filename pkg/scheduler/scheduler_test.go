package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/llm"
	"github.com/umputun/newscope/pkg/scheduler/mocks"
)

func TestNewScheduler(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	cfg := Config{
		UpdateInterval: 5 * time.Minute,
		MaxWorkers:     3,
	}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, cfg)

	assert.NotNil(t, scheduler)
	assert.Equal(t, 5*time.Minute, scheduler.updateInterval)
	assert.Equal(t, 3, scheduler.maxWorkers)
}

func TestNewScheduler_DefaultConfig(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	cfg := Config{} // empty config

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, cfg)

	assert.NotNil(t, scheduler)
	assert.Equal(t, 30*time.Minute, scheduler.updateInterval)  // default
	assert.Equal(t, 5, scheduler.maxWorkers)                   // default
	assert.Equal(t, 25, scheduler.preferenceSummaryThreshold)  // default
	assert.Equal(t, 168*time.Hour, scheduler.cleanupAge)       // default 1 week
	assert.InEpsilon(t, 5.0, scheduler.cleanupMinScore, 0.001) // default
	assert.Equal(t, 24*time.Hour, scheduler.cleanupInterval)   // default
}

func TestScheduler_UpdateFeedNow(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	cfg := Config{
		UpdateInterval: time.Hour, // long interval to prevent auto-updates
		MaxWorkers:     1,         // single worker for processing
	}
	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, cfg)

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

	// setup expectations using generated mocks
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
		assert.True(t, nextFetch.After(time.Now()))
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
	err := scheduler.UpdateFeedNow(context.Background(), 1)

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

func TestScheduler_ExtractContentNow(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, Config{})

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

	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		assert.Len(t, req.Articles, 1)
		assert.Equal(t, extractResult.Content, req.Articles[0].Content) // content should be set
		assert.Empty(t, req.Feedbacks)
		assert.Equal(t, []string{"tech", "news"}, req.CanonicalTopics)
		assert.Empty(t, req.PreferenceSummary)
		return []domain.Classification{*classification}, nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, class *domain.Classification) error {
		assert.Equal(t, testItem.ID, itemID)
		assert.Equal(t, extractResult.Content, extraction.PlainText)
		assert.Equal(t, extractResult.RichContent, extraction.RichHTML)
		assert.Equal(t, classification.GUID, class.GUID)
		assert.InEpsilon(t, classification.Score, class.Score, 0.001)
		assert.Equal(t, classification.Explanation, class.Explanation)
		assert.Equal(t, classification.Topics, class.Topics)
		assert.Equal(t, classification.Summary, class.Summary)
		assert.False(t, class.ClassifiedAt.IsZero()) // ensure ClassifiedAt is set
		return nil
	}

	// execute
	err := scheduler.ExtractContentNow(context.Background(), 1)

	// verify
	require.NoError(t, err)
	assert.Len(t, itemManager.GetItemCalls(), 1)
	assert.Len(t, extractor.ExtractCalls(), 1)
	assert.Len(t, classificationManager.GetRecentFeedbackCalls(), 1)
	assert.Len(t, classificationManager.GetTopicsCalls(), 1)
	assert.Len(t, settingManager.GetSettingCalls(), 3) // preference_summary, preferred_topics, avoided_topics
	assert.Len(t, classifier.ClassifyItemsCalls(), 1)
	assert.Len(t, itemManager.UpdateItemProcessedCalls(), 1)
}

func TestScheduler_StartStop(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	cfg := Config{
		UpdateInterval: 100 * time.Millisecond, // short interval for testing
		MaxWorkers:     1,
	}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, cfg)

	// setup minimal expectations for feed update
	feedManager.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
		assert.True(t, enabledOnly)
		return []domain.Feed{}, nil
	}

	// setup cleanup mock to prevent panic
	itemManager.DeleteOldItemsFunc = func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
		return 0, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	// start scheduler
	scheduler.Start(ctx)

	// let it run briefly
	time.Sleep(150 * time.Millisecond)

	// stop scheduler
	cancel()
	scheduler.Stop()

	// verify at least one call was made
	assert.GreaterOrEqual(t, len(feedManager.GetFeedsCalls()), 1)
}

func TestScheduler_ProcessItem_ExtractionError(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, Config{})

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

	// execute - processItem is private, so we use ExtractContentNow
	itemManager.GetItemFunc = func(ctx context.Context, id int64) (*domain.Item, error) {
		return testItem, nil
	}

	err := scheduler.ExtractContentNow(context.Background(), 1)

	// verify - should not return error but should call UpdateItemExtraction
	require.NoError(t, err)
	assert.Len(t, extractor.ExtractCalls(), 1)
	assert.Len(t, itemManager.UpdateItemExtractionCalls(), 1)
	// should not attempt classification after extraction error
	assert.Empty(t, classifier.ClassifyItemsCalls())
}

func TestScheduler_ProcessItem_ClassificationError(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, Config{})

	testItem := &domain.Item{
		ID:   1,
		GUID: "test-guid",
		Link: "https://example.com/item1",
	}

	// setup successful extraction
	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{
			Content:     "Extracted content",
			RichContent: "<p>Rich content</p>",
		}, nil
	}

	// setup classification dependencies
	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		return []domain.FeedbackExample{}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech"}, nil
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		return "", nil
	}

	// setup classification to fail
	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		return nil, assert.AnError
	}

	// execute - processItem is private, so we use ExtractContentNow
	itemManager.GetItemFunc = func(ctx context.Context, id int64) (*domain.Item, error) {
		return testItem, nil
	}

	err := scheduler.ExtractContentNow(context.Background(), 1)

	// verify - should not return error but should not call UpdateItemProcessed
	require.NoError(t, err)
	assert.Len(t, extractor.ExtractCalls(), 1)
	assert.Len(t, classifier.ClassifyItemsCalls(), 1)
	assert.Empty(t, itemManager.UpdateItemProcessedCalls()) // should not be called after classification error
}

func TestScheduler_ProcessItem_NoClassificationResults(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, Config{})

	testItem := &domain.Item{
		ID:   1,
		GUID: "test-guid",
		Link: "https://example.com/item1",
	}

	// setup successful extraction
	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{
			Content:     "Extracted content",
			RichContent: "<p>Rich content</p>",
		}, nil
	}

	// setup classification dependencies
	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		return []domain.FeedbackExample{}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech"}, nil
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		return "", nil
	}

	// setup classification to return empty results
	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		return []domain.Classification{}, nil // empty results
	}

	// execute - processItem is private, so we use ExtractContentNow
	itemManager.GetItemFunc = func(ctx context.Context, id int64) (*domain.Item, error) {
		return testItem, nil
	}

	err := scheduler.ExtractContentNow(context.Background(), 1)

	// verify - should not return error but should not call UpdateItemProcessed
	require.NoError(t, err)
	assert.Len(t, extractor.ExtractCalls(), 1)
	assert.Len(t, classifier.ClassifyItemsCalls(), 1)
	assert.Empty(t, itemManager.UpdateItemProcessedCalls()) // should not be called with empty results
}

func TestScheduler_UpdateFeed_ParseError(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, Config{})

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
	err := scheduler.UpdateFeedNow(context.Background(), 1)

	// verify - should not return error but should call UpdateFeedError
	require.NoError(t, err)
	assert.Len(t, parser.ParseCalls(), 1)
	assert.Len(t, feedManager.UpdateFeedErrorCalls(), 1)
	assert.Empty(t, itemManager.CreateItemCalls()) // should not create items after parse error
}

func TestScheduler_UpdateFeed_DuplicateItems(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, Config{})

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
	err := scheduler.UpdateFeedNow(context.Background(), 1)

	// verify
	require.NoError(t, err)
	assert.Len(t, itemManager.ItemExistsCalls(), 2)             // checked both items
	assert.Len(t, itemManager.ItemExistsByTitleOrURLCalls(), 1) // only for new item
	assert.Len(t, itemManager.CreateItemCalls(), 1)             // only created new item
	assert.Len(t, feedManager.UpdateFeedFetchedCalls(), 1)      // updated feed timestamp
}

func TestScheduler_UpdateAllFeeds_GetFeedsError(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	cfg := Config{
		UpdateInterval: 100 * time.Millisecond,
		MaxWorkers:     1,
	}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, cfg)

	// setup GetFeeds to fail
	feedManager.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
		return nil, assert.AnError
	}

	// setup mocks for background processing (in case there are residual items from other tests)
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
		return []domain.Classification{}, nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		return nil
	}

	// setup cleanup mock to prevent panic
	itemManager.DeleteOldItemsFunc = func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
		return 0, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start scheduler and let it run briefly
	scheduler.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()
	scheduler.Stop()

	// verify - should call GetFeeds but not attempt to process any feeds
	assert.GreaterOrEqual(t, len(feedManager.GetFeedsCalls()), 1)
	assert.Empty(t, parser.ParseCalls()) // should not parse if GetFeeds fails
}

func TestScheduler_UpdateAllFeeds_MultipleFeeds(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	cfg := Config{
		UpdateInterval: 100 * time.Millisecond,
		MaxWorkers:     2, // multiple workers
	}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, cfg)

	testFeeds := []domain.Feed{
		{ID: 1, URL: "https://example.com/feed1.xml", FetchInterval: 3600},
		{ID: 2, URL: "https://example.com/feed2.xml", FetchInterval: 3600},
	}

	// setup GetFeeds to return multiple feeds
	feedManager.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
		assert.True(t, enabledOnly)
		return testFeeds, nil
	}

	// setup parser to return empty feeds (no items)
	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		return &domain.ParsedFeed{Items: []domain.ParsedItem{}}, nil
	}

	// setup feed updates
	feedManager.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		assert.Contains(t, []int64{1, 2}, feedID)
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
		return []domain.Classification{}, nil // empty results for quick test
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		return nil
	}

	// setup cleanup mock to prevent panic
	itemManager.DeleteOldItemsFunc = func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
		return 0, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start scheduler and let it run briefly
	scheduler.Start(ctx)
	time.Sleep(200 * time.Millisecond) // longer time for multiple feeds
	cancel()
	scheduler.Stop()

	// verify - should process both feeds
	assert.GreaterOrEqual(t, len(feedManager.GetFeedsCalls()), 1)
	assert.GreaterOrEqual(t, len(parser.ParseCalls()), 2)                  // should parse both feeds
	assert.GreaterOrEqual(t, len(feedManager.UpdateFeedFetchedCalls()), 2) // should update both feeds
}

func TestScheduler_UpdateFeed_ItemCreationError(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, Config{})

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
		return []domain.Classification{}, nil // empty results for quick test
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		return nil
	}

	// execute
	err := scheduler.UpdateFeedNow(context.Background(), 1)

	// verify - should not return error but should still update feed
	require.NoError(t, err)
	assert.Len(t, itemManager.CreateItemCalls(), 5)        // 5 attempts due to retry logic (default)
	assert.Len(t, feedManager.UpdateFeedFetchedCalls(), 1) // should still update feed timestamp
}

func TestScheduler_UpdatePreferenceSummary(t *testing.T) {
	t.Run("generate initial summary", func(t *testing.T) {
		// setup mocks
		classificationManager := &mocks.ClassificationManagerMock{
			GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
				assert.Equal(t, 50, limit)
				return []domain.FeedbackExample{
					{Title: "AI article", Feedback: domain.FeedbackLike, Topics: []string{"ai"}},
					{Title: "Politics", Feedback: domain.FeedbackDislike, Topics: []string{"politics"}},
				}, nil
			},
			GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
				return 2, nil
			},
		}

		settingManager := &mocks.SettingManagerMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				if key == "preference_summary" {
					return "", nil // no existing summary
				}
				return "0", nil
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				return nil
			},
		}

		classifier := &mocks.ClassifierMock{
			GeneratePreferenceSummaryFunc: func(ctx context.Context, feedback []domain.FeedbackExample) (string, error) {
				assert.Len(t, feedback, 2)
				return "User likes AI, dislikes politics", nil
			},
		}

		scheduler := &Scheduler{
			classificationManager: classificationManager,
			settingManager:        settingManager,
			classifier:            classifier,
		}

		// execute
		err := scheduler.UpdatePreferenceSummary(context.Background())

		// verify
		require.NoError(t, err)
		assert.Len(t, classifier.GeneratePreferenceSummaryCalls(), 1)
		assert.Len(t, settingManager.SetSettingCalls(), 2) // summary and count

		// check both settings were updated
		var summarySet, countSet bool
		for _, call := range settingManager.SetSettingCalls() {
			if call.Key == "preference_summary" && call.Value == "User likes AI, dislikes politics" {
				summarySet = true
			}
			if call.Key == "last_summary_feedback_count" && call.Value == "2" {
				countSet = true
			}
		}
		assert.True(t, summarySet)
		assert.True(t, countSet)
	})

	t.Run("update existing summary with threshold", func(t *testing.T) {
		// setup mocks
		classificationManager := &mocks.ClassificationManagerMock{
			GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
				return []domain.FeedbackExample{
					{Title: "Go article", Feedback: domain.FeedbackLike, Topics: []string{"golang"}},
				}, nil
			},
			GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
				return 30, nil // 30 total feedbacks
			},
		}

		settingManager := &mocks.SettingManagerMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				switch key {
				case "preference_summary":
					return "Existing summary", nil
				case "last_summary_feedback_count":
					return "5", nil // 5 feedbacks last time
				}
				return "", nil
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				return nil
			},
		}

		classifier := &mocks.ClassifierMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context, currentSummary string, newFeedback []domain.FeedbackExample) (string, error) {
				assert.Equal(t, "Existing summary", currentSummary)
				return "Updated summary with Go preference", nil
			},
		}

		scheduler := &Scheduler{
			classificationManager: classificationManager,
			settingManager:        settingManager,
			classifier:            classifier,
		}

		// execute
		err := scheduler.UpdatePreferenceSummary(context.Background())

		// verify - 30-5=25 new feedbacks, exactly at threshold
		require.NoError(t, err)
		assert.Len(t, classifier.UpdatePreferenceSummaryCalls(), 1)
		assert.Len(t, settingManager.SetSettingCalls(), 2) // summary and count

		// check both settings were updated
		var summarySet, countSet bool
		for _, call := range settingManager.SetSettingCalls() {
			if call.Key == "preference_summary" && call.Value == "Updated summary with Go preference" {
				summarySet = true
			}
			if call.Key == "last_summary_feedback_count" && call.Value == "30" {
				countSet = true
			}
		}
		assert.True(t, summarySet)
		assert.True(t, countSet)
	})

	t.Run("skip update when below threshold", func(t *testing.T) {
		// setup mocks
		classificationManager := &mocks.ClassificationManagerMock{
			GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
				return []domain.FeedbackExample{}, nil
			},
			GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
				return 15, nil // only 15 total
			},
		}

		settingManager := &mocks.SettingManagerMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				switch key {
				case "preference_summary":
					return "Existing", nil
				case "last_summary_feedback_count":
					return "10", nil // 10 last time, so only 5 new
				}
				return "", nil
			},
		}

		classifier := &mocks.ClassifierMock{}

		scheduler := &Scheduler{
			classificationManager: classificationManager,
			settingManager:        settingManager,
			classifier:            classifier,
		}

		// execute
		err := scheduler.UpdatePreferenceSummary(context.Background())

		// verify - should skip update
		require.NoError(t, err)
		assert.Empty(t, classifier.UpdatePreferenceSummaryCalls())
		assert.Empty(t, settingManager.SetSettingCalls())
	})

	t.Run("context cancellation", func(t *testing.T) {
		// setup mocks with delay
		classificationManager := &mocks.ClassificationManagerMock{
			GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
				return []domain.FeedbackExample{{Title: "test"}}, nil
			},
			GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
				return 50, nil
			},
		}

		settingManager := &mocks.SettingManagerMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				if key == "preference_summary" {
					return "existing", nil
				}
				return "0", nil
			},
		}

		classifier := &mocks.ClassifierMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context, currentSummary string, newFeedback []domain.FeedbackExample) (string, error) {
				// check if context is done
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				default:
				}
				return "updated", nil
			},
		}

		scheduler := &Scheduler{
			classificationManager: classificationManager,
			settingManager:        settingManager,
			classifier:            classifier,
		}

		// create canceled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// execute
		err := scheduler.UpdatePreferenceSummary(ctx)

		// verify - should return context error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})
}

func TestScheduler_UpdateFeed_EmptyTitle(t *testing.T) {
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}
	scheduler := NewScheduler(deps, Config{})

	testFeed := &domain.Feed{
		ID:            1,
		URL:           "https://example.com/feed.xml",
		Title:         "", // empty title - should use URL in logs
		FetchInterval: 3600,
	}

	testParsedFeed := &domain.ParsedFeed{
		Items: []domain.ParsedItem{},
	}

	// setup feed and parser
	feedManager.GetFeedFunc = func(ctx context.Context, id int64) (*domain.Feed, error) {
		return testFeed, nil
	}

	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		return testParsedFeed, nil
	}

	// setup feed update
	feedManager.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	// execute
	err := scheduler.UpdateFeedNow(context.Background(), 1)

	// verify - should handle empty title gracefully
	require.NoError(t, err)
	assert.Len(t, parser.ParseCalls(), 1)
	assert.Len(t, feedManager.UpdateFeedFetchedCalls(), 1)
}

func TestScheduler_TriggerPreferenceUpdate(t *testing.T) {
	// setup mocks
	classificationManager := &mocks.ClassificationManagerMock{
		GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
			return []domain.FeedbackExample{
				{Title: "Test", Feedback: domain.FeedbackLike},
			}, nil
		},
		GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
			return 30, nil
		},
	}

	settingManager := &mocks.SettingManagerMock{
		GetSettingFunc: func(ctx context.Context, key string) (string, error) {
			if key == "last_summary_feedback_count" {
				return "0", nil
			}
			return "", nil
		},
		SetSettingFunc: func(ctx context.Context, key, value string) error {
			return nil
		},
	}

	classifier := &mocks.ClassifierMock{
		GeneratePreferenceSummaryFunc: func(ctx context.Context, feedback []domain.FeedbackExample) (string, error) {
			return "test summary", nil
		},
	}

	scheduler := &Scheduler{
		classificationManager:      classificationManager,
		settingManager:             settingManager,
		classifier:                 classifier,
		preferenceSummaryThreshold: 25,
		preferenceUpdateCh:         make(chan struct{}, 1),
	}

	// trigger multiple updates - only one should be queued
	scheduler.TriggerPreferenceUpdate()
	scheduler.TriggerPreferenceUpdate()
	scheduler.TriggerPreferenceUpdate()

	// verify channel has only one item
	assert.Len(t, scheduler.preferenceUpdateCh, 1)
}

func TestScheduler_PreferenceUpdateWorker_Debounce(t *testing.T) {
	// setup mocks
	updateCount := 0
	classificationManager := &mocks.ClassificationManagerMock{
		GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
			return []domain.FeedbackExample{{Title: "Test"}}, nil
		},
		GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
			return 30, nil
		},
	}

	settingManager := &mocks.SettingManagerMock{
		GetSettingFunc: func(ctx context.Context, key string) (string, error) {
			if key == "last_summary_feedback_count" {
				return "0", nil
			}
			return "", nil
		},
		SetSettingFunc: func(ctx context.Context, key, value string) error {
			if key == "preference_summary" {
				updateCount++
			}
			return nil
		},
	}

	classifier := &mocks.ClassifierMock{
		GeneratePreferenceSummaryFunc: func(ctx context.Context, feedback []domain.FeedbackExample) (string, error) {
			return "test summary", nil
		},
	}

	scheduler := &Scheduler{
		classificationManager:      classificationManager,
		settingManager:             settingManager,
		classifier:                 classifier,
		preferenceSummaryThreshold: 25,
		preferenceUpdateCh:         make(chan struct{}, 1),
		wg:                         sync.WaitGroup{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.wg.Add(1)

	// start worker with shorter debounce for testing
	go func() {
		defer scheduler.wg.Done()
		const debounceDelay = 100 * time.Millisecond // short delay for testing
		debounceTimer := time.NewTimer(0)
		if !debounceTimer.Stop() {
			<-debounceTimer.C
		}

		for {
			select {
			case <-ctx.Done():
				debounceTimer.Stop()
				return
			case <-scheduler.preferenceUpdateCh:
				debounceTimer.Stop()
				debounceTimer.Reset(debounceDelay)
			case <-debounceTimer.C:
				if err := scheduler.UpdatePreferenceSummary(ctx); err != nil {
					t.Logf("update error: %v", err)
				}
			}
		}
	}()

	// send multiple triggers rapidly
	scheduler.TriggerPreferenceUpdate()
	time.Sleep(50 * time.Millisecond)
	scheduler.TriggerPreferenceUpdate()
	time.Sleep(50 * time.Millisecond)
	scheduler.TriggerPreferenceUpdate()

	// wait for debounce to expire
	time.Sleep(150 * time.Millisecond)

	// stop worker
	cancel()
	scheduler.wg.Wait()

	// should have only one update despite multiple triggers
	assert.Equal(t, 1, updateCount, "should have exactly one preference update")
}

func TestScheduler_ConfigurableThreshold(t *testing.T) {
	cfg := Config{
		UpdateInterval:             time.Hour,
		MaxWorkers:                 1,
		PreferenceSummaryThreshold: 10, // custom threshold
	}

	deps := Params{
		FeedManager:           &mocks.FeedManagerMock{},
		ItemManager:           &mocks.ItemManagerMock{},
		ClassificationManager: &mocks.ClassificationManagerMock{},
		SettingManager:        &mocks.SettingManagerMock{},
		Parser:                &mocks.ParserMock{},
		Extractor:             &mocks.ExtractorMock{},
		Classifier:            &mocks.ClassifierMock{},
	}

	scheduler := NewScheduler(deps, cfg)
	assert.Equal(t, 10, scheduler.preferenceSummaryThreshold)
}

func TestScheduler_DefaultThreshold(t *testing.T) {
	cfg := Config{} // no threshold specified

	deps := Params{
		FeedManager:           &mocks.FeedManagerMock{},
		ItemManager:           &mocks.ItemManagerMock{},
		ClassificationManager: &mocks.ClassificationManagerMock{},
		SettingManager:        &mocks.SettingManagerMock{},
		Parser:                &mocks.ParserMock{},
		Extractor:             &mocks.ExtractorMock{},
		Classifier:            &mocks.ClassifierMock{},
	}

	scheduler := NewScheduler(deps, cfg)
	assert.Equal(t, 25, scheduler.preferenceSummaryThreshold) // default value
}

func TestScheduler_PerformCleanup(t *testing.T) {
	t.Run("successful cleanup", func(t *testing.T) {
		itemManager := &mocks.ItemManagerMock{
			DeleteOldItemsFunc: func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
				assert.Equal(t, 168*time.Hour, age)       // 1 week
				assert.InEpsilon(t, 5.0, minScore, 0.001) // default min score
				return 10, nil                            // deleted 10 items
			},
		}

		scheduler := &Scheduler{
			itemManager:     itemManager,
			cleanupAge:      168 * time.Hour,
			cleanupMinScore: 5.0,
		}

		// execute
		scheduler.performCleanup(context.Background())

		// verify
		assert.Len(t, itemManager.DeleteOldItemsCalls(), 1)
	})

	t.Run("cleanup error", func(t *testing.T) {
		itemManager := &mocks.ItemManagerMock{
			DeleteOldItemsFunc: func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
				return 0, assert.AnError
			},
		}

		scheduler := &Scheduler{
			itemManager:     itemManager,
			cleanupAge:      168 * time.Hour,
			cleanupMinScore: 5.0,
		}

		// execute - should not panic on error
		scheduler.performCleanup(context.Background())

		// verify
		assert.Len(t, itemManager.DeleteOldItemsCalls(), 1)
	})

	t.Run("no items to cleanup", func(t *testing.T) {
		itemManager := &mocks.ItemManagerMock{
			DeleteOldItemsFunc: func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
				return 0, nil // no items deleted
			},
		}

		scheduler := &Scheduler{
			itemManager:     itemManager,
			cleanupAge:      24 * time.Hour,
			cleanupMinScore: 8.0,
		}

		// execute
		scheduler.performCleanup(context.Background())

		// verify
		assert.Len(t, itemManager.DeleteOldItemsCalls(), 1)
	})
}

func TestScheduler_CleanupWorker(t *testing.T) {
	itemManager := &mocks.ItemManagerMock{
		DeleteOldItemsFunc: func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
			return 5, nil
		},
	}

	feedManager := &mocks.FeedManagerMock{
		GetFeedsFunc: func(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
			return []domain.Feed{}, nil
		},
	}

	extractor := &mocks.ExtractorMock{
		ExtractFunc: func(ctx context.Context, url string) (*content.ExtractResult, error) {
			return &content.ExtractResult{}, nil
		},
	}

	classificationManager := &mocks.ClassificationManagerMock{
		GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
			return []domain.FeedbackExample{}, nil
		},
		GetTopicsFunc: func(ctx context.Context) ([]string, error) {
			return []string{}, nil
		},
	}

	settingManager := &mocks.SettingManagerMock{
		GetSettingFunc: func(ctx context.Context, key string) (string, error) {
			return "", nil
		},
	}

	classifier := &mocks.ClassifierMock{
		ClassifyItemsFunc: func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
			return []domain.Classification{}, nil
		},
	}

	cfg := Config{
		UpdateInterval:  time.Hour,              // long interval to avoid feed updates
		CleanupInterval: 100 * time.Millisecond, // short interval for testing
		CleanupAge:      168 * time.Hour,
		CleanupMinScore: 5.0,
	}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                &mocks.ParserMock{},
		Extractor:             extractor,
		Classifier:            classifier,
	}

	scheduler := NewScheduler(deps, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start scheduler
	scheduler.Start(ctx)

	// wait for cleanup to run at least twice
	time.Sleep(250 * time.Millisecond)

	// stop scheduler
	cancel()
	scheduler.Stop()

	// verify cleanup was called multiple times
	require.GreaterOrEqual(t, len(itemManager.DeleteOldItemsCalls()), 2)
}

func TestScheduler_CleanupConfig(t *testing.T) {
	t.Run("custom config values", func(t *testing.T) {
		cfg := Config{
			CleanupAge:      72 * time.Hour, // 3 days
			CleanupMinScore: 7.5,
			CleanupInterval: 12 * time.Hour,
		}

		deps := Params{
			FeedManager:           &mocks.FeedManagerMock{},
			ItemManager:           &mocks.ItemManagerMock{},
			ClassificationManager: &mocks.ClassificationManagerMock{},
			SettingManager:        &mocks.SettingManagerMock{},
			Parser:                &mocks.ParserMock{},
			Extractor:             &mocks.ExtractorMock{},
			Classifier:            &mocks.ClassifierMock{},
		}

		scheduler := NewScheduler(deps, cfg)

		assert.Equal(t, 72*time.Hour, scheduler.cleanupAge)
		assert.InEpsilon(t, 7.5, scheduler.cleanupMinScore, 0.001)
		assert.Equal(t, 12*time.Hour, scheduler.cleanupInterval)
	})
}

func TestScheduler_UpdateFeed_ItemCreationWithLockError(t *testing.T) {
	// setup dependencies
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	cfg := Config{
		UpdateInterval:             100 * time.Millisecond,
		MaxWorkers:                 1,
		CleanupInterval:            24 * time.Hour,
		CleanupAge:                 7 * 24 * time.Hour,
		CleanupMinScore:            5.0,
		PreferenceSummaryThreshold: 25,
	}

	deps := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
	}

	scheduler := NewScheduler(deps, cfg)

	// setup test data
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
	callCount := 0
	itemManager.CreateItemFunc = func(ctx context.Context, item *domain.Item) error {
		callCount++
		if callCount < 5 { // fail first 4 attempts
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

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	// execute
	err := scheduler.UpdateFeedNow(context.Background(), 1)

	// verify - should succeed after retries
	require.NoError(t, err)
	assert.Equal(t, 5, callCount) // should be called 5 times (initial + 4 retries)
	assert.Len(t, feedManager.UpdateFeedFetchedCalls(), 1)
}

func TestIsLockError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "SQLITE_BUSY error",
			err:  fmt.Errorf("SQLITE_BUSY: database is busy"),
			want: true,
		},
		{
			name: "database is locked error",
			err:  fmt.Errorf("database is locked"),
			want: true,
		},
		{
			name: "database table is locked error",
			err:  fmt.Errorf("database table is locked"),
			want: true,
		},
		{
			name: "regular error",
			err:  fmt.Errorf("some other error"),
			want: false,
		},
		{
			name: "wrapped SQLITE_BUSY error",
			err:  fmt.Errorf("failed to update: %w", fmt.Errorf("SQLITE_BUSY")),
			want: true,
		},
		{
			name: "generic locked error",
			err:  fmt.Errorf("database operation failed: locked"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLockError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
