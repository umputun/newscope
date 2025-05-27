package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCategoryOperations(t *testing.T) {
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

	t.Run("add and get categories", func(t *testing.T) {
		// add single category
		err := db.AddCategory(ctx, item1.ID, "technology")
		require.NoError(t, err)

		// add multiple categories
		err = db.AddCategories(ctx, item1.ID, []string{"golang", "programming"})
		require.NoError(t, err)

		// get item categories
		categories, err := db.GetItemCategories(ctx, item1.ID)
		require.NoError(t, err)
		assert.Len(t, categories, 3)
		assert.Contains(t, categories, "technology")
		assert.Contains(t, categories, "golang")
		assert.Contains(t, categories, "programming")

		// empty categories for item2
		categories, err = db.GetItemCategories(ctx, item2.ID)
		require.NoError(t, err)
		assert.Empty(t, categories)
	})

	t.Run("get items by category", func(t *testing.T) {
		// add categories to item2
		err := db.AddCategories(ctx, item2.ID, []string{"technology", "database"})
		require.NoError(t, err)

		// get items by category
		items, err := db.GetItemsByCategory(ctx, "technology", 10, 0)
		require.NoError(t, err)
		assert.Len(t, items, 2) // both items have "technology"

		items, err = db.GetItemsByCategory(ctx, "golang", 10, 0)
		require.NoError(t, err)
		assert.Len(t, items, 1) // only item1 has "golang"
		assert.Equal(t, item1.ID, items[0].ID)

		// pagination
		items, err = db.GetItemsByCategory(ctx, "technology", 1, 0)
		require.NoError(t, err)
		assert.Len(t, items, 1)

		items, err = db.GetItemsByCategory(ctx, "technology", 1, 1)
		require.NoError(t, err)
		assert.Len(t, items, 1)
	})

	t.Run("get all categories", func(t *testing.T) {
		categories, err := db.GetAllCategories(ctx)
		require.NoError(t, err)
		assert.Len(t, categories, 4) // technology, golang, programming, database
		assert.Equal(t, []string{"database", "golang", "programming", "technology"}, categories)
	})

	t.Run("get categories with counts", func(t *testing.T) {
		counts, err := db.GetCategoriesWithCounts(ctx)
		require.NoError(t, err)
		assert.Len(t, counts, 4)
		assert.Equal(t, int64(2), counts["technology"])
		assert.Equal(t, int64(1), counts["golang"])
		assert.Equal(t, int64(1), counts["programming"])
		assert.Equal(t, int64(1), counts["database"])
	})

	t.Run("remove category", func(t *testing.T) {
		// remove single category
		err := db.RemoveCategory(ctx, item1.ID, "technology")
		require.NoError(t, err)

		categories, err := db.GetItemCategories(ctx, item1.ID)
		require.NoError(t, err)
		assert.Len(t, categories, 2)
		assert.NotContains(t, categories, "technology")

		// remove non-existent category (should not error)
		err = db.RemoveCategory(ctx, item1.ID, "nonexistent")
		require.NoError(t, err)
	})

	t.Run("remove all categories", func(t *testing.T) {
		// remove all categories from item1
		err := db.RemoveAllCategories(ctx, item1.ID)
		require.NoError(t, err)

		categories, err := db.GetItemCategories(ctx, item1.ID)
		require.NoError(t, err)
		assert.Empty(t, categories)

		// remove from item with no categories (should not error)
		err = db.RemoveAllCategories(ctx, item1.ID)
		require.NoError(t, err)
	})

	t.Run("add duplicate category", func(t *testing.T) {
		// add same category twice
		err := db.AddCategory(ctx, item2.ID, "technology")
		require.Error(t, err) // should fail due to uniqueness
	})
}
