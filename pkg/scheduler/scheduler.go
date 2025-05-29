package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-pkgz/lgr"
	"golang.org/x/sync/errgroup"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
)

// Scheduler manages periodic feed updates and content processing
type Scheduler struct {
	db         Database
	parser     Parser
	extractor  Extractor
	classifier Classifier

	updateInterval time.Duration
	maxWorkers     int

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// Database interface for scheduler operations
type Database interface {
	GetFeed(ctx context.Context, id int64) (*db.Feed, error)
	GetFeeds(ctx context.Context, enabledOnly bool) ([]db.Feed, error)
	UpdateFeedFetched(ctx context.Context, feedID int64, nextFetch time.Time) error
	UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error

	GetItem(ctx context.Context, id int64) (*db.Item, error)
	CreateItem(ctx context.Context, item *db.Item) error
	ItemExists(ctx context.Context, feedID int64, guid string) (bool, error)
	UpdateItemProcessed(ctx context.Context, itemID int64, content, richContent string, classification db.Classification) error
	GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error)
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
	UpdateInterval time.Duration
	MaxWorkers     int
}

// NewScheduler creates a new scheduler instance
func NewScheduler(database Database, parser Parser, extractor Extractor, classifier Classifier, cfg Config) *Scheduler {
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 30 * time.Minute
	}
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = 5
	}

	return &Scheduler{
		db:             database,
		parser:         parser,
		extractor:      extractor,
		classifier:     classifier,
		updateInterval: cfg.UpdateInterval,
		maxWorkers:     cfg.MaxWorkers,
	}
}

// Start begins the scheduler
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	// channel for items to process
	processCh := make(chan db.Item, 100)

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
func (s *Scheduler) processingWorker(ctx context.Context, items <-chan db.Item) {
	defer s.wg.Done()

	// get feedback examples once at start
	feedbacks, err := s.db.GetRecentFeedback(ctx, "", 10)
	if err != nil {
		lgr.Printf("[ERROR] failed to get feedback examples: %v", err)
		feedbacks = []db.FeedbackExample{}
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.maxWorkers)

	for item := range items {
		g.Go(func() error {
			s.processItem(ctx, item, feedbacks)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		lgr.Printf("[ERROR] processing worker error: %v", err)
	}
}

// processItem handles extraction and classification for a single item
func (s *Scheduler) processItem(ctx context.Context, item db.Item, feedbacks []db.FeedbackExample) {
	lgr.Printf("[DEBUG] processing item: %s", item.Link)

	// 1. Extract content
	extracted, err := s.extractor.Extract(ctx, item.Link)
	if err != nil {
		lgr.Printf("[WARN] failed to extract content from %s: %v", item.Link, err)
		// don't fail the whole process, just skip classification
		return
	}

	// 2. Classify the item with extracted content
	item.ExtractedContent = extracted.Content

	classifications, err := s.classifier.ClassifyArticles(ctx, []db.Item{item}, feedbacks)
	if err != nil {
		lgr.Printf("[ERROR] failed to classify item: %v", err)
		return
	}

	if len(classifications) == 0 {
		lgr.Printf("[WARN] no classification returned for item: %s", item.Title)
		return
	}

	// 3. Update item with both extraction and classification results
	classification := classifications[0]
	if err := s.db.UpdateItemProcessed(ctx, item.ID, extracted.Content, extracted.RichContent, classification); err != nil {
		lgr.Printf("[ERROR] failed to update item processing: %v", err)
		return
	}

	lgr.Printf("[DEBUG] processed item: %s (score: %.1f)", item.Title, classification.Score)
}

// feedUpdateWorker periodically updates all enabled feeds
func (s *Scheduler) feedUpdateWorker(ctx context.Context, processCh chan<- db.Item) {
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
func (s *Scheduler) updateAllFeeds(ctx context.Context, processCh chan<- db.Item) {
	feeds, err := s.db.GetFeeds(ctx, true)
	if err != nil {
		lgr.Printf("[ERROR] failed to get enabled feeds: %v", err)
		return
	}

	lgr.Printf("[INFO] updating %d feeds", len(feeds))

	// use errgroup with limit for concurrent feed updates
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
func (s *Scheduler) updateFeed(ctx context.Context, f db.Feed, processCh chan<- db.Item) {
	feedName := f.Title
	if feedName == "" {
		feedName = f.URL
	}
	lgr.Printf("[DEBUG] updating feed: %s", feedName)

	parsedFeed, err := s.parser.Parse(ctx, f.URL)
	if err != nil {
		lgr.Printf("[ERROR] failed to parse feed %s: %v", f.URL, err)
		if err := s.db.UpdateFeedError(ctx, f.ID, err.Error()); err != nil {
			lgr.Printf("[ERROR] failed to update feed error: %v", err)
		}
		return
	}

	// store new items
	newCount := 0
	for _, item := range parsedFeed.Items {
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

		if err := s.db.CreateItem(ctx, dbItem); err != nil {
			lgr.Printf("[ERROR] failed to create item: %v", err)
			continue
		}

		newCount++

		// send to processing channel
		select {
		case processCh <- *dbItem:
		case <-ctx.Done():
			return
		}
	}

	// update last fetched timestamp
	nextFetch := time.Now().Add(time.Duration(f.FetchInterval) * time.Second)
	if err := s.db.UpdateFeedFetched(ctx, f.ID, nextFetch); err != nil {
		lgr.Printf("[ERROR] failed to update last fetched: %v", err)
	}

	if newCount > 0 {
		lgr.Printf("[INFO] added %d new items from feed: %s", newCount, feedName)
	}
}

// UpdateFeedNow triggers immediate update of a specific feed
func (s *Scheduler) UpdateFeedNow(ctx context.Context, feedID int64) error {
	feed, err := s.db.GetFeed(ctx, feedID)
	if err != nil {
		return fmt.Errorf("get feed: %w", err)
	}

	// create a temporary channel for processing
	processCh := make(chan db.Item, 10)
	defer close(processCh)

	// process items in background
	go func() {
		for item := range processCh {
			feedbacks, _ := s.db.GetRecentFeedback(ctx, "", 10)
			s.processItem(ctx, item, feedbacks)
		}
	}()

	s.updateFeed(ctx, *feed, processCh)
	return nil
}

// ExtractContentNow triggers immediate content extraction for an item
func (s *Scheduler) ExtractContentNow(ctx context.Context, itemID int64) error {
	// this is now merged with classification, so we just process the item
	item, err := s.db.GetItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("get item: %w", err)
	}

	feedbacks, err := s.db.GetRecentFeedback(ctx, "", 10)
	if err != nil {
		lgr.Printf("[ERROR] failed to get feedback examples: %v", err)
		feedbacks = []db.FeedbackExample{}
	}

	s.processItem(ctx, *item, feedbacks)
	return nil
}
