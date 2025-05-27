package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create test feed and items
	feed := &Feed{
		URL:   "https://example.com/feed.xml",
		Title: "Test Feed",
	}
	err := db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	item1 := &Item{
		FeedID: feed.ID,
		GUID:   "guid-1",
		Title:  "Article 1",
		Link:   "https://example.com/article1",
	}
	err = db.CreateItem(ctx, item1)
	require.NoError(t, err)

	item2 := &Item{
		FeedID: feed.ID,
		GUID:   "guid-2",
		Title:  "Article 2",
		Link:   "https://example.com/article2",
	}
	err = db.CreateItem(ctx, item2)
	require.NoError(t, err)

	t.Run("create and get content", func(t *testing.T) {
		content := &Content{
			ItemID:      item1.ID,
			FullContent: "This is the full content of article 1. It contains many interesting things.",
		}

		// create content
		err := db.CreateContent(ctx, content)
		require.NoError(t, err)
		assert.NotZero(t, content.ID)

		// verify item was marked as extracted
		item, err := db.GetItem(ctx, item1.ID)
		require.NoError(t, err)
		assert.True(t, item.ContentExtracted)

		// get content
		retrieved, err := db.GetContent(ctx, item1.ID)
		require.NoError(t, err)
		assert.Equal(t, content.FullContent, retrieved.FullContent)
		assert.False(t, retrieved.ExtractionError.Valid)
	})

	t.Run("create content with error", func(t *testing.T) {
		err := db.CreateContentError(ctx, item2.ID, fmt.Errorf("extraction failed: timeout"))
		require.NoError(t, err)

		// verify item was still marked as extracted
		item, err := db.GetItem(ctx, item2.ID)
		require.NoError(t, err)
		assert.True(t, item.ContentExtracted)

		// get content with error
		content, err := db.GetContent(ctx, item2.ID)
		require.NoError(t, err)
		assert.True(t, content.ExtractionError.Valid)
		assert.Contains(t, content.ExtractionError.String, "extraction failed")
	})

	t.Run("get item with content", func(t *testing.T) {
		itemWithContent, err := db.GetItemWithContent(ctx, item1.ID)
		require.NoError(t, err)
		assert.Equal(t, "Article 1", itemWithContent.Title)
		assert.True(t, itemWithContent.FullContent.Valid)
		assert.Contains(t, itemWithContent.FullContent.String, "full content of article 1")
	})

	t.Run("get items with content", func(t *testing.T) {
		items, err := db.GetItemsWithContent(ctx, 10, 0)
		require.NoError(t, err)
		assert.Len(t, items, 2)

		// find article 1
		var article1 *ItemWithContent
		for i := range items {
			if items[i].Title == "Article 1" {
				article1 = &items[i]
				break
			}
		}
		require.NotNil(t, article1)
		assert.True(t, article1.FullContent.Valid)
	})

	t.Run("update content", func(t *testing.T) {
		content, err := db.GetContent(ctx, item1.ID)
		require.NoError(t, err)

		content.FullContent = "Updated content for article 1"
		content.ExtractedAt = time.Now()

		err = db.UpdateContent(ctx, content)
		require.NoError(t, err)

		updated, err := db.GetContent(ctx, item1.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated content for article 1", updated.FullContent)
	})

	t.Run("delete content", func(t *testing.T) {
		err := db.DeleteContent(ctx, item1.ID)
		require.NoError(t, err)

		// content should be gone
		_, err = db.GetContent(ctx, item1.ID)
		require.Error(t, err)

		// item should be marked as not extracted
		item, err := db.GetItem(ctx, item1.ID)
		require.NoError(t, err)
		assert.False(t, item.ContentExtracted)
	})

	t.Run("content stats", func(t *testing.T) {
		stats, err := db.GetContentStats(ctx)
		require.NoError(t, err)

		assert.Equal(t, int64(2), stats["total_items"])
		assert.Equal(t, int64(1), stats["with_content"]) // only item2 has content now
		assert.Equal(t, int64(1), stats["without_content"])
		assert.Equal(t, int64(1), stats["extraction_errors"])
	})
}

func TestContentFullTextSearch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// create test data
	feed := &Feed{
		URL:   "https://example.com/feed.xml",
		Title: "Test Feed",
	}
	err := db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// create items with different content
	items := []struct {
		item    Item
		content string
	}{
		{
			item: Item{
				FeedID:      feed.ID,
				GUID:        "guid-1",
				Title:       "Go Programming",
				Link:        "https://example.com/go",
				Description: sql.NullString{String: "Introduction to Go programming", Valid: true},
			},
			content: "Go is a statically typed, compiled programming language designed at Google.",
		},
		{
			item: Item{
				FeedID:      feed.ID,
				GUID:        "guid-2",
				Title:       "Python Tutorial",
				Link:        "https://example.com/python",
				Description: sql.NullString{String: "Learn Python basics", Valid: true},
			},
			content: "Python is a high-level, interpreted programming language with dynamic semantics.",
		},
		{
			item: Item{
				FeedID:      feed.ID,
				GUID:        "guid-3",
				Title:       "Database Design",
				Link:        "https://example.com/database",
				Description: sql.NullString{String: "Best practices for database design", Valid: true},
			},
			content: "Good database design is crucial for application performance and maintainability.",
		},
	}

	// create items and content
	for _, data := range items {
		err := db.CreateItem(ctx, &data.item)
		require.NoError(t, err)

		content := &Content{
			ItemID:      data.item.ID,
			FullContent: data.content,
		}
		err = db.CreateContent(ctx, content)
		require.NoError(t, err)
	}

	// search tests
	testCases := []struct {
		query         string
		expectedCount int
		expectedTitle string
	}{
		{
			query:         "programming",
			expectedCount: 2, // go and Python articles
		},
		{
			query:         "Go Google",
			expectedCount: 1,
			expectedTitle: "Go Programming",
		},
		{
			query:         "database",
			expectedCount: 1,
			expectedTitle: "Database Design",
		},
		{
			query:         "Python interpreted",
			expectedCount: 1,
			expectedTitle: "Python Tutorial",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.query, func(t *testing.T) {
			results, err := db.SearchItems(ctx, tc.query, 10, 0)
			require.NoError(t, err)
			assert.Len(t, results, tc.expectedCount)

			if tc.expectedTitle != "" && len(results) > 0 {
				assert.Equal(t, tc.expectedTitle, results[0].Title)
			}
		})
	}
}
