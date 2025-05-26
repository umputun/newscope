package feed

import (
	"context"
	"log"
	"sync"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/feed/types"
)

//go:generate moq -out mocks/config.go -pkg mocks -skip-ensure -fmt goimports . ConfigProvider
//go:generate moq -out mocks/fetcher.go -pkg mocks -skip-ensure -fmt goimports . Fetcher

// ConfigProvider provides feed configuration
type ConfigProvider interface {
	GetFeeds() []config.Feed
}

// Fetcher retrieves and parses RSS/Atom feeds
type Fetcher interface {
	Fetch(ctx context.Context, feedURL, feedName string) ([]types.Item, error)
}

// Manager coordinates feed fetching
type Manager struct {
	config  ConfigProvider
	fetcher Fetcher
	items   []types.Item
	mu      sync.RWMutex
}

// NewManager creates a new feed manager
func NewManager(cfg ConfigProvider, fetcher Fetcher) *Manager {
	return &Manager{
		config:  cfg,
		fetcher: fetcher,
		items:   make([]types.Item, 0),
	}
}

// FetchAll fetches all configured feeds
func (m *Manager) FetchAll(ctx context.Context) error {
	feeds := m.config.GetFeeds()
	var wg sync.WaitGroup
	itemsChan := make(chan []types.Item, len(feeds))
	errChan := make(chan error, len(feeds))

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

	// collect results
	allItems := make([]types.Item, 0)
	for items := range itemsChan {
		allItems = append(allItems, items...)
	}

	// check for errors
	var firstErr error
	for err := range errChan {
		if firstErr == nil {
			firstErr = err
		}
	}

	// update stored items
	m.mu.Lock()
	m.items = allItems
	m.mu.Unlock()

	log.Printf("[INFO] total items fetched: %d", len(allItems))
	return firstErr
}

// GetItems returns all fetched items
func (m *Manager) GetItems() []types.Item {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// return a copy to avoid race conditions
	items := make([]types.Item, len(m.items))
	copy(items, m.items)
	return items
}