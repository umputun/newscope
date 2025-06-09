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
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-pkgz/lgr"
	"golang.org/x/sync/errgroup"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/llm"
)

const (
	defaultChannelBufferSize = 100
	defaultUpdateFeedBuffer  = 10
)

// FeedManager handles feed operations for scheduler
type FeedManager interface {
	GetFeed(ctx context.Context, id int64) (*domain.Feed, error)
	GetFeeds(ctx context.Context, enabledOnly bool) ([]domain.Feed, error)
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
	DeleteOldItems(ctx context.Context, age time.Duration, minScore float64) (int64, error)
}

// ClassificationManager handles classification operations for scheduler
type ClassificationManager interface {
	GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error)
	GetTopics(ctx context.Context) ([]string, error)
	GetFeedbackCount(ctx context.Context) (int64, error)
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

	updateInterval             time.Duration
	maxWorkers                 int
	preferenceSummaryThreshold int
	cleanupAge                 time.Duration
	cleanupMinScore            float64
	cleanupInterval            time.Duration
	preferenceUpdateCh         chan struct{}

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
	ClassifyItems(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error)
	GeneratePreferenceSummary(ctx context.Context, feedback []domain.FeedbackExample) (string, error)
	UpdatePreferenceSummary(ctx context.Context, currentSummary string, newFeedback []domain.FeedbackExample) (string, error)
}

// Config holds scheduler configuration
type Config struct {
	UpdateInterval             time.Duration
	MaxWorkers                 int
	PreferenceSummaryThreshold int
	CleanupAge                 time.Duration
	CleanupMinScore            float64
	CleanupInterval            time.Duration
}

// Params groups all dependencies needed by the scheduler
type Params struct {
	FeedManager           FeedManager
	ItemManager           ItemManager
	ClassificationManager ClassificationManager
	SettingManager        SettingManager
	Parser                Parser
	Extractor             Extractor
	Classifier            Classifier
}

// NewScheduler creates a new scheduler instance
func NewScheduler(deps Params, cfg Config) *Scheduler {
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = 30 * time.Minute
	}
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = 5
	}
	if cfg.PreferenceSummaryThreshold == 0 {
		cfg.PreferenceSummaryThreshold = 25
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 24 * time.Hour
	}
	if cfg.CleanupAge == 0 {
		cfg.CleanupAge = 168 * time.Hour // 1 week
	}
	if cfg.CleanupMinScore == 0 {
		cfg.CleanupMinScore = 5.0
	}

	return &Scheduler{
		feedManager:                deps.FeedManager,
		itemManager:                deps.ItemManager,
		classificationManager:      deps.ClassificationManager,
		settingManager:             deps.SettingManager,
		parser:                     deps.Parser,
		extractor:                  deps.Extractor,
		classifier:                 deps.Classifier,
		updateInterval:             cfg.UpdateInterval,
		maxWorkers:                 cfg.MaxWorkers,
		preferenceSummaryThreshold: cfg.PreferenceSummaryThreshold,
		cleanupAge:                 cfg.CleanupAge,
		cleanupMinScore:            cfg.CleanupMinScore,
		cleanupInterval:            cfg.CleanupInterval,
		preferenceUpdateCh:         make(chan struct{}, 1), // buffered channel to coalesce updates
	}
}

// Start begins the scheduler
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	// channel for items to process
	processCh := make(chan domain.Item, defaultChannelBufferSize)

	// start processing worker
	s.wg.Add(1)
	go s.processingWorker(ctx, processCh)

	// start feed update worker
	s.wg.Add(1)
	go s.feedUpdateWorker(ctx, processCh)

	// start preference update worker
	s.wg.Add(1)
	go s.preferenceUpdateWorker(ctx)

	// start cleanup worker
	s.wg.Add(1)
	go s.cleanupWorker(ctx)

	lgr.Printf("[INFO] scheduler started with update interval %v, max workers %d, preference threshold %d, cleanup interval %v",
		s.updateInterval, s.maxWorkers, s.preferenceSummaryThreshold, s.cleanupInterval)
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
func (s *Scheduler) processingWorker(ctx context.Context, items <-chan domain.Item) {
	defer s.wg.Done()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.maxWorkers)

	for item := range items {
		g.Go(func() error {
			s.processItem(ctx, &item)
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
	feedbacks, err := s.classificationManager.GetRecentFeedback(ctx, "", 50)
	if err != nil {
		lgr.Printf("[WARN] failed to get feedback examples: %v", err)
		feedbacks = []domain.FeedbackExample{}
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

	// get topic preferences
	var preferredTopics, avoidedTopics []string
	if preferredJSON, err := s.settingManager.GetSetting(ctx, domain.SettingPreferredTopics); err == nil && preferredJSON != "" {
		if err := json.Unmarshal([]byte(preferredJSON), &preferredTopics); err != nil {
			lgr.Printf("[WARN] failed to parse preferred topics: %v", err)
		}
	}
	if avoidedJSON, err := s.settingManager.GetSetting(ctx, domain.SettingAvoidedTopics); err == nil && avoidedJSON != "" {
		if err := json.Unmarshal([]byte(avoidedJSON), &avoidedTopics); err != nil {
			lgr.Printf("[WARN] failed to parse avoided topics: %v", err)
		}
	}

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
	classifications, err := s.classifier.ClassifyItems(ctx, req)
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

	if err := s.itemManager.UpdateItemProcessed(ctx, item.ID, extraction, &classification); err != nil {
		lgr.Printf("[WARN] failed to update item processing: %v", err)
		return
	}

	lgr.Printf("[DEBUG] processed item: %s (score: %.1f)", item.Title, classification.Score)
}

// feedUpdateWorker periodically updates all enabled feeds
func (s *Scheduler) feedUpdateWorker(ctx context.Context, processCh chan<- domain.Item) {
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
func (s *Scheduler) updateAllFeeds(ctx context.Context, processCh chan<- domain.Item) {
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
			s.updateFeed(ctx, &f, processCh)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		lgr.Printf("[ERROR] feed update error: %v", err)
	}

	lgr.Printf("[INFO] feed update completed")
}

// updateFeed fetches and stores new items for a single feed
func (s *Scheduler) updateFeed(ctx context.Context, f *domain.Feed, processCh chan<- domain.Item) {
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

		if err := s.itemManager.CreateItem(ctx, &domainItem); err != nil {
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
	nextFetch := time.Now().Add(f.FetchInterval)
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

	processCh := make(chan domain.Item, defaultUpdateFeedBuffer)
	defer close(processCh)

	go func() {
		for item := range processCh {
			s.processItem(ctx, &item)
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

// TriggerPreferenceUpdate triggers a preference summary update via the worker
func (s *Scheduler) TriggerPreferenceUpdate() {
	// non-blocking send to buffered channel
	select {
	case s.preferenceUpdateCh <- struct{}{}:
		lgr.Printf("[DEBUG] preference update triggered")
	default:
		// channel is full, update already pending
		lgr.Printf("[DEBUG] preference update already pending")
	}
}

// UpdatePreferenceSummary updates the user preference summary based on recent feedback
func (s *Scheduler) UpdatePreferenceSummary(ctx context.Context) error {
	// get more feedback examples for better learning (50 instead of 10)
	const feedbackExamples = 50

	feedbacks, err := s.classificationManager.GetRecentFeedback(ctx, "", feedbackExamples)
	if err != nil {
		return fmt.Errorf("get recent feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		lgr.Printf("[INFO] no feedback to process for preference summary")
		return nil
	}

	// get current preference summary
	currentSummary, err := s.settingManager.GetSetting(ctx, "preference_summary")
	if err != nil || currentSummary == "" {
		// if no summary exists yet, generate initial one
		lgr.Printf("[INFO] generating initial preference summary from %d feedback examples", len(feedbacks))
		newSummary, err := s.classifier.GeneratePreferenceSummary(ctx, feedbacks)
		if err != nil {
			return fmt.Errorf("generate initial preference summary: %w", err)
		}

		if err := s.settingManager.SetSetting(ctx, "preference_summary", newSummary); err != nil {
			return fmt.Errorf("save initial preference summary: %w", err)
		}

		// get current feedback count
		currentCount, err := s.classificationManager.GetFeedbackCount(ctx)
		if err != nil {
			return fmt.Errorf("get feedback count: %w", err)
		}

		// save initial feedback count
		if err := s.settingManager.SetSetting(ctx, "last_summary_feedback_count", strconv.FormatInt(currentCount, 10)); err != nil {
			return fmt.Errorf("save initial feedback count: %w", err)
		}

		lgr.Printf("[INFO] initial preference summary generated successfully")
		return nil
	}

	// get last feedback count from settings
	lastCountStr, _ := s.settingManager.GetSetting(ctx, "last_summary_feedback_count")
	lastCount := int64(0)
	if lastCountStr != "" {
		if parsed, err := strconv.ParseInt(lastCountStr, 10, 64); err == nil {
			lastCount = parsed
		}
	}

	// get current feedback count
	currentCount, err := s.classificationManager.GetFeedbackCount(ctx)
	if err != nil {
		return fmt.Errorf("get feedback count: %w", err)
	}

	// calculate new feedback since last update
	newFeedbackCount := currentCount - lastCount
	if newFeedbackCount < int64(s.preferenceSummaryThreshold) {
		lgr.Printf("[DEBUG] only %d new feedbacks since last update, skipping (threshold: %d)", newFeedbackCount, s.preferenceSummaryThreshold)
		return nil
	}

	// check context before proceeding with expensive operation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	lgr.Printf("[INFO] updating preference summary with %d new feedbacks (total: %d)", newFeedbackCount, currentCount)

	// update the preference summary
	newSummary, err := s.classifier.UpdatePreferenceSummary(ctx, currentSummary, feedbacks)
	if err != nil {
		return fmt.Errorf("update preference summary: %w", err)
	}

	// save updated summary and count
	if err := s.settingManager.SetSetting(ctx, "preference_summary", newSummary); err != nil {
		return fmt.Errorf("save preference summary: %w", err)
	}

	if err := s.settingManager.SetSetting(ctx, "last_summary_feedback_count", strconv.FormatInt(currentCount, 10)); err != nil {
		return fmt.Errorf("save feedback count: %w", err)
	}

	lgr.Printf("[INFO] preference summary updated successfully")
	return nil
}

// preferenceUpdateWorker processes preference update requests
func (s *Scheduler) preferenceUpdateWorker(ctx context.Context) {
	defer s.wg.Done()

	// debounce timer to avoid too frequent updates
	const debounceDelay = 5 * time.Minute
	debounceTimer := time.NewTimer(0)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			debounceTimer.Stop()
			return
		case <-s.preferenceUpdateCh:
			// reset debounce timer
			debounceTimer.Stop()
			debounceTimer.Reset(debounceDelay)
		case <-debounceTimer.C:
			// debounce period expired, perform update
			lgr.Printf("[DEBUG] processing preference update")
			if err := s.UpdatePreferenceSummary(ctx); err != nil {
				lgr.Printf("[WARN] failed to update preference summary: %v", err)
			}
		}
	}
}

// cleanupWorker periodically removes old articles with low scores
func (s *Scheduler) cleanupWorker(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()

	// run cleanup on start
	s.performCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performCleanup(ctx)
		}
	}
}

// performCleanup removes old articles with scores below the threshold
func (s *Scheduler) performCleanup(ctx context.Context) {
	lgr.Printf("[INFO] starting cleanup: removing articles older than %v with score below %.1f", s.cleanupAge, s.cleanupMinScore)

	deleted, err := s.itemManager.DeleteOldItems(ctx, s.cleanupAge, s.cleanupMinScore)
	if err != nil {
		lgr.Printf("[ERROR] cleanup failed: %v", err)
		return
	}

	if deleted > 0 {
		lgr.Printf("[INFO] cleanup completed: removed %d old articles", deleted)
	} else {
		lgr.Printf("[DEBUG] cleanup completed: no articles to remove")
	}
}
