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
	dbFeeds, err := d.DB.GetFeeds(ctx)
	if err != nil {
		return nil, err
	}

	feeds := make([]types.Feed, len(dbFeeds))
	for i, f := range dbFeeds {
		feeds[i] = types.Feed{
			Title:       f.Title,
			Description: f.Description.String,
			Link:        f.URL,
		}
	}
	return feeds, nil
}

// GetItems adapts db.GetItems to return types.Item
func (d *DBAdapter) GetItems(ctx context.Context, limit, offset int) ([]types.Item, error) {
	dbItems, err := d.DB.GetItems(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	items := make([]types.Item, len(dbItems))
	for i, item := range dbItems {
		items[i] = types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description.String,
			Author:      item.Author.String,
			Published:   item.Published.Time,
		}
	}
	return items, nil
}

// GetItemsByFeed adapts db.GetItemsByFeed to return types.Item
func (d *DBAdapter) GetItemsByFeed(ctx context.Context, feedID int64, limit, offset int) ([]types.Item, error) {
	dbItems, err := d.DB.GetItemsByFeed(ctx, feedID, limit, offset)
	if err != nil {
		return nil, err
	}

	items := make([]types.Item, len(dbItems))
	for i, item := range dbItems {
		items[i] = types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description.String,
			Author:      item.Author.String,
			Published:   item.Published.Time,
		}
	}
	return items, nil
}

// GetItemsWithContent adapts db methods to return types.ItemWithContent
func (d *DBAdapter) GetItemsWithContent(ctx context.Context, limit, offset int) ([]types.ItemWithContent, error) {
	dbItems, err := d.DB.GetItemsWithContent(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	items := make([]types.ItemWithContent, len(dbItems))
	for i, item := range dbItems {
		content := ""
		if item.FullContent.Valid {
			content = item.FullContent.String
		}

		items[i] = types.ItemWithContent{
			Item: types.Item{
				GUID:        item.GUID,
				Title:       item.Title,
				Link:        item.Link,
				Description: item.Description.String,
				Author:      item.Author.String,
				Published:   item.Published.Time,
			},
			ExtractedContent: content,
		}
	}
	return items, nil
}

// SearchItems adapts db.SearchItems
func (d *DBAdapter) SearchItems(ctx context.Context, query string, limit, offset int) ([]types.Item, error) {
	dbItems, err := d.DB.SearchItems(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}

	items := make([]types.Item, len(dbItems))
	for i, item := range dbItems {
		items[i] = types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description.String,
			Author:      item.Author.String,
			Published:   item.Published.Time,
		}
	}
	return items, nil
}

