package server

import (
	"context"

	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
)

// DBAdapter adapts db.DB to server.Database interface
type DBAdapter struct {
	*db.DB
}

// GetFeeds adapts db.GetFeeds to return types.Feed
func (d *DBAdapter) GetFeeds(ctx context.Context) ([]types.Feed, error) {
	dbFeeds, err := d.DB.GetFeeds(ctx, false) // get all feeds
	if err != nil {
		return nil, err
	}

	feeds := make([]types.Feed, len(dbFeeds))
	for i, f := range dbFeeds {
		feeds[i] = types.Feed{
			Title:       f.Title,
			Description: f.Description,
			Link:        f.URL,
		}
	}
	return feeds, nil
}

// GetItems adapts db.GetItems to return types.Item
func (d *DBAdapter) GetItems(ctx context.Context, limit, _ int) ([]types.Item, error) {
	// DB uses minScore instead of offset
	// for now, return all items with score >= 0
	dbItems, err := d.DB.GetItems(ctx, limit, 0)
	if err != nil {
		return nil, err
	}

	items := make([]types.Item, len(dbItems))
	for i, item := range dbItems {
		items[i] = types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Author:      item.Author,
			Published:   item.Published,
		}
	}
	return items, nil
}
