package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-pkgz/lgr"
	"golang.org/x/sync/errgroup"

	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/llm"
)

// FeedProcessor handles feed updating and item processing.
// It is responsible for:
//   - Periodically fetching RSS/Atom feeds and detecting new items
//   - Extracting full content from article URLs
//   - Classifying items using the LLM classifier with user preferences
//   - Managing concurrent processing of feeds and items
//   - Retrying failed operations with exponential backoff
//
// The FeedProcessor delegates database operations to the provided managers
// and uses the parser, extractor, and classifier for content processing.
type FeedProcessor struct {
	feedManager           FeedManager
	itemManager           ItemManager
	classificationManager ClassificationManager
	settingManager        SettingManager
	parser                Parser
	extractor             Extractor
	classifier            Classifier

	maxWorkers int
	retryFunc  func(ctx context.Context, operation func() error) error
}

// FeedProcessorConfig holds configuration for FeedProcessor
type FeedProcessorConfig struct {
	FeedManager           FeedManager
	ItemManager           ItemManager
	ClassificationManager ClassificationManager
	SettingManager        SettingManager
	Parser                Parser
	Extractor             Extractor
	Classifier            Classifier
	MaxWorkers            int
	RetryFunc             func(ctx context.Context, operation func() error) error
}

// NewFeedProcessor creates a new feed processor with the provided configuration.
// The configuration must include all required dependencies (managers, parser, extractor, classifier)
// and operational parameters (max workers, retry function).
func NewFeedProcessor(cfg FeedProcessorConfig) *FeedProcessor {
	return &FeedProcessor{
		feedManager:           cfg.FeedManager,
		itemManager:           cfg.ItemManager,
		classificationManager: cfg.ClassificationManager,
		settingManager:        cfg.SettingManager,
		parser:                cfg.Parser,
		extractor:             cfg.Extractor,
		classifier:            cfg.Classifier,
		maxWorkers:            cfg.MaxWorkers,
		retryFunc:             cfg.RetryFunc,
	}
}

// ProcessingWorker processes items from the channel with concurrent workers.
// It manages a pool of workers (limited by maxWorkers) that process items
// for content extraction and classification. This method blocks until the
// channel is closed or the context is canceled.
func (fp *FeedProcessor) ProcessingWorker(ctx context.Context, items <-chan domain.Item) {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fp.maxWorkers)

	for item := range items {
		g.Go(func() error {
			fp.ProcessItem(ctx, &item)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		lgr.Printf("[ERROR] processing worker error: %v", err)
	}
}

// ProcessItem handles extraction and classification for a single item.
// The processing pipeline includes:
// 1. Extracting full content from the item's URL
// 2. Gathering context (feedback, topics, preferences) for classification
// 3. Classifying the item using the LLM with user preferences
// 4. Persisting both extraction and classification results
// Errors at any stage are logged but don't stop the overall process.
func (fp *FeedProcessor) ProcessItem(ctx context.Context, item *domain.Item) {
	itemID := fp.getItemIdentifier(item)
	lgr.Printf("[DEBUG] processing item: %s", itemID)

	// 1. Extract content
	extracted, err := fp.extractor.Extract(ctx, item.Link)
	if err != nil {
		lgr.Printf("[WARN] failed to extract content for item %d from %s: %v", item.ID, item.Link, err)
		extraction := &domain.ExtractedContent{
			Error:       err.Error(),
			ExtractedAt: time.Now(),
		}
		updateErr := fp.retryFunc(ctx, func() error {
			return fp.itemManager.UpdateItemExtraction(ctx, item.ID, extraction)
		})
		if updateErr != nil {
			lgr.Printf("[WARN] failed to update extraction error for item %d after retries: %v", item.ID, updateErr)
		}
		return
	}

	// 2. Get context for classification
	feedbacks, err := fp.classificationManager.GetRecentFeedback(ctx, "", 50)
	if err != nil {
		lgr.Printf("[WARN] item %d: failed to get feedback examples: %v", item.ID, err)
		feedbacks = []domain.FeedbackExample{}
	}

	topics, err := fp.classificationManager.GetTopics(ctx)
	if err != nil {
		lgr.Printf("[WARN] item %d: failed to get canonical topics: %v", item.ID, err)
		topics = []string{}
	}

	preferenceSummary, err := fp.settingManager.GetSetting(ctx, "preference_summary")
	if err != nil {
		lgr.Printf("[WARN] item %d: failed to get preference summary: %v", item.ID, err)
		preferenceSummary = ""
	}

	// get topic preferences
	preferredTopics, avoidedTopics := fp.getTopicPreferences(ctx, itemID)

	// set extracted content for classification
	item.Content = extracted.Content

	// 3. Classify the item
	req := llm.ClassifyRequest{
		Articles:          []domain.Item{*item},
		Feedbacks:         feedbacks,
		CanonicalTopics:   topics,
		PreferenceSummary: preferenceSummary,
		PreferredTopics:   preferredTopics,
		AvoidedTopics:     avoidedTopics,
	}
	classifications, err := fp.classifier.ClassifyItems(ctx, req)
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

	err = fp.retryFunc(ctx, func() error {
		return fp.itemManager.UpdateItemProcessed(ctx, item.ID, extraction, &classification)
	})
	if err != nil {
		lgr.Printf("[WARN] failed to update item %d processing after retries: %v", item.ID, err)
		return
	}

	lgr.Printf("[DEBUG] processed item %d: %s (score: %.1f, topics: %s)", item.ID, item.Title, classification.Score, strings.Join(classification.Topics, ", "))
}

// UpdateAllFeeds fetches and updates all enabled feeds concurrently.
// It retrieves all enabled feeds from the database, then processes each
// feed in parallel (limited by maxWorkers). New items discovered during
// the update are sent to the processCh channel for extraction and classification.
func (fp *FeedProcessor) UpdateAllFeeds(ctx context.Context, processCh chan<- domain.Item) {
	feeds, err := fp.feedManager.GetFeeds(ctx, true)
	if err != nil {
		lgr.Printf("[ERROR] failed to get enabled feeds: %v", err)
		return
	}

	lgr.Printf("[INFO] updating %d feeds", len(feeds))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fp.maxWorkers)

	for _, f := range feeds {
		g.Go(func() error {
			fp.UpdateFeed(ctx, &f, processCh)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		lgr.Printf("[ERROR] feed update error: %v", err)
	}

	lgr.Printf("[INFO] feed update completed")
}

// UpdateFeed fetches and stores new items for a single feed
func (fp *FeedProcessor) UpdateFeed(ctx context.Context, f *domain.Feed, processCh chan<- domain.Item) {
	feedID := fp.getFeedIdentifier(f)
	lgr.Printf("[DEBUG] updating feed: %s", feedID)

	parsedFeed, err := fp.parser.Parse(ctx, f.URL)
	if err != nil {
		lgr.Printf("[WARN] failed to parse feed %s: %v", feedID, err)
		if err := fp.feedManager.UpdateFeedError(ctx, f.ID, err.Error()); err != nil {
			lgr.Printf("[WARN] failed to update error status for feed %s: %v", feedID, err)
		}
		return
	}

	// store new items
	newCount := 0
	for _, item := range parsedFeed.Items {
		// check if item exists
		exists, err := fp.itemManager.ItemExists(ctx, f.ID, item.GUID)
		if err != nil {
			lgr.Printf("[WARN] failed to check item existence in feed %s (GUID %s): %v", feedID, item.GUID, err)
			continue
		}
		if exists {
			continue
		}

		// check for duplicates
		duplicateExists, err := fp.itemManager.ItemExistsByTitleOrURL(ctx, item.Title, item.Link)
		if err != nil {
			lgr.Printf("[WARN] failed to check duplicate item in feed %s (title: %s): %v", feedID, item.Title, err)
			continue
		}
		if duplicateExists {
			lgr.Printf("[DEBUG] skipping duplicate item in feed %s: %s", feedID, item.Title)
			continue
		}

		domainItem := domain.Item{
			FeedID:      f.ID,
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Content:     item.Content,
			Author:      item.Author,
			Published:   item.Published,
		}

		// retry on SQLite lock errors
		createErr := fp.retryFunc(ctx, func() error {
			return fp.itemManager.CreateItem(ctx, &domainItem)
		})
		if createErr != nil {
			lgr.Printf("[WARN] failed to create item in feed %s after retries (title: %s): %v", feedID, item.Title, createErr)
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
	nextFetch := time.Now().Add(f.FetchInterval)
	err = fp.retryFunc(ctx, func() error {
		return fp.feedManager.UpdateFeedFetched(ctx, f.ID, nextFetch)
	})
	if err != nil {
		lgr.Printf("[WARN] failed to update last fetched for feed %s after retries: %v", feedID, err)
	}

	if newCount > 0 {
		lgr.Printf("[INFO] added %d new items from feed %s", newCount, feedID)
	}
}

// UpdateFeedNow triggers immediate update of a specific feed
func (fp *FeedProcessor) UpdateFeedNow(ctx context.Context, feedID int64) error {
	lgr.Printf("[DEBUG] triggering immediate update for feed %d", feedID)
	feed, err := fp.feedManager.GetFeed(ctx, feedID)
	if err != nil {
		return fmt.Errorf("get feed %d: %w", feedID, err)
	}

	processCh := make(chan domain.Item, defaultUpdateFeedBuffer)
	defer close(processCh)

	go func() {
		for item := range processCh {
			fp.ProcessItem(ctx, &item)
		}
	}()

	fp.UpdateFeed(ctx, feed, processCh)
	return nil
}

// ExtractContentNow triggers immediate content extraction for an item
func (fp *FeedProcessor) ExtractContentNow(ctx context.Context, itemID int64) error {
	lgr.Printf("[DEBUG] triggering immediate content extraction for item %d", itemID)
	item, err := fp.itemManager.GetItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("get item %d: %w", itemID, err)
	}

	fp.ProcessItem(ctx, item)
	return nil
}

// getTopicPreferences retrieves user's preferred and avoided topics
func (fp *FeedProcessor) getTopicPreferences(ctx context.Context, itemID string) (preferred, avoided []string) {
	var preferredTopics, avoidedTopics []string

	if preferredJSON, err := fp.settingManager.GetSetting(ctx, domain.SettingPreferredTopics); err == nil && preferredJSON != "" {
		if err := json.Unmarshal([]byte(preferredJSON), &preferredTopics); err != nil {
			lgr.Printf("[WARN] failed to parse preferred topics for %s: %v", itemID, err)
		}
	}

	if avoidedJSON, err := fp.settingManager.GetSetting(ctx, domain.SettingAvoidedTopics); err == nil && avoidedJSON != "" {
		if err := json.Unmarshal([]byte(avoidedJSON), &avoidedTopics); err != nil {
			lgr.Printf("[WARN] failed to parse avoided topics for %s: %v", itemID, err)
		}
	}

	return preferredTopics, avoidedTopics
}

// getFeedIdentifier returns a human-readable identifier for a feed
func (fp *FeedProcessor) getFeedIdentifier(f *domain.Feed) string {
	if f.Title != "" {
		return f.Title
	}
	return f.URL
}

// getItemIdentifier returns a human-readable identifier for an item
func (fp *FeedProcessor) getItemIdentifier(item *domain.Item) string {
	if item.Title != "" {
		return item.Title
	}
	return item.Link
}
