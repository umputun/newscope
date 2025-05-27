package feed_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/feed"
	"github.com/umputun/newscope/pkg/feed/mocks"
	"github.com/umputun/newscope/pkg/feed/types"
)

func TestManager_FetchAll(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		// setup mocks
		mockConfig := &mocks.ConfigProviderMock{
			GetFeedsFunc: func() []config.Feed {
				return []config.Feed{
					{URL: "https://feed1.com", Name: "Feed1", Interval: 5 * time.Minute},
					{URL: "https://feed2.com", Name: "Feed2", Interval: 10 * time.Minute},
				}
			},
			GetExtractionConfigFunc: func() config.ExtractionConfig {
				return config.ExtractionConfig{Enabled: false}
			},
		}

		mockFetcher := &mocks.FetcherMock{
			FetchFunc: func(ctx context.Context, feedURL, feedName string) ([]types.Item, error) {
				switch feedURL {
				case "https://feed1.com":
					return []types.Item{
						{FeedName: "Feed1", Title: "Article 1", Link: "https://feed1.com/1"},
						{FeedName: "Feed1", Title: "Article 2", Link: "https://feed1.com/2"},
					}, nil
				case "https://feed2.com":
					return []types.Item{
						{FeedName: "Feed2", Title: "Article 3", Link: "https://feed2.com/3"},
					}, nil
				}
				return nil, errors.New("unexpected feed URL")
			},
		}

		manager := feed.NewManager(mockConfig, mockFetcher, nil)
		err := manager.FetchAll(context.Background())
		require.NoError(t, err)

		// verify all feeds were fetched
		assert.Len(t, mockFetcher.FetchCalls(), 2)

		// check stored items
		items := manager.GetItems()
		assert.Len(t, items, 3)

		// verify items are from both feeds
		feed1Count := 0
		feed2Count := 0
		for _, item := range items {
			switch item.FeedName {
			case "Feed1":
				feed1Count++
			case "Feed2":
				feed2Count++
			}
		}
		assert.Equal(t, 2, feed1Count)
		assert.Equal(t, 1, feed2Count)
	})

	t.Run("partial failure", func(t *testing.T) {
		mockConfig := &mocks.ConfigProviderMock{
			GetFeedsFunc: func() []config.Feed {
				return []config.Feed{
					{URL: "https://feed1.com", Name: "Feed1", Interval: 5 * time.Minute},
					{URL: "https://feed2.com", Name: "Feed2", Interval: 10 * time.Minute},
				}
			},
			GetExtractionConfigFunc: func() config.ExtractionConfig {
				return config.ExtractionConfig{Enabled: false}
			},
		}

		mockFetcher := &mocks.FetcherMock{
			FetchFunc: func(ctx context.Context, feedURL, feedName string) ([]types.Item, error) {
				if feedURL == "https://feed1.com" {
					return []types.Item{
						{FeedName: "Feed1", Title: "Article 1", Link: "https://feed1.com/1"},
					}, nil
				}
				return nil, errors.New("fetch failed")
			},
		}

		manager := feed.NewManager(mockConfig, mockFetcher, nil)
		err := manager.FetchAll(context.Background())
		require.Error(t, err) // should return first error

		// but should still have items from successful fetch
		items := manager.GetItems()
		assert.Len(t, items, 1)
		assert.Equal(t, "Feed1", items[0].FeedName)
	})

	t.Run("no feeds", func(t *testing.T) {
		mockConfig := &mocks.ConfigProviderMock{
			GetFeedsFunc: func() []config.Feed {
				return []config.Feed{}
			},
			GetExtractionConfigFunc: func() config.ExtractionConfig {
				return config.ExtractionConfig{Enabled: false}
			},
		}

		mockFetcher := &mocks.FetcherMock{}

		manager := feed.NewManager(mockConfig, mockFetcher, nil)
		err := manager.FetchAll(context.Background())
		require.NoError(t, err)

		items := manager.GetItems()
		assert.Empty(t, items)
		assert.Empty(t, mockFetcher.FetchCalls())
	})

	t.Run("context cancellation", func(t *testing.T) {
		mockConfig := &mocks.ConfigProviderMock{
			GetFeedsFunc: func() []config.Feed {
				return []config.Feed{
					{URL: "https://feed1.com", Name: "Feed1", Interval: 5 * time.Minute},
				}
			},
			GetExtractionConfigFunc: func() config.ExtractionConfig {
				return config.ExtractionConfig{Enabled: false}
			},
		}

		mockFetcher := &mocks.FetcherMock{
			FetchFunc: func(ctx context.Context, feedURL, feedName string) ([]types.Item, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return []types.Item{{FeedName: "Feed1", Title: "Article 1"}}, nil
				}
			},
		}

		manager := feed.NewManager(mockConfig, mockFetcher, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		err := manager.FetchAll(ctx)
		assert.Error(t, err)
	})
}

func TestManager_GetItems(t *testing.T) {

	t.Run("empty items", func(t *testing.T) {
		mockConfig := &mocks.ConfigProviderMock{
			GetFeedsFunc: func() []config.Feed {
				return []config.Feed{}
			},
			GetExtractionConfigFunc: func() config.ExtractionConfig {
				return config.ExtractionConfig{Enabled: false}
			},
		}
		mockFetcher := &mocks.FetcherMock{}

		manager := feed.NewManager(mockConfig, mockFetcher, nil)
		items := manager.GetItems()
		assert.NotNil(t, items)
		assert.Empty(t, items)
	})
}
