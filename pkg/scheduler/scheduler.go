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

// Database interface for scheduler operations
type Database interface {
	GetEnabledFeeds(ctx context.Context) ([]db.Feed, error)
	GetFeed(ctx context.Context, id int64) (*db.Feed, error)
	UpdateFeed(ctx context.Context, feed *db.Feed) error
	UpdateFeedLastFetched(ctx context.Context, feedID int64, lastFetched time.Time) error
	UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error
	CreateItem(ctx context.Context, item *db.Item) error
	GetItem(ctx context.Context, id int64) (*db.Item, error)
	GetItemsForExtraction(ctx context.Context, limit int) ([]db.Item, error)
	CreateContent(ctx context.Context, content *db.Content) error
}

// Parser interface for feed parsing
type Parser interface {
	Parse(ctx context.Context, url string) (*types.Feed, error)
}

// Extractor interface for content extraction
type Extractor interface {
	Extract(ctx context.Context, url string) (*content.ExtractResult, error)
}

// Scheduler manages periodic feed updates and content extraction
type Scheduler struct {
	db              Database
	parser          Parser
	extractor       Extractor
	updateInterval  time.Duration
	extractInterval time.Duration
	maxWorkers      int
	wg              sync.WaitGroup
	cancel          context.CancelFunc
}

// Config holds scheduler configuration
type Config struct {
	UpdateInterval  time.Duration
	ExtractInterval time.Duration
	MaxWorkers      int
}

// NewScheduler creates a new scheduler instance
func NewScheduler(database Database, parser Parser, extractor Extractor, cfg Config) *Scheduler {
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 30 * time.Minute
	}
	if cfg.ExtractInterval == 0 {
		cfg.ExtractInterval = 5 * time.Minute
	}
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = 5
	}

	return &Scheduler{
		db:              database,
		parser:          parser,
		extractor:       extractor,
		updateInterval:  cfg.UpdateInterval,
		extractInterval: cfg.ExtractInterval,
		maxWorkers:      cfg.MaxWorkers,
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

	lgr.Printf("[INFO] scheduler started with update interval %v and extract interval %v", s.updateInterval, s.extractInterval)
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
	feeds, err := s.db.GetEnabledFeeds(ctx)
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
		if err := s.db.UpdateFeedError(ctx, f.ID, err.Error()); err != nil {
			lgr.Printf("[ERROR] failed to update feed error: %v", err)
		}
		return
	}

	// update feed metadata if changed
	if parsedFeed.Title != f.Title || parsedFeed.Description != f.Description.String {
		f.Title = parsedFeed.Title
		f.Description = db.NullString{String: parsedFeed.Description, Valid: parsedFeed.Description != ""}
		if err := s.db.UpdateFeed(ctx, &f); err != nil {
			lgr.Printf("[ERROR] failed to update feed metadata: %v", err)
		}
	}

	// store new items
	newCount := 0
	for _, item := range parsedFeed.Items {
		dbItem := &db.Item{
			FeedID:      f.ID,
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: db.NullString{String: item.Description, Valid: item.Description != ""},
			Author:      db.NullString{String: item.Author, Valid: item.Author != ""},
			Published:   db.NullTime{Time: item.Published, Valid: !item.Published.IsZero()},
		}

		if err := s.db.CreateItem(ctx, dbItem); err != nil {
			lgr.Printf("[ERROR] failed to create item: %v", err)
			continue
		}

		if dbItem.ID != 0 { // item was created (not duplicate)
			newCount++
		}
	}

	// update last fetched timestamp
	if err := s.db.UpdateFeedLastFetched(ctx, f.ID, time.Now()); err != nil {
		lgr.Printf("[ERROR] failed to update last fetched: %v", err)
	}

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
	items, err := s.db.GetItemsForExtraction(ctx, s.maxWorkers)
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
		lgr.Printf("[ERROR] failed to extract content from %s: %v", item.Link, err)
		// store extraction error
		dbContent := &db.Content{
			ItemID:          item.ID,
			ExtractionError: db.NullString{String: err.Error(), Valid: true},
		}
		if err := s.db.CreateContent(ctx, dbContent); err != nil {
			lgr.Printf("[ERROR] failed to store extraction error: %v", err)
		}
		return
	}

	// store extracted content
	dbContent := &db.Content{
		ItemID:      item.ID,
		FullContent: extracted.Content,
	}

	if err := s.db.CreateContent(ctx, dbContent); err != nil {
		lgr.Printf("[ERROR] failed to store content: %v", err)
		return
	}

	lgr.Printf("[DEBUG] extracted %d characters from: %s", len(extracted.Content), item.Title)
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

