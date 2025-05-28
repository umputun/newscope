package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
)

// Scheduler manages periodic feed updates, content extraction, and classification
type Scheduler struct {
	db               Database
	parser           Parser
	extractor        Extractor
	classifier       Classifier
	updateInterval   time.Duration
	extractInterval  time.Duration
	classifyInterval time.Duration
	maxWorkers       int
	wg               sync.WaitGroup
	cancel           context.CancelFunc
	dbMutex          sync.Mutex // serialize database writes
}

// Database interface for scheduler operations
type Database interface {
	GetFeed(ctx context.Context, id int64) (*db.Feed, error)
	GetFeeds(ctx context.Context, enabledOnly bool) ([]db.Feed, error)
	GetFeedsToFetch(ctx context.Context, limit int) ([]db.Feed, error)
	UpdateFeedFetched(ctx context.Context, feedID int64, nextFetch time.Time) error
	UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error

	GetItem(ctx context.Context, id int64) (*db.Item, error)
	CreateItem(ctx context.Context, item *db.Item) error
	ItemExists(ctx context.Context, feedID int64, guid string) (bool, error)
	GetItemsNeedingExtraction(ctx context.Context, limit int) ([]db.Item, error)
	UpdateItemExtraction(ctx context.Context, itemID int64, content, richContent string, err error) error

	GetUnclassifiedItems(ctx context.Context, limit int) ([]db.Item, error)
	GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error)
	UpdateClassifications(ctx context.Context, classifications []db.Classification, itemsByGUID map[string]int64) error
}

// Parser interface for feed parsing
type Parser interface {
	Parse(ctx context.Context, url string) (*types.Feed, error)
}

// Extractor interface for content extraction
type Extractor interface {
	Extract(ctx context.Context, url string) (*content.ExtractResult, error)
}

// Classifier interface for LLM classification
type Classifier interface {
	ClassifyArticles(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample) ([]db.Classification, error)
}

// Config holds scheduler configuration
type Config struct {
	UpdateInterval   time.Duration
	ExtractInterval  time.Duration
	ClassifyInterval time.Duration
	MaxWorkers       int
}

// NewScheduler creates a new scheduler instance
func NewScheduler(database Database, parser Parser, extractor Extractor, classifier Classifier, cfg Config) *Scheduler {
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 30 * time.Minute
	}
	if cfg.ExtractInterval == 0 {
		cfg.ExtractInterval = 5 * time.Minute
	}
	if cfg.ClassifyInterval == 0 {
		cfg.ClassifyInterval = 10 * time.Minute
	}
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = 5
	}

	return &Scheduler{
		db:               database,
		parser:           parser,
		extractor:        extractor,
		classifier:       classifier,
		updateInterval:   cfg.UpdateInterval,
		extractInterval:  cfg.ExtractInterval,
		classifyInterval: cfg.ClassifyInterval,
		maxWorkers:       cfg.MaxWorkers,
	}
}

// Start begins the scheduler
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	// start feed update worker
	s.wg.Add(1)
	go s.feedUpdateWorker(ctx)

	// start content extraction worker
	s.wg.Add(1)
	go s.contentExtractionWorker(ctx)

	// start classification worker if classifier is provided
	if s.classifier != nil {
		s.wg.Add(1)
		go s.classificationWorker(ctx)
	}

	lgr.Printf("[INFO] scheduler started with update interval %v, extract interval %v, classify interval %v",
		s.updateInterval, s.extractInterval, s.classifyInterval)
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

// feedUpdateWorker periodically updates all enabled feeds
func (s *Scheduler) feedUpdateWorker(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.updateInterval)
	defer ticker.Stop()

	// run immediately on start
	s.updateAllFeeds(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateAllFeeds(ctx)
		}
	}
}

// updateAllFeeds fetches and updates all enabled feeds
func (s *Scheduler) updateAllFeeds(ctx context.Context) {
	feeds, err := s.db.GetFeeds(ctx, true)
	if err != nil {
		lgr.Printf("[ERROR] failed to get enabled feeds: %v", err)
		return
	}

	lgr.Printf("[INFO] updating %d feeds", len(feeds))

	// use worker pool to update feeds concurrently
	sem := make(chan struct{}, s.maxWorkers)
	var wg sync.WaitGroup

	for _, f := range feeds {
		wg.Add(1)
		go func(feed db.Feed) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			s.updateFeed(ctx, feed)
		}(f)
	}

	wg.Wait()
	lgr.Printf("[INFO] feed update completed")
}

// updateFeed fetches and stores new items for a single feed
func (s *Scheduler) updateFeed(ctx context.Context, f db.Feed) {
	lgr.Printf("[DEBUG] updating feed: %s", f.URL)

	parsedFeed, err := s.parser.Parse(ctx, f.URL)
	if err != nil {
		lgr.Printf("[ERROR] failed to parse feed %s: %v", f.URL, err)
		s.dbMutex.Lock()
		if err := s.db.UpdateFeedError(ctx, f.ID, err.Error()); err != nil {
			lgr.Printf("[ERROR] failed to update feed error: %v", err)
		}
		s.dbMutex.Unlock()
		return
	}

	// update feed metadata if changed - skip for now as we don't have UpdateFeed method
	// TODO: add UpdateFeed method if needed

	// store new items
	newCount := 0
	for _, item := range parsedFeed.Items {
		// check if item already exists
		exists, err := s.db.ItemExists(ctx, f.ID, item.GUID)
		if err != nil {
			lgr.Printf("[ERROR] failed to check item existence: %v", err)
			continue
		}
		if exists {
			continue
		}

		dbItem := &db.Item{
			FeedID:      f.ID,
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Content:     item.Content,
			Author:      item.Author,
			Published:   item.Published,
		}

		s.dbMutex.Lock()
		if err := s.db.CreateItem(ctx, dbItem); err != nil {
			lgr.Printf("[ERROR] failed to create item: %v", err)
			s.dbMutex.Unlock()
			continue
		}
		s.dbMutex.Unlock()

		newCount++

	}

	// update last fetched timestamp
	nextFetch := time.Now().Add(time.Duration(f.FetchInterval) * time.Second)
	s.dbMutex.Lock()
	if err := s.db.UpdateFeedFetched(ctx, f.ID, nextFetch); err != nil {
		lgr.Printf("[ERROR] failed to update last fetched: %v", err)
	}
	s.dbMutex.Unlock()

	if newCount > 0 {
		lgr.Printf("[INFO] added %d new items from feed: %s", newCount, f.Title)
	}
}

// contentExtractionWorker periodically extracts content for items
func (s *Scheduler) contentExtractionWorker(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.extractInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.extractPendingContent(ctx)
		}
	}
}

// extractPendingContent extracts content for items that need it
func (s *Scheduler) extractPendingContent(ctx context.Context) {
	items, err := s.db.GetItemsNeedingExtraction(ctx, s.maxWorkers)
	if err != nil {
		lgr.Printf("[ERROR] failed to get items for extraction: %v", err)
		return
	}

	if len(items) == 0 {
		return
	}

	lgr.Printf("[INFO] extracting content for %d items", len(items))

	// use worker pool for concurrent extraction
	sem := make(chan struct{}, s.maxWorkers)
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)
		go func(i db.Item) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			s.extractItemContent(ctx, i)
		}(item)
	}

	wg.Wait()
	lgr.Printf("[INFO] content extraction completed")
}

// extractItemContent extracts and stores content for a single item
func (s *Scheduler) extractItemContent(ctx context.Context, item db.Item) {
	lgr.Printf("[DEBUG] extracting content for: %s", item.Link)

	extracted, err := s.extractor.Extract(ctx, item.Link)
	if err != nil {
		lgr.Printf("[WARN] failed to extract content from %s: %v", item.Link, err)
		// store extraction error
		s.dbMutex.Lock()
		if err := s.db.UpdateItemExtraction(ctx, item.ID, "", "", err); err != nil {
			lgr.Printf("[WARN] failed to store extraction error: %v", err)
		}
		s.dbMutex.Unlock()
		return
	}

	// store extracted content
	s.dbMutex.Lock()
	if err := s.db.UpdateItemExtraction(ctx, item.ID, extracted.Content, extracted.RichContent, nil); err != nil {
		lgr.Printf("[WARN] failed to store content: %v", err)
	}
	s.dbMutex.Unlock()

	lgr.Printf("[DEBUG] extracted %d characters from: %s", len(extracted.Content), item.Title)
}

// classificationWorker periodically classifies articles using LLM
func (s *Scheduler) classificationWorker(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.classifyInterval)
	defer ticker.Stop()

	// run classification after a short delay to allow content extraction
	time.Sleep(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.classifyPendingItems(ctx)
		}
	}
}

// classifyPendingItems classifies items that haven't been classified yet
func (s *Scheduler) classifyPendingItems(ctx context.Context) {
	if s.classifier == nil {
		return
	}

	batchSize := 5 // matches the default batch_size in config

	// get unclassified items
	items, err := s.db.GetUnclassifiedItems(ctx, batchSize*s.maxWorkers)
	if err != nil {
		lgr.Printf("[ERROR] failed to get unclassified items: %v", err)
		return
	}

	if len(items) == 0 {
		return
	}

	lgr.Printf("[INFO] classifying %d items", len(items))

	// get recent feedback for context
	feedbackLimit := 10 // could be configurable
	feedbacks, err := s.db.GetRecentFeedback(ctx, "", feedbackLimit)
	if err != nil {
		lgr.Printf("[ERROR] failed to get feedback examples: %v", err)
		// continue without feedback
		feedbacks = []db.FeedbackExample{}
	}

	// process items in batches
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		s.classifyBatch(ctx, batch, feedbacks)
	}

	lgr.Printf("[INFO] classification completed")
}

// classifyBatch processes a batch of items through the LLM classifier
func (s *Scheduler) classifyBatch(ctx context.Context, items []db.Item, feedbacks []db.FeedbackExample) {
	// call LLM classifier
	classifications, err := s.classifier.ClassifyArticles(ctx, items, feedbacks)
	if err != nil {
		lgr.Printf("[ERROR] failed to classify batch: %v", err)
		return
	}

	// build a map of GUID to item ID for updating
	itemsByGUID := make(map[string]int64)
	for _, item := range items {
		itemsByGUID[item.GUID] = item.ID
	}

	// update classifications in database
	s.dbMutex.Lock()
	if err := s.db.UpdateClassifications(ctx, classifications, itemsByGUID); err != nil {
		lgr.Printf("[ERROR] failed to update classifications: %v", err)
		s.dbMutex.Unlock()
		return
	}
	s.dbMutex.Unlock()

	lgr.Printf("[DEBUG] classified %d items", len(classifications))
}

// UpdateFeedNow triggers immediate update of a specific feed
func (s *Scheduler) UpdateFeedNow(ctx context.Context, feedID int64) error {
	feed, err := s.db.GetFeed(ctx, feedID)
	if err != nil {
		return fmt.Errorf("get feed: %w", err)
	}

	s.updateFeed(ctx, *feed)
	return nil
}

// ExtractContentNow triggers immediate content extraction for an item
func (s *Scheduler) ExtractContentNow(ctx context.Context, itemID int64) error {
	item, err := s.db.GetItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("get item: %w", err)
	}

	s.extractItemContent(ctx, *item)
	return nil
}

// ClassifyNow triggers immediate classification of pending items
func (s *Scheduler) ClassifyNow(ctx context.Context) error {
	lgr.Printf("[INFO] triggered immediate classification")

	// run classification once
	s.classifyPendingItems(ctx)

	return nil
}
