package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/domain"
)

func TestItemRepository_GetItems(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

	// create test items with different scores
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	testItems := make([]domain.Item, 5)
	for i := 0; i < 5; i++ {
		testItems[i] = domain.Item{
			FeedID:      testFeed.ID,
			GUID:        fmt.Sprintf("item-%d", i+1),
			Title:       fmt.Sprintf("Test Article %d", i+1),
			Link:        fmt.Sprintf("https://example.com/item%d", i+1),
			Description: fmt.Sprintf("Description for item %d", i+1),
			Published:   baseTime.Add(time.Duration(i) * time.Hour),
		}
	}

	// create items and add varying classifications
	for i := range testItems {
		err := repos.Item.CreateItem(context.Background(), &testItems[i])
		require.NoError(t, err)

		if i < 3 { // classify first 3 items
			classification := &domain.Classification{
				GUID:        testItems[i].GUID,
				Score:       float64(5 + i*2), // scores: 5, 7, 9
				Explanation: fmt.Sprintf("Classification for item %d", i+1),
				Topics:      []string{"test", "sample"},
			}
			err = repos.Item.UpdateItemClassification(context.Background(), testItems[i].ID, classification)
			require.NoError(t, err)
		}
	}

	t.Run("get items with score filter", func(t *testing.T) {
		items, err := repos.Item.GetItems(context.Background(), 10, 6.0)
		require.NoError(t, err)

		// should return items with score >= 6.0 (items 2 and 3)
		assert.Len(t, items, 2)

		// items should be ordered by published date desc
		assert.Equal(t, "item-3", items[0].GUID)
		assert.Equal(t, "item-2", items[1].GUID)
	})

	t.Run("get items with high score threshold", func(t *testing.T) {
		items, err := repos.Item.GetItems(context.Background(), 10, 10.0)
		require.NoError(t, err)

		// no items should meet this threshold
		assert.Empty(t, items)
	})

	t.Run("get items with limit", func(t *testing.T) {
		items, err := repos.Item.GetItems(context.Background(), 1, 0.0)
		require.NoError(t, err)

		// should return only 1 item (the most recent)
		assert.Len(t, items, 1)
		// GetItems returns items by published DESC, so item-5 should be first
		assert.Equal(t, "item-5", items[0].GUID)
	})

	t.Run("get items returns all items with score filter", func(t *testing.T) {
		items, err := repos.Item.GetItems(context.Background(), 10, 0.0)
		require.NoError(t, err)

		// GetItems returns all items with relevance_score >= threshold
		// since default score is 0, all items are returned
		assert.Len(t, items, 5)

		// should be ordered by published DESC
		assert.Equal(t, "item-5", items[0].GUID)
		assert.Equal(t, "item-4", items[1].GUID)
		assert.Equal(t, "item-3", items[2].GUID)
		assert.Equal(t, "item-2", items[3].GUID)
		assert.Equal(t, "item-1", items[4].GUID)
	})
}

func TestItemRepository_GetUnclassifiedItems(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

	// create test items
	classifiedItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "classified-item",
		Title:       "Classified Article",
		Link:        "https://example.com/classified",
		Description: "Classified description",
		Published:   time.Now(),
	}

	unclassifiedItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "unclassified-item",
		Title:       "Unclassified Article",
		Link:        "https://example.com/unclassified",
		Description: "Unclassified description",
		Published:   time.Now(),
	}

	// create items
	err := repos.Item.CreateItem(context.Background(), classifiedItem)
	require.NoError(t, err)
	err = repos.Item.CreateItem(context.Background(), unclassifiedItem)
	require.NoError(t, err)

	// add extraction to both items (GetUnclassifiedItems requires extracted_content != '')
	extraction := &domain.ExtractedContent{
		PlainText: "Extracted content for testing",
		RichHTML:  "<p>Extracted content for testing</p>",
		Error:     "",
	}
	err = repos.Item.UpdateItemExtraction(context.Background(), classifiedItem.ID, extraction)
	require.NoError(t, err)
	err = repos.Item.UpdateItemExtraction(context.Background(), unclassifiedItem.ID, extraction)
	require.NoError(t, err)

	// classify only one item
	classification := &domain.Classification{
		GUID:        classifiedItem.GUID,
		Score:       8.0,
		Explanation: "Test classification",
		Topics:      []string{"test"},
	}
	err = repos.Item.UpdateItemClassification(context.Background(), classifiedItem.ID, classification)
	require.NoError(t, err)

	t.Run("get unclassified items", func(t *testing.T) {
		items, err := repos.Item.GetUnclassifiedItems(context.Background(), 10)
		require.NoError(t, err)

		// should return only the unclassified item
		assert.Len(t, items, 1)
		assert.Equal(t, "unclassified-item", items[0].GUID)
	})

	t.Run("get unclassified items with limit", func(t *testing.T) {
		// add another unclassified item
		anotherItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        "another-unclassified",
			Title:       "Another Unclassified",
			Link:        "https://example.com/another",
			Description: "Another description",
			Published:   time.Now(),
		}
		err = repos.Item.CreateItem(context.Background(), anotherItem)
		require.NoError(t, err)

		items, err := repos.Item.GetUnclassifiedItems(context.Background(), 1)
		require.NoError(t, err)

		// should return only 1 item due to limit
		assert.Len(t, items, 1)
	})
}

func TestItemRepository_GetItemsNeedingExtraction(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

	// create items - one needs extraction, one already extracted
	needsExtractionItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "needs-extraction",
		Title:       "Needs Extraction",
		Link:        "https://example.com/needs",
		Description: "Needs extraction description",
		Published:   time.Now(),
	}

	extractedItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "already-extracted",
		Title:       "Already Extracted",
		Link:        "https://example.com/extracted",
		Description: "Already extracted description",
		Published:   time.Now(),
	}

	// create items
	err := repos.Item.CreateItem(context.Background(), needsExtractionItem)
	require.NoError(t, err)
	err = repos.Item.CreateItem(context.Background(), extractedItem)
	require.NoError(t, err)

	// mark one item as extracted
	extraction := &domain.ExtractedContent{
		PlainText: "Extracted text",
		RichHTML:  "<p>Extracted HTML</p>",
		Error:     "",
	}
	err = repos.Item.UpdateItemExtraction(context.Background(), extractedItem.ID, extraction)
	require.NoError(t, err)

	t.Run("get items needing extraction", func(t *testing.T) {
		items, err := repos.Item.GetItemsNeedingExtraction(context.Background(), 10)
		require.NoError(t, err)

		// should return only the item that needs extraction
		assert.Len(t, items, 1)
		assert.Equal(t, "needs-extraction", items[0].GUID)
	})

	t.Run("get items needing extraction with limit", func(t *testing.T) {
		// add another item needing extraction
		anotherItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        "another-needs-extraction",
			Title:       "Another Needs Extraction",
			Link:        "https://example.com/another-needs",
			Description: "Another needs extraction",
			Published:   time.Now(),
		}
		err = repos.Item.CreateItem(context.Background(), anotherItem)
		require.NoError(t, err)

		items, err := repos.Item.GetItemsNeedingExtraction(context.Background(), 1)
		require.NoError(t, err)

		// should return only 1 item due to limit
		assert.Len(t, items, 1)
	})
}

func TestItemRepository_UpdateItemExtraction(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed and item
	testFeed := createTestFeed(t, repos, "Test Feed")

	testItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "extraction-item",
		Title:       "Extraction Article",
		Link:        "https://example.com/extraction",
		Description: "Extraction description",
		Published:   time.Now(),
	}
	err := repos.Item.CreateItem(context.Background(), testItem)
	require.NoError(t, err)

	t.Run("update extraction success", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "This is the extracted plain text content.",
			RichHTML:  "<p>This is the <strong>extracted</strong> rich HTML content.</p>",
			Error:     "",
		}

		err := repos.Item.UpdateItemExtraction(context.Background(), testItem.ID, extraction)
		require.NoError(t, err)

		// this test verifies the update doesn't error - extraction verification
		// would require checking via GetClassifiedItem or database query
	})

	t.Run("update extraction error", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "",
			RichHTML:  "",
			Error:     "Failed to extract content from URL",
		}

		err := repos.Item.UpdateItemExtraction(context.Background(), testItem.ID, extraction)
		require.NoError(t, err)
	})

	t.Run("update extraction non-existent item", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "Some text",
			RichHTML:  "<p>Some HTML</p>",
			Error:     "",
		}

		err := repos.Item.UpdateItemExtraction(context.Background(), 99999, extraction)
		assert.NoError(t, err) // sQLite doesn't error on UPDATE with no matches
	})
}

func TestItemRepository_UpdateItemProcessed(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed and item
	testFeed := createTestFeed(t, repos, "Test Feed")

	testItem := &domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "processed-item",
		Title:       "Test Article",
		Link:        "https://example.com/article",
		Description: "Test description",
		Published:   time.Now(),
	}
	err := repos.Item.CreateItem(context.Background(), testItem)
	require.NoError(t, err)

	t.Run("update item with extraction and classification", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "This is the extracted plain text content.",
			RichHTML:  "<p>This is the <strong>extracted</strong> rich HTML content.</p>",
			Error:     "",
		}

		classification := &domain.Classification{
			GUID:        testItem.GUID,
			Score:       8.5,
			Explanation: "High quality technical article",
			Topics:      []string{"technology", "programming"},
			Summary:     "Updated summary from processing",
		}

		err := repos.Item.UpdateItemProcessed(context.Background(), testItem.ID, extraction, classification)
		require.NoError(t, err)

		// verify the update by getting the item and checking fields were set
		// UpdateItemProcessed updates multiple fields atomically
		var result struct {
			Summary          string  `db:"summary"`
			RelevanceScore   float64 `db:"relevance_score"`
			ExtractedContent string  `db:"extracted_content"`
		}
		err = repos.DB.GetContext(context.Background(), &result,
			"SELECT summary, relevance_score, extracted_content FROM items WHERE id = ?", testItem.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated summary from processing", result.Summary)
		assert.InDelta(t, 8.5, result.RelevanceScore, 0.001)
		assert.Equal(t, extraction.PlainText, result.ExtractedContent)
	})

	t.Run("update item without summary", func(t *testing.T) {
		// create another item for this test
		anotherItem := &domain.Item{
			FeedID:      testFeed.ID,
			GUID:        "processed-item-2",
			Title:       "Another Test Article",
			Link:        "https://example.com/article2",
			Description: "Another test description",
			Published:   time.Now(),
		}
		err = repos.Item.CreateItem(context.Background(), anotherItem)
		require.NoError(t, err)

		extraction := &domain.ExtractedContent{
			PlainText: "Plain text without summary update.",
			RichHTML:  "<p>HTML without summary update.</p>",
			Error:     "",
		}

		classification := &domain.Classification{
			GUID:        anotherItem.GUID,
			Score:       7.0,
			Explanation: "Decent article",
			Topics:      []string{"general"},
			Summary:     "", // empty summary - shouldn't update description
		}

		err := repos.Item.UpdateItemProcessed(context.Background(), anotherItem.ID, extraction, classification)
		require.NoError(t, err)

		// verify empty summary is stored correctly
		var summary string
		err = repos.DB.GetContext(context.Background(), &summary,
			"SELECT summary FROM items WHERE id = ?", anotherItem.ID)
		require.NoError(t, err)
		assert.Empty(t, summary)
	})

	t.Run("update non-existent item", func(t *testing.T) {
		extraction := &domain.ExtractedContent{
			PlainText: "Some text",
			RichHTML:  "<p>Some HTML</p>",
			Error:     "",
		}

		classification := &domain.Classification{
			GUID:        "non-existent",
			Score:       5.0,
			Explanation: "Test",
			Topics:      []string{"test"},
		}

		err := repos.Item.UpdateItemProcessed(context.Background(), 99999, extraction, classification)
		require.NoError(t, err) // function should not error on non-existent items
	})
}

func TestItemRepository_ItemExistsByTitleOrURL(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

	// create test items
	existingItems := []domain.Item{
		{
			FeedID:      testFeed.ID,
			GUID:        "existing-item-1",
			Title:       "Unique Test Article",
			Link:        "https://example.com/unique-article",
			Description: "Description for unique article",
			Published:   time.Now(),
		},
		{
			FeedID:      testFeed.ID,
			GUID:        "existing-item-2",
			Title:       "Another Article",
			Link:        "https://different.com/another-article",
			Description: "Description for another article",
			Published:   time.Now(),
		},
	}

	for i := range existingItems {
		err := repos.Item.CreateItem(context.Background(), &existingItems[i])
		require.NoError(t, err)
	}

	t.Run("item exists by title", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Unique Test Article", "https://some-other-url.com")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("item exists by URL", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Some Other Title", "https://example.com/unique-article")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("item exists by both title and URL", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Unique Test Article", "https://example.com/unique-article")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("item does not exist", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Non-existent Title", "https://non-existent.com/url")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("empty title and URL", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "", "")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("check against second item", func(t *testing.T) {
		exists, err := repos.Item.ItemExistsByTitleOrURL(context.Background(), "Another Article", "https://random.com/url")
		require.NoError(t, err)
		assert.True(t, exists) // should find by title match
	})
}

func TestItemRepository_DeleteOldItems(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

	// create test items with different ages, scores, and feedback
	now := time.Now()
	testCases := []struct {
		guid         string
		age          time.Duration
		score        float64
		userFeedback string
		shouldDelete bool
	}{
		{"old-low-score", 10 * 24 * time.Hour, 3.0, "", true},            // old, low score, no feedback
		{"old-high-score", 10 * 24 * time.Hour, 8.0, "", false},          // old, high score, no feedback
		{"old-low-liked", 10 * 24 * time.Hour, 3.0, "like", false},       // old, low score, but liked
		{"recent-low-score", 2 * 24 * time.Hour, 3.0, "", false},         // recent, low score
		{"old-mid-score", 10 * 24 * time.Hour, 4.9, "", true},            // old, just below threshold
		{"old-threshold", 10 * 24 * time.Hour, 5.0, "", false},           // old, exactly at threshold
		{"old-low-disliked", 10 * 24 * time.Hour, 3.0, "dislike", false}, // old, low score, but has feedback
	}

	// create items
	for _, tc := range testCases {
		item := domain.Item{
			FeedID:      testFeed.ID,
			GUID:        tc.guid,
			Title:       "Test " + tc.guid,
			Link:        "https://example.com/" + tc.guid,
			Description: "Description for " + tc.guid,
			Published:   now.Add(-tc.age),
		}
		err := repos.Item.CreateItem(context.Background(), &item)
		require.NoError(t, err)

		// add classification
		classification := &domain.Classification{
			GUID:        item.GUID,
			Score:       tc.score,
			Explanation: "Test classification",
			Topics:      []string{"test"},
		}
		err = repos.Item.UpdateItemClassification(context.Background(), item.ID, classification)
		require.NoError(t, err)

		// add feedback if specified
		if tc.userFeedback != "" {
			feedback := &domain.Feedback{
				Type: domain.FeedbackType(tc.userFeedback),
			}
			err = repos.Classification.UpdateItemFeedback(context.Background(), item.ID, feedback)
			require.NoError(t, err)
		}
	}

	// run cleanup - delete items older than 7 days with score < 5.0
	deleted, err := repos.Item.DeleteOldItems(context.Background(), 7*24*time.Hour, 5.0)
	require.NoError(t, err)

	// should delete 2 items: old-low-score and old-mid-score
	assert.Equal(t, int64(2), deleted)

	// verify remaining items
	items, err := repos.Item.GetItems(context.Background(), 100, 0.0)
	require.NoError(t, err)

	// should have 5 items remaining
	assert.Len(t, items, 5)

	// verify the correct items were deleted
	remainingGUIDs := make(map[string]bool)
	for _, item := range items {
		remainingGUIDs[item.GUID] = true
	}

	// check that deleted items are not present
	assert.False(t, remainingGUIDs["old-low-score"])
	assert.False(t, remainingGUIDs["old-mid-score"])

	// check that preserved items are present
	assert.True(t, remainingGUIDs["old-high-score"])   // high score
	assert.True(t, remainingGUIDs["old-low-liked"])    // has feedback
	assert.True(t, remainingGUIDs["recent-low-score"]) // too recent
	assert.True(t, remainingGUIDs["old-threshold"])    // score at threshold
	assert.True(t, remainingGUIDs["old-low-disliked"]) // has feedback
}

func TestItemRepository_DeleteOldItems_NoItems(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// run cleanup on empty database
	deleted, err := repos.Item.DeleteOldItems(context.Background(), 7*24*time.Hour, 5.0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

func TestItemRepository_DeleteOldItems_NoMatchingItems(t *testing.T) {
	repos, cleanup := setupTestDB(t)
	defer cleanup()

	// create test feed
	testFeed := createTestFeed(t, repos, "Test Feed")

	// create recent high-score item
	item := domain.Item{
		FeedID:      testFeed.ID,
		GUID:        "recent-high",
		Title:       "Recent High Score",
		Link:        "https://example.com/recent",
		Description: "Recent item with high score",
		Published:   time.Now().Add(-1 * time.Hour),
	}
	err := repos.Item.CreateItem(context.Background(), &item)
	require.NoError(t, err)

	// add high score classification
	classification := &domain.Classification{
		GUID:        item.GUID,
		Score:       9.5,
		Explanation: "High score",
		Topics:      []string{"test"},
	}
	err = repos.Item.UpdateItemClassification(context.Background(), item.ID, classification)
	require.NoError(t, err)

	// run cleanup - should not delete anything
	deleted, err := repos.Item.DeleteOldItems(context.Background(), 7*24*time.Hour, 5.0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)

	// verify item still exists
	items, err := repos.Item.GetItems(context.Background(), 10, 0.0)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}
