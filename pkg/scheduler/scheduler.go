//go:generate moq -out mocks/feed_manager.go -pkg mocks -skip-ensure -fmt goimports . FeedManager
//go:generate moq -out mocks/item_manager.go -pkg mocks -skip-ensure -fmt goimports . ItemManager
//go:generate moq -out mocks/classification_manager.go -pkg mocks -skip-ensure -fmt goimports . ClassificationManager
//go:generate moq -out mocks/setting_manager.go -pkg mocks -skip-ensure -fmt goimports . SettingManager
//go:generate moq -out mocks/parser.go -pkg mocks -skip-ensure -fmt goimports . Parser
//go:generate moq -out mocks/extractor.go -pkg mocks -skip-ensure -fmt goimports . Extractor
//go:generate moq -out mocks/classifier.go -pkg mocks -skip-ensure -fmt goimports . Classifier

package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-pkgz/lgr"
	"golang.org/x/sync/errgroup"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/domain"
)

// FeedManager handles feed operations for scheduler
type FeedManager interface {
	GetFeed(ctx context.Context, id int64) (*domain.Feed, error)
	GetFeeds(ctx context.Context, enabledOnly bool) ([]*domain.Feed, error)
	UpdateFeedFetched(ctx context.Context, feedID int64, nextFetch time.Time) error
	UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error
}

// ItemManager handles item operations for scheduler
type ItemManager interface {
	GetItem(ctx context.Context, id int64) (*domain.Item, error)
	CreateItem(ctx context.Context, item *domain.Item) error
	ItemExists(ctx context.Context, feedID int64, guid string) (bool, error)
	ItemExistsByTitleOrURL(ctx context.Context, title, url string) (bool, error)
	UpdateItemProcessed(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error
	UpdateItemExtraction(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error
}

// ClassificationManager handles classification operations for scheduler
type ClassificationManager interface {
	GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]*domain.FeedbackExample, error)
	GetTopics(ctx context.Context) ([]string, error)
}

// SettingManager handles settings for scheduler
type SettingManager interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Scheduler manages periodic feed updates and content processing
type Scheduler struct {
	feedManager           FeedManager
	itemManager           ItemManager
	classificationManager ClassificationManager
	settingManager        SettingManager
	parser                Parser
	extractor             Extractor
	classifier            Classifier

	updateInterval time.Duration
	maxWorkers     int

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// Parser interface for feed parsing
type Parser interface {
	Parse(ctx context.Context, url string) (*domain.ParsedFeed, error)
}

// Extractor interface for content extraction
type Extractor interface {
	Extract(ctx context.Context, url string) (*content.ExtractResult, error)
}

// Classifier interface for LLM classification (simplified for now)
type Classifier interface {
	ClassifyItems(ctx context.Context, items []*domain.Item, feedbacks []*domain.FeedbackExample, topics []string, preferenceSummary string) ([]*domain.Classification, error)
	GeneratePreferenceSummary(ctx context.Context, feedback []*domain.FeedbackExample) (string, error)
	UpdatePreferenceSummary(ctx context.Context, currentSummary string, newFeedback []*domain.FeedbackExample) (string, error)
}

// Config holds scheduler configuration
type Config struct {
	UpdateInterval time.Duration
	MaxWorkers     int
}

// NewScheduler creates a new scheduler instance
func NewScheduler(feedManager FeedManager, itemManager ItemManager, classificationManager ClassificationManager, settingManager SettingManager, parser Parser, extractor Extractor, classifier Classifier, cfg Config) *Scheduler {
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 30 * time.Minute
	}
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = 5
	}

	return &Scheduler{
		feedManager:           feedManager,
		itemManager:           itemManager,
		classificationManager: classificationManager,
		settingManager:        settingManager,
		parser:                parser,
		extractor:             extractor,
		classifier:            classifier,
		updateInterval:        cfg.UpdateInterval,
		maxWorkers:            cfg.MaxWorkers,
	}
}

// Start begins the scheduler
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	// channel for items to process
	processCh := make(chan *domain.Item, 100)

	// start processing worker
	s.wg.Add(1)
	go s.processingWorker(ctx, processCh)

	// start feed update worker
	s.wg.Add(1)
	go s.feedUpdateWorker(ctx, processCh)

	lgr.Printf("[INFO] scheduler started with update interval %v, max workers %d",
		s.updateInterval, s.maxWorkers)
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() {
	lgr.Printf("[INFO] stopping scheduler...")
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	lgr.Printf("[INFO] scheduler stopped")
}

// processingWorker processes items from the channel
func (s *Scheduler) processingWorker(ctx context.Context, items <-chan *domain.Item) {
	defer s.wg.Done()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.maxWorkers)

	for item := range items {
		g.Go(func() error {
			s.processItem(ctx, item)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		lgr.Printf("[ERROR] processing worker error: %v", err)
	}
}

// processItem handles extraction and classification for a single item
func (s *Scheduler) processItem(ctx context.Context, item *domain.Item) {
	lgr.Printf("[DEBUG] processing item: %s", item.Link)

	// 1. Extract content
	extracted, err := s.extractor.Extract(ctx, item.Link)
	if err != nil {
		lgr.Printf("[WARN] failed to extract content from %s: %v", item.Link, err)
		extraction := &domain.ExtractedContent{
			Error:       err.Error(),
			ExtractedAt: time.Now(),
		}
		if updateErr := s.itemManager.UpdateItemExtraction(ctx, item.ID, extraction); updateErr != nil {
			lgr.Printf("[WARN] failed to update extraction error: %v", updateErr)
		}
		return
	}

	// 2. Get context for classification
	feedbacks, err := s.classificationManager.GetRecentFeedback(ctx, "", 10)
	if err != nil {
		lgr.Printf("[WARN] failed to get feedback examples: %v", err)
		feedbacks = []*domain.FeedbackExample{}
	}

	topics, err := s.classificationManager.GetTopics(ctx)
	if err != nil {
		lgr.Printf("[WARN] failed to get canonical topics: %v", err)
		topics = []string{}
	}

	preferenceSummary, err := s.settingManager.GetSetting(ctx, "preference_summary")
	if err != nil {
		lgr.Printf("[WARN] failed to get preference summary: %v", err)
		preferenceSummary = ""
	}

	// set extracted content for classification
	item.Content = extracted.Content

	// 3. Classify the item
	classifications, err := s.classifier.ClassifyItems(ctx, []*domain.Item{item}, feedbacks, topics, preferenceSummary)
	if err != nil {
		lgr.Printf("[WARN] failed to classify item: %v", err)
		return
	}

	if len(classifications) == 0 {
		lgr.Printf("[WARN] no classification returned for item: %s", item.Title)
		return
	}

	// 4. Update item with both extraction and classification results
	extraction := &domain.ExtractedContent{
		PlainText:   extracted.Content,
		RichHTML:    extracted.RichContent,
		ExtractedAt: time.Now(),
	}

	classification := classifications[0]
	classification.ClassifiedAt = time.Now()

	if err := s.itemManager.UpdateItemProcessed(ctx, item.ID, extraction, classification); err != nil {
		lgr.Printf("[WARN] failed to update item processing: %v", err)
		return
	}

	lgr.Printf("[DEBUG] processed item: %s (score: %.1f)", item.Title, classification.Score)
}

// feedUpdateWorker periodically updates all enabled feeds
func (s *Scheduler) feedUpdateWorker(ctx context.Context, processCh chan<- *domain.Item) {
	defer s.wg.Done()
	defer close(processCh)

	ticker := time.NewTicker(s.updateInterval)
	defer ticker.Stop()

	// run immediately on start
	s.updateAllFeeds(ctx, processCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateAllFeeds(ctx, processCh)
		}
	}
}

// updateAllFeeds fetches and updates all enabled feeds
func (s *Scheduler) updateAllFeeds(ctx context.Context, processCh chan<- *domain.Item) {
	feeds, err := s.feedManager.GetFeeds(ctx, true)
	if err != nil {
		lgr.Printf("[ERROR] failed to get enabled feeds: %v", err)
		return
	}

	lgr.Printf("[INFO] updating %d feeds", len(feeds))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.maxWorkers)

	for _, f := range feeds {
		g.Go(func() error {
			s.updateFeed(ctx, f, processCh)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		lgr.Printf("[ERROR] feed update error: %v", err)
	}

	lgr.Printf("[INFO] feed update completed")
}

// updateFeed fetches and stores new items for a single feed
func (s *Scheduler) updateFeed(ctx context.Context, f *domain.Feed, processCh chan<- *domain.Item) {
	feedName := f.Title
	if feedName == "" {
		feedName = f.URL
	}
	lgr.Printf("[DEBUG] updating feed: %s", feedName)

	parsedFeed, err := s.parser.Parse(ctx, f.URL)
	if err != nil {
		lgr.Printf("[WARN] failed to parse feed %s: %v", f.URL, err)
		if err := s.feedManager.UpdateFeedError(ctx, f.ID, err.Error()); err != nil {
			lgr.Printf("[WARN] failed to update feed error: %v", err)
		}
		return
	}

	// store new items
	newCount := 0
	for _, item := range parsedFeed.Items {
		// check if item exists
		exists, err := s.itemManager.ItemExists(ctx, f.ID, item.GUID)
		if err != nil {
			lgr.Printf("[WARN] failed to check item existence: %v", err)
			continue
		}
		if exists {
			continue
		}

		// check for duplicates
		duplicateExists, err := s.itemManager.ItemExistsByTitleOrURL(ctx, item.Title, item.Link)
		if err != nil {
			lgr.Printf("[WARN] failed to check duplicate item: %v", err)
			continue
		}
		if duplicateExists {
			lgr.Printf("[DEBUG] skipping duplicate item: %s", item.Title)
			continue
		}

		domainItem := &domain.Item{
			FeedID:      f.ID,
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Content:     item.Content,
			Author:      item.Author,
			Published:   item.Published,
		}

		if err := s.itemManager.CreateItem(ctx, domainItem); err != nil {
			lgr.Printf("[WARN] failed to create item: %v", err)
			continue
		}

		newCount++

		// send to processing channel
		select {
		case processCh <- domainItem:
		case <-ctx.Done():
			return
		}
	}

	// update last fetched timestamp
	nextFetch := time.Now().Add(time.Duration(f.FetchInterval) * time.Second)
	if err := s.feedManager.UpdateFeedFetched(ctx, f.ID, nextFetch); err != nil {
		lgr.Printf("[WARN] failed to update last fetched: %v", err)
	}

	if newCount > 0 {
		lgr.Printf("[INFO] added %d new items from feed: %s", newCount, feedName)
	}
}

// UpdateFeedNow triggers immediate update of a specific feed
func (s *Scheduler) UpdateFeedNow(ctx context.Context, feedID int64) error {
	feed, err := s.feedManager.GetFeed(ctx, feedID)
	if err != nil {
		return fmt.Errorf("get feed: %w", err)
	}

	processCh := make(chan *domain.Item, 10)
	defer close(processCh)

	go func() {
		for item := range processCh {
			s.processItem(ctx, item)
		}
	}()

	s.updateFeed(ctx, feed, processCh)
	return nil
}

// ExtractContentNow triggers immediate content extraction for an item
func (s *Scheduler) ExtractContentNow(ctx context.Context, itemID int64) error {
	item, err := s.itemManager.GetItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("get item: %w", err)
	}

	s.processItem(ctx, item)
	return nil
}
