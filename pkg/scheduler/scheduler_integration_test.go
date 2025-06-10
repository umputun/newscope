package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/llm"
	"github.com/umputun/newscope/pkg/scheduler/mocks"
)

func TestScheduler_Integration_FullWorkflow(t *testing.T) {
	// setup all mocks
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	// test data
	testFeeds := []domain.Feed{
		{ID: 1, URL: "https://example.com/feed1.xml", Title: "Tech Feed", Enabled: true, FetchInterval: time.Hour},
		{ID: 2, URL: "https://example.com/feed2.xml", Title: "News Feed", Enabled: true, FetchInterval: time.Hour},
	}

	parsedItems := map[string]*domain.ParsedFeed{
		"https://example.com/feed1.xml": {
			Title: "Tech Feed",
			Items: []domain.ParsedItem{
				{
					GUID:        "tech-1",
					Title:       "AI breakthrough",
					Link:        "https://example.com/ai-breakthrough",
					Description: "Major AI advancement announced",
					Published:   time.Now(),
				},
			},
		},
		"https://example.com/feed2.xml": {
			Title: "News Feed",
			Items: []domain.ParsedItem{
				{
					GUID:        "news-1",
					Title:       "Breaking News",
					Link:        "https://example.com/breaking-news",
					Description: "Important news update",
					Published:   time.Now(),
				},
			},
		},
	}

	// track created items
	var createdItems []domain.Item
	nextItemID := int64(100)

	// setup mocks
	feedManager.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
		assert.True(t, enabledOnly)
		return testFeeds, nil
	}

	feedManager.GetFeedFunc = func(ctx context.Context, id int64) (*domain.Feed, error) {
		for _, f := range testFeeds {
			if f.ID == id {
				return &f, nil
			}
		}
		return nil, assert.AnError
	}

	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		if pf, ok := parsedItems[url]; ok {
			return pf, nil
		}
		return nil, assert.AnError
	}

	itemManager.ItemExistsFunc = func(ctx context.Context, feedID int64, guid string) (bool, error) {
		// check if item already created
		for _, item := range createdItems {
			if item.FeedID == feedID && item.GUID == guid {
				return true, nil
			}
		}
		return false, nil
	}

	itemManager.ItemExistsByTitleOrURLFunc = func(ctx context.Context, title, url string) (bool, error) {
		for _, item := range createdItems {
			if item.Title == title || item.Link == url {
				return true, nil
			}
		}
		return false, nil
	}

	itemManager.CreateItemFunc = func(ctx context.Context, item *domain.Item) error {
		item.ID = nextItemID
		nextItemID++
		createdItems = append(createdItems, *item)
		return nil
	}

	itemManager.GetItemFunc = func(ctx context.Context, id int64) (*domain.Item, error) {
		for _, item := range createdItems {
			if item.ID == id {
				return &item, nil
			}
		}
		return nil, assert.AnError
	}

	feedManager.UpdateFeedFetchedFunc = func(ctx context.Context, feedID int64, nextFetch time.Time) error {
		return nil
	}

	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{
			Content:     "Extracted content for " + url,
			RichContent: "<p>Rich content for " + url + "</p>",
		}, nil
	}

	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		// return some feedback examples
		return []domain.FeedbackExample{
			{Title: "Previous AI article", Feedback: domain.FeedbackLike, Topics: []string{"ai", "tech"}},
			{Title: "Sports news", Feedback: domain.FeedbackDislike, Topics: []string{"sports"}},
		}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{"tech", "ai", "news", "sports"}, nil
	}

	classificationManager.GetFeedbackCountFunc = func(ctx context.Context) (int64, error) {
		return 50, nil // enough to trigger preference summary update
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		switch key {
		case "preference_summary":
			return "User prefers tech and AI content, dislikes sports", nil
		case "last_summary_feedback_count":
			return "10", nil // will trigger update since we have 50 feedbacks
		default:
			return "", nil
		}
	}

	settingManager.SetSettingFunc = func(ctx context.Context, key, value string) error {
		return nil
	}

	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		var classifications []domain.Classification
		for _, article := range req.Articles {
			score := 7.5
			topics := []string{"tech"}
			if article.Title == "AI breakthrough" {
				score = 9.0
				topics = []string{"tech", "ai"}
			}
			classifications = append(classifications, domain.Classification{
				GUID:        article.GUID,
				Score:       score,
				Topics:      topics,
				Explanation: "Classified based on content",
				Summary:     "Summary of " + article.Title,
			})
		}
		return classifications, nil
	}

	classifier.UpdatePreferenceSummaryFunc = func(ctx context.Context, currentSummary string, newFeedback []domain.FeedbackExample) (string, error) {
		return "Updated preference summary", nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	itemManager.UpdateItemExtractionFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
		return nil
	}

	itemManager.DeleteOldItemsFunc = func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
		return 0, nil // no old items to delete
	}

	// create scheduler with short intervals for testing
	params := Params{
		FeedManager:                feedManager,
		ItemManager:                itemManager,
		ClassificationManager:      classificationManager,
		SettingManager:             settingManager,
		Parser:                     parser,
		Extractor:                  extractor,
		Classifier:                 classifier,
		UpdateInterval:             200 * time.Millisecond,
		MaxWorkers:                 2,
		PreferenceSummaryThreshold: 25,
		CleanupInterval:            time.Hour, // long interval to avoid cleanup during test
	}
	scheduler := NewScheduler(params)

	// start scheduler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)
	defer scheduler.Stop()

	// let the scheduler run through at least one update cycle
	time.Sleep(300 * time.Millisecond)

	// verify feeds were fetched
	assert.GreaterOrEqual(t, len(feedManager.GetFeedsCalls()), 1)

	// verify items were created
	assert.GreaterOrEqual(t, len(itemManager.CreateItemCalls()), 2) // one for each feed

	// verify items were processed (extraction and classification)
	assert.GreaterOrEqual(t, len(extractor.ExtractCalls()), 2)
	assert.GreaterOrEqual(t, len(classifier.ClassifyItemsCalls()), 2)

	// preference summary is only updated when explicitly triggered
	// assert.GreaterOrEqual(t, len(classificationManager.GetFeedbackCountCalls()), 1)

	// test manual feed update
	err := scheduler.UpdateFeedNow(ctx, 1)
	require.NoError(t, err)

	// test manual content extraction
	if len(createdItems) > 0 {
		err = scheduler.ExtractContentNow(ctx, createdItems[0].ID)
		require.NoError(t, err)
	}

	// test preference update directly (worker has 5-minute debounce)
	t.Log("Testing preference update")
	err = scheduler.UpdatePreferenceSummary(ctx)
	require.NoError(t, err)

	// verify preference summary was checked and updated
	assert.GreaterOrEqual(t, len(classificationManager.GetFeedbackCountCalls()), 1)
	assert.GreaterOrEqual(t, len(classifier.UpdatePreferenceSummaryCalls()), 1)

	// also test the trigger mechanism (though it won't process immediately due to debounce)
	scheduler.TriggerPreferenceUpdate()
}

func TestScheduler_Integration_ErrorHandling(t *testing.T) {
	// test scheduler behavior with various errors
	feedManager := &mocks.FeedManagerMock{}
	itemManager := &mocks.ItemManagerMock{}
	classificationManager := &mocks.ClassificationManagerMock{}
	settingManager := &mocks.SettingManagerMock{}
	parser := &mocks.ParserMock{}
	extractor := &mocks.ExtractorMock{}
	classifier := &mocks.ClassifierMock{}

	// setup feed that will have parse error
	testFeed := domain.Feed{
		ID:            1,
		URL:           "https://example.com/broken.xml",
		Title:         "Broken Feed",
		Enabled:       true,
		FetchInterval: time.Hour,
	}

	feedManager.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
		return []domain.Feed{testFeed}, nil
	}

	feedManager.GetFeedFunc = func(ctx context.Context, id int64) (*domain.Feed, error) {
		return &testFeed, nil
	}

	parser.ParseFunc = func(ctx context.Context, url string) (*domain.ParsedFeed, error) {
		return nil, assert.AnError // simulate parse error
	}

	feedManager.UpdateFeedErrorFunc = func(ctx context.Context, feedID int64, errMsg string) error {
		assert.Equal(t, testFeed.ID, feedID)
		assert.NotEmpty(t, errMsg)
		return nil
	}

	// setup other mocks to return empty/default values
	classificationManager.GetRecentFeedbackFunc = func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
		return []domain.FeedbackExample{}, nil
	}

	classificationManager.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
		return []string{}, nil
	}

	classificationManager.GetFeedbackCountFunc = func(ctx context.Context) (int64, error) {
		return 0, nil
	}

	settingManager.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
		return "", nil
	}

	extractor.ExtractFunc = func(ctx context.Context, url string) (*content.ExtractResult, error) {
		return &content.ExtractResult{}, nil
	}

	classifier.ClassifyItemsFunc = func(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error) {
		return []domain.Classification{}, nil
	}

	itemManager.DeleteOldItemsFunc = func(ctx context.Context, age time.Duration, minScore float64) (int64, error) {
		return 0, nil
	}

	itemManager.UpdateItemProcessedFunc = func(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
		return nil
	}

	// create scheduler
	params := Params{
		FeedManager:           feedManager,
		ItemManager:           itemManager,
		ClassificationManager: classificationManager,
		SettingManager:        settingManager,
		Parser:                parser,
		Extractor:             extractor,
		Classifier:            classifier,
		UpdateInterval:        200 * time.Millisecond,
		MaxWorkers:            1,
		CleanupInterval:       time.Hour,
	}
	scheduler := NewScheduler(params)

	// start scheduler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)
	defer scheduler.Stop()

	// let it run
	time.Sleep(300 * time.Millisecond)

	// verify error was handled gracefully
	assert.GreaterOrEqual(t, len(parser.ParseCalls()), 1)
	assert.GreaterOrEqual(t, len(feedManager.UpdateFeedErrorCalls()), 1)

	// scheduler should continue running despite errors
	assert.GreaterOrEqual(t, len(feedManager.GetFeedsCalls()), 1)
}
