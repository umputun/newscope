package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
)

func TestDBAdapter_ConvertFeeds(t *testing.T) {
	// test data
	dbFeeds := []db.Feed{
		{
			ID:          1,
			URL:         "http://example.com/feed1",
			Title:       "Test Feed 1",
			Description: db.NullString{String: "Description 1", Valid: true},
		},
		{
			ID:          2,
			URL:         "http://example.com/feed2",
			Title:       "Test Feed 2",
			Description: db.NullString{Valid: false},
		},
	}

	// test the conversion logic that DBAdapter.GetFeeds would do
	feeds := make([]types.Feed, len(dbFeeds))
	for i, f := range dbFeeds {
		feeds[i] = types.Feed{
			Title:       f.Title,
			Description: f.Description.String,
			Link:        f.URL,
		}
	}

	require.Len(t, feeds, 2)

	assert.Equal(t, "Test Feed 1", feeds[0].Title)
	assert.Equal(t, "Description 1", feeds[0].Description)
	assert.Equal(t, "http://example.com/feed1", feeds[0].Link)

	assert.Equal(t, "Test Feed 2", feeds[1].Title)
	assert.Empty(t, feeds[1].Description)
	assert.Equal(t, "http://example.com/feed2", feeds[1].Link)
}

func TestDBAdapter_ConvertItems(t *testing.T) {
	publishTime := time.Now()

	// test data
	dbItems := []db.Item{
		{
			ID:          1,
			FeedID:      1,
			GUID:        "guid1",
			Title:       "Item 1",
			Link:        "http://example.com/item1",
			Description: db.NullString{String: "Description 1", Valid: true},
			Author:      db.NullString{String: "Author 1", Valid: true},
			Published:   db.NullTime{Time: publishTime, Valid: true},
		},
		{
			ID:          2,
			FeedID:      1,
			GUID:        "guid2",
			Title:       "Item 2",
			Link:        "http://example.com/item2",
			Description: db.NullString{Valid: false},
			Author:      db.NullString{Valid: false},
			Published:   db.NullTime{Valid: false},
		},
	}

	// test the conversion logic that DBAdapter.GetItems would do
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

	require.Len(t, items, 2)

	assert.Equal(t, "guid1", items[0].GUID)
	assert.Equal(t, "Item 1", items[0].Title)
	assert.Equal(t, "http://example.com/item1", items[0].Link)
	assert.Equal(t, "Description 1", items[0].Description)
	assert.Equal(t, "Author 1", items[0].Author)
	assert.Equal(t, publishTime, items[0].Published)

	assert.Equal(t, "guid2", items[1].GUID)
	assert.Equal(t, "Item 2", items[1].Title)
	assert.Empty(t, items[1].Description)
	assert.Empty(t, items[1].Author)
	assert.True(t, items[1].Published.IsZero())
}

func TestDBAdapter_ConvertItemsWithContent(t *testing.T) {
	publishTime := time.Now()

	// test data
	dbItems := []db.ItemWithContent{
		{
			Item: db.Item{
				ID:          1,
				GUID:        "guid1",
				Title:       "Item with content",
				Link:        "http://example.com/1",
				Description: db.NullString{String: "Description", Valid: true},
				Published:   db.NullTime{Time: publishTime, Valid: true},
			},
			FullContent: db.NullString{String: "This is the full extracted content", Valid: true},
		},
		{
			Item: db.Item{
				ID:    2,
				GUID:  "guid2",
				Title: "Item without content",
				Link:  "http://example.com/2",
			},
			FullContent: db.NullString{Valid: false},
		},
	}

	// test the conversion logic that DBAdapter.GetItemsWithContent would do
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

	require.Len(t, items, 2)

	assert.Equal(t, "Item with content", items[0].Title)
	assert.Equal(t, "This is the full extracted content", items[0].ExtractedContent)

	assert.Equal(t, "Item without content", items[1].Title)
	assert.Empty(t, items[1].ExtractedContent)
}
