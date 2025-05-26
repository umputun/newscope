package feed

import (
	"context"
	"fmt"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/umputun/newscope/pkg/feed/types"
)

// HTTPFetcher fetches RSS/Atom feeds via HTTP
type HTTPFetcher struct {
	parser  *gofeed.Parser
	timeout time.Duration
}

// NewHTTPFetcher creates a new feed fetcher
func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	return &HTTPFetcher{
		parser:  gofeed.NewParser(),
		timeout: timeout,
	}
}

// Fetch retrieves and parses a feed from the given URL
func (f *HTTPFetcher) Fetch(ctx context.Context, feedURL, feedName string) ([]types.Item, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	feed, err := f.parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("parse feed %s: %w", feedURL, err)
	}

	items := make([]types.Item, 0, len(feed.Items))
	for _, item := range feed.Items {
		parsed := types.Item{
			FeedName:    feedName,
			Title:       item.Title,
			URL:         item.Link,
			Description: item.Description,
			Content:     item.Content,
			GUID:        item.GUID,
		}

		// parse publish time
		if item.PublishedParsed != nil {
			parsed.Published = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			parsed.Published = *item.UpdatedParsed
		}

		items = append(items, parsed)
	}

	return items, nil
}