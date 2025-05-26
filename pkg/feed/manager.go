package feed

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/feed/types"
)

//go:generate moq -out mocks/config.go -pkg mocks -skip-ensure -fmt goimports . ConfigProvider
//go:generate moq -out mocks/fetcher.go -pkg mocks -skip-ensure -fmt goimports . Fetcher
//go:generate moq -out mocks/extractor.go -pkg mocks -skip-ensure -fmt goimports . Extractor

// ConfigProvider provides feed configuration
type ConfigProvider interface {
	GetFeeds() []config.Feed
	GetExtractionConfig() config.ExtractionConfig
}

// Fetcher retrieves and parses RSS/Atom feeds
type Fetcher interface {
	Fetch(ctx context.Context, feedURL, feedName string) ([]types.FeedItem, error)
}

// Extractor extracts full content from article URLs
type Extractor interface {
	Extract(ctx context.Context, url string) (string, error)
}

// Manager coordinates feed fetching and content extraction
type Manager struct {
	config    ConfigProvider
	fetcher   Fetcher
	extractor Extractor
	items     []types.ExtractedItem
	mu        sync.RWMutex
}

// NewManager creates a new feed manager
func NewManager(cfg ConfigProvider, fetcher Fetcher, extractor Extractor) *Manager {
	return &Manager{
		config:    cfg,
		fetcher:   fetcher,
		extractor: extractor,
		items:     make([]types.ExtractedItem, 0),
	}
}

// FetchAll fetches all configured feeds and optionally extracts content
func (m *Manager) FetchAll(ctx context.Context) error {
	feeds := m.config.GetFeeds()
	var wg sync.WaitGroup
	itemsChan := make(chan []types.FeedItem, len(feeds))
	errChan := make(chan error, len(feeds))

	// fetch all feeds concurrently
	for _, feed := range feeds {
		wg.Add(1)
		go func(f config.Feed) {
			defer wg.Done()

			log.Printf("[INFO] fetching feed: %s", f.Name)
			items, err := m.fetcher.Fetch(ctx, f.URL, f.Name)
			if err != nil {
				log.Printf("[ERROR] failed to fetch %s: %v", f.Name, err)
				errChan <- err
				return
			}

			log.Printf("[INFO] fetched %d items from %s", len(items), f.Name)
			itemsChan <- items
		}(feed)
	}

	// wait for all fetches to complete
	go func() {
		wg.Wait()
		close(itemsChan)
		close(errChan)
	}()

	// collect feed items
	allItems := make([]types.FeedItem, 0)
	for items := range itemsChan {
		allItems = append(allItems, items...)
	}

	// check for fetch errors
	var firstErr error
	for err := range errChan {
		if firstErr == nil {
			firstErr = err
		}
	}

	// extract content if enabled
	extractedItems := m.extractContent(ctx, allItems)

	// update stored items
	m.mu.Lock()
	m.items = extractedItems
	m.mu.Unlock()

	log.Printf("[INFO] total items processed: %d", len(extractedItems))
	return firstErr
}

// GetItems returns all fetched and extracted items
func (m *Manager) GetItems() []types.ExtractedItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// return a copy to avoid race conditions
	items := make([]types.ExtractedItem, len(m.items))
	copy(items, m.items)
	return items
}

// extractContent extracts full content from feed items
func (m *Manager) extractContent(ctx context.Context, feedItems []types.FeedItem) []types.ExtractedItem {
	extractCfg := m.config.GetExtractionConfig()

	// convert to extracted items without extraction if disabled
	if !extractCfg.Enabled || m.extractor == nil {
		log.Printf("[INFO] content extraction disabled")
		extracted := make([]types.ExtractedItem, len(feedItems))
		for i, item := range feedItems {
			extracted[i] = types.ExtractedItem{
				FeedItem:         item,
				ContentExtracted: false,
			}
		}
		return extracted
	}

	log.Printf("[INFO] extracting content from %d items", len(feedItems))

	// use semaphore to limit concurrent extractions
	sem := make(chan struct{}, extractCfg.MaxConcurrent)

	// rate limiter
	rateLimiter := time.NewTicker(extractCfg.RateLimit)
	defer rateLimiter.Stop()

	extracted := make([]types.ExtractedItem, len(feedItems))
	var wg sync.WaitGroup

	for i, item := range feedItems {
		wg.Add(1)
		go func(idx int, feedItem types.FeedItem) {
			defer wg.Done()

			// acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// wait for rate limit
			<-rateLimiter.C

			extractedItem := types.ExtractedItem{
				FeedItem:    feedItem,
				ExtractedAt: time.Now(),
			}

			// extract content
			content, err := m.extractor.Extract(ctx, feedItem.URL)
			if err != nil {
				log.Printf("[WARN] failed to extract content from %s: %v", feedItem.URL, err)
				extractedItem.ContentExtracted = false
			} else {
				extractedItem.FullContent = content
				extractedItem.ContentExtracted = true
			}

			extracted[idx] = extractedItem
		}(i, item)
	}

	wg.Wait()

	// count successful extractions
	successCount := 0
	for _, item := range extracted {
		if item.ContentExtracted {
			successCount++
		}
	}

	log.Printf("[INFO] content extracted from %d/%d items", successCount, len(feedItems))
	return extracted
}
