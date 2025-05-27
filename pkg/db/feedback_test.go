package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedbackOperations(t *testing.T) {
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

	items := []Item{
		{FeedID: feed.ID, GUID: "item1", Title: "Article 1", Link: "https://example.com/1"},
		{FeedID: feed.ID, GUID: "item2", Title: "Article 2", Link: "https://example.com/2"},
		{FeedID: feed.ID, GUID: "item3", Title: "Article 3", Link: "https://example.com/3"},
	}

	for i := range items {
		err = db.CreateItem(ctx, &items[i])
		require.NoError(t, err)
	}

	t.Run("create and get user feedback", func(t *testing.T) {
		feedback := &UserFeedback{
			ArticleID:     items[0].ID,
			FeedbackType:  "interesting",
			FeedbackValue: sql.NullInt64{Int64: 5, Valid: true},
			FeedbackAt:    time.Now(),
			TimeSpent:     sql.NullInt64{Int64: 120, Valid: true}, // 2 minutes
		}

		err := db.CreateUserFeedback(ctx, feedback)
		require.NoError(t, err)

		// get feedback
		retrieved, err := db.GetUserFeedback(ctx, items[0].ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, feedback.FeedbackType, retrieved.FeedbackType)
		assert.Equal(t, feedback.FeedbackValue.Int64, retrieved.FeedbackValue.Int64)
		assert.Equal(t, feedback.TimeSpent.Int64, retrieved.TimeSpent.Int64)
		assert.False(t, retrieved.UsedForTraining)
	})

	t.Run("update existing feedback", func(t *testing.T) {
		// update feedback for same article
		newFeedback := &UserFeedback{
			ArticleID:     items[0].ID,
			FeedbackType:  "boring",
			FeedbackValue: sql.NullInt64{Int64: 2, Valid: true},
			FeedbackAt:    time.Now(),
		}

		err := db.CreateUserFeedback(ctx, newFeedback)
		require.NoError(t, err)

		// verify update
		retrieved, err := db.GetUserFeedback(ctx, items[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "boring", retrieved.FeedbackType)
		assert.Equal(t, int64(2), retrieved.FeedbackValue.Int64)
	})

	t.Run("get unused feedback", func(t *testing.T) {
		// create more feedback
		feedbacks := []UserFeedback{
			{ArticleID: items[1].ID, FeedbackType: "interesting", FeedbackValue: sql.NullInt64{Int64: 5, Valid: true}, FeedbackAt: time.Now()},
			{ArticleID: items[2].ID, FeedbackType: "spam", FeedbackAt: time.Now()},
		}

		for i := range feedbacks {
			err := db.CreateUserFeedback(ctx, &feedbacks[i])
			require.NoError(t, err)
		}

		// get unused feedback
		unused, err := db.GetUnusedFeedback(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, unused, 3) // all should be unused

		// verify they're ordered by feedback_at DESC
		for i := 1; i < len(unused); i++ {
			assert.True(t, unused[i-1].FeedbackAt.After(unused[i].FeedbackAt) ||
				unused[i-1].FeedbackAt.Equal(unused[i].FeedbackAt))
		}
	})

	t.Run("mark feedback as used", func(t *testing.T) {
		unused, err := db.GetUnusedFeedback(ctx, 2)
		require.NoError(t, err)
		require.Len(t, unused, 2)

		feedbackIDs := []int64{unused[0].ID, unused[1].ID}
		err = db.MarkFeedbackUsed(ctx, feedbackIDs)
		require.NoError(t, err)

		// verify they're marked as used
		stillUnused, err := db.GetUnusedFeedback(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, stillUnused, 1) // only one should remain unused

		// test with empty IDs (should not error)
		err = db.MarkFeedbackUsed(ctx, []int64{})
		require.NoError(t, err)
	})

	t.Run("get feedback stats", func(t *testing.T) {
		stats, err := db.GetFeedbackStats(ctx)
		require.NoError(t, err)

		feedbackByType := stats["feedback_by_type"].(map[string]int64)
		assert.Equal(t, int64(1), feedbackByType["boring"])
		assert.Equal(t, int64(1), feedbackByType["interesting"])
		assert.Equal(t, int64(1), feedbackByType["spam"])

		// check average value (only 2 have values: 2 and 5)
		if avgValue, ok := stats["avg_feedback_value"]; ok {
			assert.InDelta(t, 3.5, avgValue, 0.01)
		}

		assert.Equal(t, int64(1), stats["unused_feedback_count"])
	})

	t.Run("create and get user actions", func(t *testing.T) {
		action := &UserAction{
			ArticleID: items[0].ID,
			Action:    "view",
			ActionAt:  time.Now(),
			Context:   sql.NullString{String: `{"source": "feed"}`, Valid: true},
		}

		err := db.CreateUserAction(ctx, action)
		require.NoError(t, err)

		// get actions for article
		actions, err := db.GetUserActions(ctx, items[0].ID)
		require.NoError(t, err)
		assert.Len(t, actions, 1)
		assert.Equal(t, "view", actions[0].Action)
		assert.JSONEq(t, `{"source": "feed"}`, actions[0].Context.String)
	})

	t.Run("get recent actions", func(t *testing.T) {
		// create more actions
		actions := []UserAction{
			{ArticleID: items[1].ID, Action: "click", ActionAt: time.Now()},
			{ArticleID: items[2].ID, Action: "share", ActionAt: time.Now()},
			{ArticleID: items[0].ID, Action: "save", ActionAt: time.Now()},
		}

		for i := range actions {
			err := db.CreateUserAction(ctx, &actions[i])
			require.NoError(t, err)
		}

		// get recent actions
		since := time.Now().Add(-1 * time.Hour)
		recent, err := db.GetRecentActions(ctx, 10, since)
		require.NoError(t, err)
		assert.Len(t, recent, 4) // all actions are recent

		// verify order (newest first)
		for i := 1; i < len(recent); i++ {
			assert.True(t, recent[i-1].ActionAt.After(recent[i].ActionAt) ||
				recent[i-1].ActionAt.Equal(recent[i].ActionAt))
		}
	})

	t.Run("get action stats", func(t *testing.T) {
		since := time.Now().Add(-1 * time.Hour)
		stats, err := db.GetActionStats(ctx, since)
		require.NoError(t, err)

		assert.Equal(t, int64(1), stats["view"])
		assert.Equal(t, int64(1), stats["click"])
		assert.Equal(t, int64(1), stats["share"])
		assert.Equal(t, int64(1), stats["save"])
	})

	t.Run("delete old actions", func(t *testing.T) {
		// create an old action
		oldAction := &UserAction{
			ArticleID: items[0].ID,
			Action:    "old-view",
			ActionAt:  time.Now().Add(-48 * time.Hour), // 2 days old
		}
		err := db.CreateUserAction(ctx, oldAction)
		require.NoError(t, err)

		// verify it exists
		allActions, err := db.GetUserActions(ctx, items[0].ID)
		require.NoError(t, err)
		oldCount := len(allActions)

		// delete actions older than 1 day
		deleted, err := db.DeleteOldActions(ctx, 24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, int64(1), deleted)

		// verify old action is gone
		newActions, err := db.GetUserActions(ctx, items[0].ID)
		require.NoError(t, err)
		assert.Len(t, newActions, oldCount-1)
	})
}
