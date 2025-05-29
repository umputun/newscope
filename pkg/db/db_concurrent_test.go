package db

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentUpdates tests that concurrent database updates work with retry logic
func TestConcurrentUpdates(t *testing.T) {
	ctx := context.Background()

	// create a test database with file to ensure proper concurrency
	tmpFile := t.TempDir() + "/test.db"
	db, err := New(ctx, Config{
		DSN:          "file:" + tmpFile + "?cache=shared&mode=rwc&_txlock=immediate",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	})
	require.NoError(t, err)
	defer db.Close()

	// create a test feed
	feed := &Feed{
		URL:           "https://example.com/feed",
		Title:         "Test Feed",
		FetchInterval: 30,
		Enabled:       true,
	}
	err = db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	// create test items
	const numItems = 20
	items := make([]*Item, numItems)
	for i := 0; i < numItems; i++ {
		item := &Item{
			FeedID:      feed.ID,
			GUID:        fmt.Sprintf("guid-%d", i),
			Title:       fmt.Sprintf("Item %d", i),
			Link:        fmt.Sprintf("https://example.com/item-%d", i),
			Description: fmt.Sprintf("Description %d", i),
			Published:   time.Now().Add(-time.Duration(i) * time.Hour),
		}
		err := db.CreateItem(ctx, item)
		require.NoError(t, err)
		items[i] = item
	}

	// simulate concurrent updates
	var wg sync.WaitGroup
	errors := make(chan error, numItems)

	// run concurrent updates
	for i := 0; i < numItems; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// simulate processing delay
			time.Sleep(time.Duration(idx%5) * time.Millisecond)

			// update item with classification
			classification := Classification{
				Score:       float64(idx%10) + 0.5,
				Explanation: fmt.Sprintf("Test explanation %d", idx),
				Topics:      []string{fmt.Sprintf("topic-%d", idx%3)},
				Summary:     fmt.Sprintf("Summary %d", idx),
			}

			err := db.UpdateItemProcessed(ctx, items[idx].ID,
				fmt.Sprintf("Extracted content %d", idx),
				fmt.Sprintf("Rich content %d", idx),
				classification)

			if err != nil {
				errors <- err
			}
		}(i)
	}

	// wait for all updates to complete
	wg.Wait()
	close(errors)

	// check for errors
	var errorCount int
	for err := range errors {
		t.Logf("Update error: %v", err)
		errorCount++
	}

	// all updates should succeed with retry logic
	assert.Equal(t, 0, errorCount, "No errors expected with retry logic")

	// verify all items were updated
	for i, item := range items {
		updated, err := db.GetItem(ctx, item.ID)
		require.NoError(t, err)

		assert.NotNil(t, updated.ExtractedAt)
		assert.NotNil(t, updated.ClassifiedAt)
		assert.Equal(t, fmt.Sprintf("Extracted content %d", i), updated.ExtractedContent)
		assert.Equal(t, fmt.Sprintf("Summary %d", i), updated.Description)
		assert.Greater(t, updated.RelevanceScore, 0.0)
	}
}

// TestConcurrentFeedUpdates tests concurrent feed status updates
func TestConcurrentFeedUpdates(t *testing.T) {
	ctx := context.Background()

	// create a test database with file to ensure proper concurrency
	tmpFile := t.TempDir() + "/test.db"
	db, err := New(ctx, Config{
		DSN:          "file:" + tmpFile + "?cache=shared&mode=rwc&_txlock=immediate",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	})
	require.NoError(t, err)
	defer db.Close()

	// create test feeds
	const numFeeds = 10
	feeds := make([]*Feed, numFeeds)
	for i := 0; i < numFeeds; i++ {
		feed := &Feed{
			URL:           fmt.Sprintf("https://example.com/feed%d", i),
			Title:         fmt.Sprintf("Test Feed %d", i),
			FetchInterval: 30,
			Enabled:       true,
		}
		err := db.CreateFeed(ctx, feed)
		require.NoError(t, err)
		feeds[i] = feed
	}

	// simulate concurrent feed updates
	var wg sync.WaitGroup
	errors := make(chan error, numFeeds*2)

	// run concurrent updates
	for i := 0; i < numFeeds; i++ {
		// update feed as fetched
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			nextFetch := time.Now().Add(30 * time.Minute)
			err := db.UpdateFeedFetched(ctx, feeds[idx].ID, nextFetch)
			if err != nil {
				errors <- err
			}
		}(i)

		// simulate some errors
		if i%3 == 0 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				err := db.UpdateFeedError(ctx, feeds[idx].ID, "Test error")
				if err != nil {
					errors <- err
				}
			}(i)
		}
	}

	// wait for all updates to complete
	wg.Wait()
	close(errors)

	// check for errors
	var errorCount int
	for err := range errors {
		t.Logf("Feed update error: %v", err)
		errorCount++
	}

	// all updates should succeed with retry logic
	assert.Equal(t, 0, errorCount, "No errors expected with retry logic")
}

// TestStressWithHighContention tests database under high contention
func TestStressWithHighContention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	ctx := context.Background()

	// create a test database with limited connections to force contention
	tmpFile := t.TempDir() + "/test.db"
	db, err := New(ctx, Config{
		DSN:          "file:" + tmpFile + "?cache=shared&mode=rwc&_txlock=immediate",
		MaxOpenConns: 2, // force contention
		MaxIdleConns: 1,
	})
	require.NoError(t, err)
	defer db.Close()

	// create a single item that all goroutines will update
	feed := &Feed{
		URL:     "https://example.com/feed",
		Title:   "Test Feed",
		Enabled: true,
	}
	err = db.CreateFeed(ctx, feed)
	require.NoError(t, err)

	item := &Item{
		FeedID:      feed.ID,
		GUID:        "test-guid",
		Title:       "Test Item",
		Link:        "https://example.com/item",
		Description: "Test Description",
		Published:   time.Now(),
	}
	err = db.CreateItem(ctx, item)
	require.NoError(t, err)

	// run many concurrent updates to the same item
	const numGoroutines = 50
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			classification := Classification{
				Score:       float64(idx%10) + 0.5,
				Explanation: fmt.Sprintf("Update %d", idx),
				Topics:      []string{fmt.Sprintf("topic-%d", idx%5)},
			}

			err := db.UpdateItemProcessed(ctx, item.ID,
				fmt.Sprintf("Content %d", idx),
				fmt.Sprintf("Rich %d", idx),
				classification)

			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			} else {
				t.Logf("Update %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// with retry logic, all updates should eventually succeed
	assert.Equal(t, numGoroutines, successCount, "All updates should succeed with retry")

	// verify the item was updated
	updated, err := db.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.NotNil(t, updated.ExtractedAt)
	assert.NotNil(t, updated.ClassifiedAt)
}
