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
	"strings"
	"sync"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/go-pkgz/repeater/v2"

	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/llm"
)

// Scheduler manages periodic feed updates and content processing
type Scheduler struct {
	feedProcessor     *FeedProcessor
	preferenceManager *PreferenceManager
	itemManager       ItemManager

	updateInterval     time.Duration
	cleanupAge         time.Duration
	cleanupMinScore    float64
	cleanupInterval    time.Duration
	preferenceUpdateCh chan struct{}
	// retry configuration
	retryAttempts     int
	retryInitialDelay time.Duration
	retryMaxDelay     time.Duration
	retryJitter       float64

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

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

// Parser interface for feed parsing
type Parser interface {
	Parse(ctx context.Context, url string) (*domain.ParsedFeed, error)
}

// Extractor interface for content extraction
type Extractor interface {
	Extract(ctx context.Context, url string) (*content.ExtractResult, error)
}

// Classifier interface for LLM classification
type Classifier interface {
	ClassifyItems(ctx context.Context, req llm.ClassifyRequest) ([]domain.Classification, error)
	GeneratePreferenceSummary(ctx context.Context, feedback []domain.FeedbackExample) (string, error)
	UpdatePreferenceSummary(ctx context.Context, currentSummary string, newFeedback []domain.FeedbackExample) (string, error)
}

// Params groups all dependencies and configuration needed by the scheduler
type Params struct {
	// dependencies
	FeedManager           FeedManager
	ItemManager           ItemManager
	ClassificationManager ClassificationManager
	SettingManager        SettingManager
	Parser                Parser
	Extractor             Extractor
	Classifier            Classifier

	// configuration
	UpdateInterval             time.Duration
	MaxWorkers                 int
	PreferenceSummaryThreshold int
	CleanupAge                 time.Duration
	CleanupMinScore            float64
	CleanupInterval            time.Duration
	// retry configuration for database operations
	RetryAttempts     int           // number of retry attempts (default: 5)
	RetryInitialDelay time.Duration // initial retry delay (default: 100ms)
	RetryMaxDelay     time.Duration // max retry delay (default: 5s)
	RetryJitter       float64       // jitter factor 0-1 (default: 0.3)
}

// NewScheduler creates a new scheduler instance
func NewScheduler(params Params) *Scheduler {
	// all defaults are set in config.Load(), not here

	s := &Scheduler{
		itemManager:        params.ItemManager,
		updateInterval:     params.UpdateInterval,
		cleanupAge:         params.CleanupAge,
		cleanupMinScore:    params.CleanupMinScore,
		cleanupInterval:    params.CleanupInterval,
		preferenceUpdateCh: make(chan struct{}, 1), // buffered channel to coalesce updates
		retryAttempts:      params.RetryAttempts,
		retryInitialDelay:  params.RetryInitialDelay,
		retryMaxDelay:      params.RetryMaxDelay,
		retryJitter:        params.RetryJitter,
	}

	// create retry function for components
	retryFunc := func(ctx context.Context, operation func() error) error {
		return s.retryDBOperation(ctx, operation)
	}

	// initialize feed processor
	s.feedProcessor = NewFeedProcessor(FeedProcessorConfig{
		FeedManager:           params.FeedManager,
		ItemManager:           params.ItemManager,
		ClassificationManager: params.ClassificationManager,
		SettingManager:        params.SettingManager,
		Parser:                params.Parser,
		Extractor:             params.Extractor,
		Classifier:            params.Classifier,
		MaxWorkers:            params.MaxWorkers,
		RetryFunc:             retryFunc,
	})

	// initialize preference manager
	s.preferenceManager = NewPreferenceManager(PreferenceManagerConfig{
		ClassificationManager:      params.ClassificationManager,
		SettingManager:             params.SettingManager,
		Classifier:                 params.Classifier,
		PreferenceSummaryThreshold: params.PreferenceSummaryThreshold,
		RetryFunc:                  retryFunc,
	})

	return s
}

// Start begins the scheduler
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	// channel for items to process
	processCh := make(chan domain.Item, defaultChannelBufferSize)

	// start processing worker
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.feedProcessor.ProcessingWorker(ctx, processCh)
	}()

	// start feed update worker
	s.wg.Add(1)
	go s.feedUpdateWorker(ctx, processCh)

	// start preference update worker
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.preferenceManager.PreferenceUpdateWorker(ctx, s.preferenceUpdateCh)
	}()

	// start cleanup worker if cleanup is enabled
	if s.cleanupInterval > 0 {
		s.wg.Add(1)
		go s.cleanupWorker(ctx)
	}

	if s.cleanupInterval > 0 {
		lgr.Printf("[INFO] scheduler started with update interval %v, cleanup interval %v",
			s.updateInterval, s.cleanupInterval)
	} else {
		lgr.Printf("[INFO] scheduler started with update interval %v, cleanup disabled",
			s.updateInterval)
	}
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
func (s *Scheduler) feedUpdateWorker(ctx context.Context, processCh chan<- domain.Item) {
	defer s.wg.Done()
	defer close(processCh)

	ticker := time.NewTicker(s.updateInterval)
	defer ticker.Stop()

	// run immediately on start
	s.feedProcessor.UpdateAllFeeds(ctx, processCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.feedProcessor.UpdateAllFeeds(ctx, processCh)
		}
	}
}

// UpdateFeedNow triggers immediate update of a specific feed
func (s *Scheduler) UpdateFeedNow(ctx context.Context, feedID int64) error {
	return s.feedProcessor.UpdateFeedNow(ctx, feedID)
}

// ExtractContentNow triggers immediate content extraction for an item
func (s *Scheduler) ExtractContentNow(ctx context.Context, itemID int64) error {
	return s.feedProcessor.ExtractContentNow(ctx, itemID)
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
	return s.preferenceManager.UpdatePreferenceSummary(ctx)
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

// retryDBOperation executes a database operation with retry logic for lock errors
func (s *Scheduler) retryDBOperation(ctx context.Context, operation func() error) error {
	attempt := 0
	// use exponential backoff with jitter to avoid thundering herd
	return repeater.NewBackoff(s.retryAttempts, s.retryInitialDelay,
		repeater.WithMaxDelay(s.retryMaxDelay), repeater.WithJitter(s.retryJitter)).Do(ctx, func() error {
		attempt++
		if err := operation(); err != nil {
			if isLockError(err) {
				if attempt < s.retryAttempts {
					lgr.Printf("[DEBUG] retrying database operation (attempt %d/%d): %v", attempt, s.retryAttempts, err)
				}
				return err // retry on lock errors
			}
			return &criticalError{err: err} // stop retrying for other errors
		}
		if attempt > 1 {
			lgr.Printf("[DEBUG] database operation succeeded after %d attempts", attempt)
		}
		return nil
	}, &criticalError{})
}

// criticalError wraps an error to signal repeater to stop retrying
type criticalError struct {
	err error
}

func (e *criticalError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *criticalError) Is(target error) bool {
	_, ok := target.(*criticalError)
	return ok
}



// isLockError checks if an error is a SQLite lock/busy error
func isLockError(err error) bool {
	if err == nil {
		return false
	}

	// check the entire error chain for lock-related messages
	errStr := err.Error()
	return strings.Contains(errStr, "SQLITE_BUSY") ||
		strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "database table is locked") ||
		strings.Contains(errStr, "locked")
}
