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
	Extractor             Extractor // optional, nil disables content extraction
	Classifier            Classifier
	MaxWorkers            int
	RetryFunc             func(ctx context.Context, operation func() error) error
}

// NewFeedProcessor creates a new feed processor with the provided configuration.
// The configuration must include the managers, parser and classifier along with the
// operational parameters (max workers, retry function); the extractor is optional and
// a nil one disables content extraction.
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
	// get batch configuration with defaults
	batchSize := 10
	batchTimeout := 5 * time.Second
	if fp.classifier != nil {
		if cfg, ok := fp.classifier.(*llm.Classifier); ok && cfg.GetBatchSize() > 0 {
			batchSize = cfg.GetBatchSize()
		}
		if cfg, ok := fp.classifier.(*llm.Classifier); ok && cfg.GetBatchTimeout() > 0 {
			batchTimeout = cfg.GetBatchTimeout()
		}
	}

	// create batch collector
	batch := make([]domain.Item, 0, batchSize)
	batchTimer := time.NewTimer(batchTimeout)
	defer batchTimer.Stop()

	// process items in batches
	for {
		select {
		case item, ok := <-items:
			if !ok {
				// channel closed, process remaining batch
				if len(batch) > 0 {
					fp.ProcessBatch(ctx, batch)
				}
				return
			}

			// extract content for the item first
			fp.extractContent(ctx, &item)
			batch = append(batch, item)

			// process batch if full
			if len(batch) >= batchSize {
				fp.ProcessBatch(ctx, batch)
				batch = make([]domain.Item, 0, batchSize)
				batchTimer.Reset(batchTimeout)
			}

		case <-batchTimer.C:
			// timeout reached, process current batch
			if len(batch) > 0 {
				fp.ProcessBatch(ctx, batch)
				batch = make([]domain.Item, 0, batchSize)
			}
			batchTimer.Reset(batchTimeout)

		case <-ctx.Done():
			// context canceled, process remaining batch
			if len(batch) > 0 {
				fp.ProcessBatch(ctx, batch)
			}
			return
		}
	}
}

// extractionEnabled reports whether content extraction is configured
func (fp *FeedProcessor) extractionEnabled() bool {
	return fp.extractor != nil
}

// hasClassifiableContent reports whether the item carries anything to classify.
// With extraction enabled empty content means extraction failed, with extraction
// disabled the feed description is all a description-only feed provides.
func (fp *FeedProcessor) hasClassifiableContent(item *domain.Item) bool {
	if item.Content != "" {
		return true
	}
	return !fp.extractionEnabled() && item.Description != ""
}

// extractContent extracts content for a single item and updates it in the database.
// With extraction disabled the item keeps the content provided by the feed.
func (fp *FeedProcessor) extractContent(ctx context.Context, item *domain.Item) {
	itemID := fp.getItemIdentifier(item)
	if !fp.extractionEnabled() {
		lgr.Printf("[DEBUG] extraction disabled, using feed content for: %s", itemID)
		return
	}
	lgr.Printf("[DEBUG] extracting content for: %s", itemID)

	// extract content
	extracted, err := fp.extractor.Extract(ctx, item.Link)
	if err != nil {
		// check if error indicates unsupported content type (PDF, images, etc)
		if strings.Contains(err.Error(), "unsupported content type") {
			lgr.Printf("[INFO] non-HTML content for item %d from %s: %v", item.ID, item.Link, err)
			// store error for non-HTML content so user knows why it wasn't extracted
			extraction := &domain.ExtractedContent{
				Error:       "Binary content (PDF, image, or other non-HTML format)",
				ExtractedAt: time.Now(),
			}
			updateErr := fp.retryFunc(ctx, func() error {
				return fp.itemManager.UpdateItemExtraction(ctx, item.ID, extraction)
			})
			if updateErr != nil {
				lgr.Printf("[WARN] failed to update extraction status for item %d after retries: %v", item.ID, updateErr)
			}
			return
		}
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

	// update item with extracted content for classification
	item.Content = extracted.Content

	// store extraction in database
	extraction := &domain.ExtractedContent{
		PlainText:   extracted.Content,
		RichHTML:    extracted.RichContent,
		ExtractedAt: time.Now(),
	}

	err = fp.retryFunc(ctx, func() error {
		return fp.itemManager.UpdateItemExtraction(ctx, item.ID, extraction)
	})
	if err != nil {
		lgr.Printf("[WARN] failed to update extraction for item %d after retries: %v", item.ID, err)
	}
}

// ProcessBatch classifies multiple items in a single API call
func (fp *FeedProcessor) ProcessBatch(ctx context.Context, items []domain.Item) {
	if len(items) == 0 {
		return
	}

	lgr.Printf("[INFO] processing batch of %d items", len(items))

	// get context for classification
	feedbacks, err := fp.classificationManager.GetRecentFeedback(ctx, "", 50)
	if err != nil {
		lgr.Printf("[WARN] batch: failed to get feedback examples: %v", err)
		feedbacks = []domain.FeedbackExample{}
	}

	topics, err := fp.classificationManager.GetTopics(ctx)
	if err != nil {
		lgr.Printf("[WARN] batch: failed to get canonical topics: %v", err)
		topics = []string{}
	}

	preferenceSummary, err := fp.settingManager.GetSetting(ctx, "preference_summary")
	if err != nil {
		lgr.Printf("[WARN] batch: failed to get preference summary: %v", err)
		preferenceSummary = ""
	}

	// get topic preferences
	preferredTopics, avoidedTopics := fp.getTopicPreferences(ctx, "batch")

	// classify all items in a single API call
	req := llm.ClassifyRequest{
		Articles:          items,
		Feedbacks:         feedbacks,
		CanonicalTopics:   topics,
		PreferenceSummary: preferenceSummary,
		PreferredTopics:   preferredTopics,
		AvoidedTopics:     avoidedTopics,
	}

	classifications, err := fp.classifier.ClassifyItems(ctx, req)
	if err != nil {
		lgr.Printf("[WARN] failed to classify batch: %v", err)
		return
	}

	// map classifications by GUID for quick lookup
	classMap := make(map[string]domain.Classification)
	for _, class := range classifications {
		classMap[class.GUID] = class
	}

	// update each item with its classification
	for _, item := range items {
		classification, found := classMap[item.GUID]
		if !found {
			lgr.Printf("[WARN] no classification returned for item: %s", item.Title)
			continue
		}

		classification.ClassifiedAt = time.Now()

		// update classification in database (use empty extraction since it's already saved)
		err = fp.retryFunc(ctx, func() error {
			return fp.itemManager.UpdateItemProcessed(ctx, item.ID, nil, &classification)
		})
		if err != nil {
			lgr.Printf("[WARN] failed to update item %d classification after retries: %v", item.ID, err)
			continue
		}

		lgr.Printf("[DEBUG] classified item %d: %s (score: %.1f, topics: %s)",
			item.ID, item.Title, classification.Score, strings.Join(classification.Topics, ", "))
	}

	lgr.Printf("[INFO] batch processing completed: %d/%d items classified", len(classifications), len(items))
}

// ProcessItem handles extraction and classification for a single item.
// This is used for manual triggers and backward compatibility.
func (fp *FeedProcessor) ProcessItem(ctx context.Context, item *domain.Item) {
	// extract content
	fp.extractContent(ctx, item)

	if !fp.hasClassifiableContent(item) {
		lgr.Printf("[WARN] skipping classification for item %d: no content to classify", item.ID)
		return
	}

	// process as single-item batch
	fp.ProcessBatch(ctx, []domain.Item{*item})
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
	if !fp.extractionEnabled() {
		return fmt.Errorf("content extraction is disabled")
	}
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
