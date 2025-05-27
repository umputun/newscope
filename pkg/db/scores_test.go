package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScoreOperations(t *testing.T) {
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

	t.Run("create and get article score", func(t *testing.T) {
		score := &ArticleScore{
			ArticleID:    items[0].ID,
			RuleScore:    sql.NullFloat64{Float64: 0.7, Valid: true},
			MLScore:      sql.NullFloat64{Float64: 0.8, Valid: true},
			SourceScore:  sql.NullFloat64{Float64: 0.5, Valid: true},
			RecencyScore: sql.NullFloat64{Float64: 0.9, Valid: true},
			FinalScore:   0.75,
			ScoredAt:     time.Now(),
			ModelVersion: sql.NullInt64{Int64: 1, Valid: true},
		}

		// set explanation
		explanation := ScoreExplanation{
			RuleMatches:  []string{"golang", "programming"},
			MLConfidence: 0.85,
			Details: map[string]interface{}{
				"category": "technology",
			},
		}
		err := score.SetScoreExplanation(explanation)
		require.NoError(t, err)

		// create score
		err = db.CreateArticleScore(ctx, score)
		require.NoError(t, err)

		// get score
		retrieved, err := db.GetArticleScore(ctx, items[0].ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.InDelta(t, score.FinalScore, retrieved.FinalScore, 0.01)
		assert.InDelta(t, score.RuleScore.Float64, retrieved.RuleScore.Float64, 0.01)

		// get explanation
		gotExplanation, err := retrieved.GetScoreExplanation()
		require.NoError(t, err)
		assert.Equal(t, explanation.RuleMatches, gotExplanation.RuleMatches)
		assert.InDelta(t, explanation.MLConfidence, gotExplanation.MLConfidence, 0.01)

		// test nil explanation
		scoreNoExp := &ArticleScore{
			ArticleID:  items[0].ID,
			FinalScore: 0.5,
			ScoredAt:   time.Now(),
		}
		nilExp, err := scoreNoExp.GetScoreExplanation()
		require.NoError(t, err)
		assert.Nil(t, nilExp)
	})

	t.Run("get multiple article scores", func(t *testing.T) {
		// create scores for other items
		scores := []ArticleScore{
			{
				ArticleID:  items[1].ID,
				FinalScore: 0.6,
				ScoredAt:   time.Now(),
			},
			{
				ArticleID:  items[2].ID,
				FinalScore: 0.4,
				ScoredAt:   time.Now(),
			},
		}

		for i := range scores {
			err := db.CreateArticleScore(ctx, &scores[i])
			require.NoError(t, err)
		}

		// get scores for multiple articles
		articleIDs := []int64{items[0].ID, items[1].ID, items[2].ID}
		retrieved, err := db.GetArticleScores(ctx, articleIDs)
		require.NoError(t, err)
		assert.Len(t, retrieved, 3)

		// test with empty IDs
		emptyScores, err := db.GetArticleScores(ctx, []int64{})
		require.NoError(t, err)
		assert.Empty(t, emptyScores)
	})

	t.Run("get high scoring articles", func(t *testing.T) {
		highScoring, err := db.GetHighScoringArticles(ctx, 0.6, 10, 0)
		require.NoError(t, err)
		assert.Len(t, highScoring, 2) // should get items[0] and items[1]

		// verify order (highest score first)
		assert.Equal(t, items[0].ID, highScoring[0]) // score 0.75
		assert.Equal(t, items[1].ID, highScoring[1]) // score 0.6
	})

	t.Run("update feed average score", func(t *testing.T) {
		err := db.UpdateFeedAverageScore(ctx, feed.ID)
		require.NoError(t, err)

		// verify the average was updated
		updatedFeed, err := db.GetFeed(ctx, feed.ID)
		require.NoError(t, err)
		assert.True(t, updatedFeed.AvgScore.Valid)
		// average of 0.75, 0.6, 0.4 = 0.583...
		assert.InDelta(t, 0.583, updatedFeed.AvgScore.Float64, 0.01)
	})

	t.Run("get score stats", func(t *testing.T) {
		stats, err := db.GetScoreStats(ctx)
		require.NoError(t, err)

		assert.Equal(t, int64(3), stats["total_scored"])
		assert.InDelta(t, 0.583, stats["avg_final_score"], 0.01)

		// check component scores if present
		if avgRule, ok := stats["avg_rule_score"]; ok {
			assert.InDelta(t, 0.7, avgRule, 0.01) // only one has rule score
		}
	})

	t.Run("update existing score", func(t *testing.T) {
		// update score for items[0]
		newScore := &ArticleScore{
			ArticleID:  items[0].ID,
			FinalScore: 0.9,
			ScoredAt:   time.Now(),
		}

		err := db.CreateArticleScore(ctx, newScore)
		require.NoError(t, err)

		// verify update
		retrieved, err := db.GetArticleScore(ctx, items[0].ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.9, retrieved.FinalScore, 0.01)
	})

	t.Run("delete old scores", func(t *testing.T) {
		// create an old score
		oldItem := &Item{
			FeedID: feed.ID,
			GUID:   "old-item",
			Title:  "Old Article",
			Link:   "https://example.com/old",
		}
		err := db.CreateItem(ctx, oldItem)
		require.NoError(t, err)

		oldScore := &ArticleScore{
			ArticleID:  oldItem.ID,
			FinalScore: 0.5,
			ScoredAt:   time.Now().Add(-48 * time.Hour), // 2 days old
		}
		err = db.CreateArticleScore(ctx, oldScore)
		require.NoError(t, err)

		// delete scores older than 1 day
		deleted, err := db.DeleteOldScores(ctx, 24*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, int64(1), deleted)

		// verify old score is gone
		retrieved, err := db.GetArticleScore(ctx, oldItem.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}
