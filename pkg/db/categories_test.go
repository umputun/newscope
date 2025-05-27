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

	t.Run("create and get category", func(t *testing.T) {
		category := &Category{
			Name:       "technology",
			IsPositive: true,
			Weight:     1.5,
			Active:     true,
		}

		// set keywords
		err := category.SetKeywords([]string{"tech", "software", "programming"})
		require.NoError(t, err)

		// create category
		err = db.CreateCategory(ctx, category)
		require.NoError(t, err)
		assert.NotZero(t, category.ID)

		// get category by ID
		retrieved, err := db.GetCategory(ctx, category.ID)
		require.NoError(t, err)
		assert.Equal(t, category.Name, retrieved.Name)
		assert.Equal(t, category.IsPositive, retrieved.IsPositive)
		assert.InDelta(t, category.Weight, retrieved.Weight, 0.01)

		// get keywords
		keywords, err := retrieved.GetKeywords()
		require.NoError(t, err)
		assert.Equal(t, []string{"tech", "software", "programming"}, keywords)

		// test empty keywords
		emptyCategory := &Category{Keywords: ""}
		emptyKeywords, err := emptyCategory.GetKeywords()
		require.NoError(t, err)
		assert.Empty(t, emptyKeywords)
	})

	t.Run("get category by name", func(t *testing.T) {
		category := &Category{
			Name:       "science",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}

		err := db.CreateCategory(ctx, category)
		require.NoError(t, err)

		retrieved, err := db.GetCategoryByName(ctx, "science")
		require.NoError(t, err)
		assert.Equal(t, category.ID, retrieved.ID)
		assert.Equal(t, category.Name, retrieved.Name)
	})

	t.Run("assign items to categories", func(t *testing.T) {
		// create test feed
		feed := &Feed{
			URL:   "https://example.com/feed.xml",
			Title: "Test Feed",
		}
		err := db.CreateFeed(ctx, feed)
		require.NoError(t, err)

		// create test item
		item := &Item{
			FeedID: feed.ID,
			GUID:   "item1",
			Title:  "Test Article",
			Link:   "https://example.com/1",
		}
		err = db.CreateItem(ctx, item)
		require.NoError(t, err)

		// create category
		category := &Category{
			Name:       "test-category",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}
		err = db.CreateCategory(ctx, category)
		require.NoError(t, err)

		// assign category to item
		err = db.AssignItemCategory(ctx, item.ID, category.ID, 0.8)
		require.NoError(t, err)

		// get item categories
		categories, err := db.GetItemCategories(ctx, item.ID)
		require.NoError(t, err)
		assert.Len(t, categories, 1)
		assert.Equal(t, category.Name, categories[0].Name)

		// get items by category
		items, err := db.GetItemsByCategory(ctx, category.ID, 10, 0)
		require.NoError(t, err)
		assert.Len(t, items, 1)
		assert.Equal(t, item.ID, items[0].ID)
	})

	t.Run("get active categories", func(t *testing.T) {
		// create active category
		activeCat := &Category{
			Name:       "active-cat",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}
		err := db.CreateCategory(ctx, activeCat)
		require.NoError(t, err)

		// create inactive category
		inactiveCat := &Category{
			Name:       "inactive-cat",
			IsPositive: false,
			Weight:     0.5,
			Active:     false,
			Keywords:   "[]",
		}
		err = db.CreateCategory(ctx, inactiveCat)
		require.NoError(t, err)

		// get active categories
		categories, err := db.GetActiveCategories(ctx)
		require.NoError(t, err)

		// check that inactive category is not included
		found := false
		for _, cat := range categories {
			if cat.ID == inactiveCat.ID {
				found = true
				break
			}
		}
		assert.False(t, found, "inactive category should not be in active categories list")
	})

	t.Run("update category", func(t *testing.T) {
		category := &Category{
			Name:       "update-test",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}
		err := db.CreateCategory(ctx, category)
		require.NoError(t, err)

		// update category
		category.Name = "updated-name"
		category.Weight = 2.0
		category.Active = false
		err = db.UpdateCategory(ctx, category)
		require.NoError(t, err)

		// verify update
		updated, err := db.GetCategory(ctx, category.ID)
		require.NoError(t, err)
		assert.Equal(t, "updated-name", updated.Name)
		assert.InDelta(t, 2.0, updated.Weight, 0.01)
		assert.False(t, updated.Active)
	})

	t.Run("delete category", func(t *testing.T) {
		category := &Category{
			Name:       "to-delete",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}
		err := db.CreateCategory(ctx, category)
		require.NoError(t, err)

		// delete category
		err = db.DeleteCategory(ctx, category.ID)
		require.NoError(t, err)

		// verify deletion
		deleted, err := db.GetCategory(ctx, category.ID)
		require.NoError(t, err)
		assert.Nil(t, deleted)
	})

	t.Run("get item categories with confidence", func(t *testing.T) {
		// create test feed and item
		feed := &Feed{
			URL:   "https://example.com/feed2.xml",
			Title: "Test Feed 2",
		}
		err := db.CreateFeed(ctx, feed)
		require.NoError(t, err)

		item := &Item{
			FeedID: feed.ID,
			GUID:   "item-conf",
			Title:  "Test Item",
			Link:   "https://example.com/conf",
		}
		err = db.CreateItem(ctx, item)
		require.NoError(t, err)

		// create categories
		cat1 := &Category{
			Name:       "high-conf",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}
		err = db.CreateCategory(ctx, cat1)
		require.NoError(t, err)

		cat2 := &Category{
			Name:       "low-conf",
			IsPositive: false,
			Weight:     0.5,
			Active:     true,
			Keywords:   "[]",
		}
		err = db.CreateCategory(ctx, cat2)
		require.NoError(t, err)

		// assign with different confidence levels
		err = db.AssignItemCategory(ctx, item.ID, cat1.ID, 0.9)
		require.NoError(t, err)
		err = db.AssignItemCategory(ctx, item.ID, cat2.ID, 0.3)
		require.NoError(t, err)

		// get categories with confidence
		categories, err := db.GetItemCategoriesWithConfidence(ctx, item.ID)
		require.NoError(t, err)
		assert.Len(t, categories, 2)

		// verify confidence values
		for _, cat := range categories {
			switch cat.Name {
			case "high-conf":
				assert.InDelta(t, 0.9, cat.Confidence, 0.01)
			case "low-conf":
				assert.InDelta(t, 0.3, cat.Confidence, 0.01)
			}
		}
	})

	t.Run("remove item category", func(t *testing.T) {
		// create test data
		feed := &Feed{
			URL:   "https://example.com/feed3.xml",
			Title: "Test Feed 3",
		}
		err := db.CreateFeed(ctx, feed)
		require.NoError(t, err)

		item := &Item{
			FeedID: feed.ID,
			GUID:   "item-remove",
			Title:  "Test Item",
			Link:   "https://example.com/remove",
		}
		err = db.CreateItem(ctx, item)
		require.NoError(t, err)

		category := &Category{
			Name:       "to-remove",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}
		err = db.CreateCategory(ctx, category)
		require.NoError(t, err)

		// assign and then remove
		err = db.AssignItemCategory(ctx, item.ID, category.ID, 0.7)
		require.NoError(t, err)

		err = db.RemoveItemCategory(ctx, item.ID, category.ID)
		require.NoError(t, err)

		// verify removal
		categories, err := db.GetItemCategories(ctx, item.ID)
		require.NoError(t, err)
		assert.Empty(t, categories)
	})

	t.Run("remove all item categories", func(t *testing.T) {
		// create test data
		feed := &Feed{
			URL:   "https://example.com/feed4.xml",
			Title: "Test Feed 4",
		}
		err := db.CreateFeed(ctx, feed)
		require.NoError(t, err)

		item := &Item{
			FeedID: feed.ID,
			GUID:   "item-remove-all",
			Title:  "Test Item",
			Link:   "https://example.com/remove-all",
		}
		err = db.CreateItem(ctx, item)
		require.NoError(t, err)

		// create and assign multiple categories
		for i := 0; i < 3; i++ {
			cat := &Category{
				Name:       string(rune('a' + i)),
				IsPositive: true,
				Weight:     1.0,
				Active:     true,
				Keywords:   "[]",
			}
			err = db.CreateCategory(ctx, cat)
			require.NoError(t, err)
			err = db.AssignItemCategory(ctx, item.ID, cat.ID, 0.5)
			require.NoError(t, err)
		}

		// remove all categories
		err = db.RemoveAllItemCategories(ctx, item.ID)
		require.NoError(t, err)

		// verify all removed
		categories, err := db.GetItemCategories(ctx, item.ID)
		require.NoError(t, err)
		assert.Empty(t, categories)
	})

	t.Run("get category stats", func(t *testing.T) {
		// create test data
		feed := &Feed{
			URL:   "https://example.com/feed5.xml",
			Title: "Test Feed 5",
		}
		err := db.CreateFeed(ctx, feed)
		require.NoError(t, err)

		category := &Category{
			Name:       "stats-test",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}
		err = db.CreateCategory(ctx, category)
		require.NoError(t, err)

		// create and assign items
		for i := 0; i < 5; i++ {
			item := &Item{
				FeedID: feed.ID,
				GUID:   string(rune('A' + i)),
				Title:  "Test Item",
				Link:   "https://example.com/" + string(rune('A'+i)),
			}
			err = db.CreateItem(ctx, item)
			require.NoError(t, err)
			err = db.AssignItemCategory(ctx, item.ID, category.ID, 0.5+float64(i)*0.1)
			require.NoError(t, err)
		}

		// get stats
		stats, err := db.GetCategoryStats(ctx)
		require.NoError(t, err)

		// find our category in stats
		found := false
		for _, stat := range stats {
			if stat.CategoryID != category.ID {
				continue
			}
			found = true
			assert.Equal(t, int64(5), stat.ItemCount)
			assert.True(t, stat.AvgConfidence.Valid)
			assert.InDelta(t, 0.7, stat.AvgConfidence.Float64, 0.01)
			break
		}
		assert.True(t, found, "category should be in stats")
	})

	t.Run("create category errors", func(t *testing.T) {
		// test duplicate category name
		category := &Category{
			Name:       "duplicate-test",
			IsPositive: true,
			Weight:     1.0,
			Active:     true,
			Keywords:   "[]",
		}
		err := db.CreateCategory(ctx, category)
		require.NoError(t, err)

		// try to create another with same name (should fail due to UNIQUE constraint)
		duplicate := &Category{
			Name:       "duplicate-test",
			IsPositive: false,
			Weight:     0.5,
			Active:     true,
			Keywords:   "[]",
		}
		err = db.CreateCategory(ctx, duplicate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create category")
	})
}
