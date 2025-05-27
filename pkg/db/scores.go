package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// score-related database operations

// CreateArticleScore creates or updates an article score
func (db *DB) CreateArticleScore(ctx context.Context, score *ArticleScore) error {
	query := `
		INSERT OR REPLACE INTO article_scores (
			article_id, rule_score, ml_score, source_score, recency_score,
			final_score, explanation, scored_at, model_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var explanationJSON sql.NullString
	if score.Explanation.Valid {
		explanationJSON = score.Explanation
	}

	_, err := db.conn.ExecContext(ctx, query,
		score.ArticleID,
		score.RuleScore,
		score.MLScore,
		score.SourceScore,
		score.RecencyScore,
		score.FinalScore,
		explanationJSON,
		score.ScoredAt,
		score.ModelVersion,
	)
	if err != nil {
		return fmt.Errorf("create article score: %w", err)
	}
	return nil
}

// GetArticleScore retrieves the score for a specific article
func (db *DB) GetArticleScore(ctx context.Context, articleID int64) (*ArticleScore, error) {
	var score ArticleScore
	query := `SELECT * FROM article_scores WHERE article_id = ?`

	err := db.conn.GetContext(ctx, &score, query, articleID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get article score: %w", err)
	}
	return &score, nil
}

// GetArticleScores retrieves scores for multiple articles
func (db *DB) GetArticleScores(ctx context.Context, articleIDs []int64) ([]ArticleScore, error) {
	if len(articleIDs) == 0 {
		return []ArticleScore{}, nil
	}

	query, args, err := sqlx.In("SELECT * FROM article_scores WHERE article_id IN (?)", articleIDs)
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	query = db.conn.Rebind(query)

	var scores []ArticleScore
	err = db.conn.SelectContext(ctx, &scores, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get article scores: %w", err)
	}
	return scores, nil
}

// GetHighScoringArticles retrieves articles with scores above threshold
func (db *DB) GetHighScoringArticles(ctx context.Context, minScore float64, limit, offset int) ([]int64, error) {
	query := `
		SELECT article_id FROM article_scores 
		WHERE final_score >= ? 
		ORDER BY final_score DESC, scored_at DESC
		LIMIT ? OFFSET ?`

	var articleIDs []int64
	err := db.conn.SelectContext(ctx, &articleIDs, query, minScore, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get high scoring articles: %w", err)
	}
	return articleIDs, nil
}

// UpdateFeedAverageScore updates the average score for a feed
func (db *DB) UpdateFeedAverageScore(ctx context.Context, feedID int64) error {
	query := `
		UPDATE feeds 
		SET avg_score = (
			SELECT AVG(s.final_score)
			FROM article_scores s
			JOIN items i ON i.id = s.article_id
			WHERE i.feed_id = ?
		)
		WHERE id = ?`

	_, err := db.conn.ExecContext(ctx, query, feedID, feedID)
	if err != nil {
		return fmt.Errorf("update feed average score: %w", err)
	}
	return nil
}

// GetScoreStats returns scoring statistics
func (db *DB) GetScoreStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// get total scored articles
	var totalScored int64
	err := db.conn.GetContext(ctx, &totalScored, "SELECT COUNT(*) FROM article_scores")
	if err != nil {
		return nil, fmt.Errorf("get total scored: %w", err)
	}
	stats["total_scored"] = totalScored

	// get average scores
	query := `
		SELECT 
			AVG(final_score) as avg_final,
			AVG(rule_score) as avg_rule,
			AVG(ml_score) as avg_ml,
			AVG(source_score) as avg_source,
			AVG(recency_score) as avg_recency
		FROM article_scores`

	var avgScores struct {
		AvgFinal   sql.NullFloat64 `db:"avg_final"`
		AvgRule    sql.NullFloat64 `db:"avg_rule"`
		AvgML      sql.NullFloat64 `db:"avg_ml"`
		AvgSource  sql.NullFloat64 `db:"avg_source"`
		AvgRecency sql.NullFloat64 `db:"avg_recency"`
	}

	err = db.conn.GetContext(ctx, &avgScores, query)
	if err != nil {
		return nil, fmt.Errorf("get average scores: %w", err)
	}

	if avgScores.AvgFinal.Valid {
		stats["avg_final_score"] = avgScores.AvgFinal.Float64
	}
	if avgScores.AvgRule.Valid {
		stats["avg_rule_score"] = avgScores.AvgRule.Float64
	}
	if avgScores.AvgML.Valid {
		stats["avg_ml_score"] = avgScores.AvgML.Float64
	}
	if avgScores.AvgSource.Valid {
		stats["avg_source_score"] = avgScores.AvgSource.Float64
	}
	if avgScores.AvgRecency.Valid {
		stats["avg_recency_score"] = avgScores.AvgRecency.Float64
	}

	return stats, nil
}

// DeleteOldScores removes scores older than the specified duration
func (db *DB) DeleteOldScores(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := db.conn.ExecContext(ctx,
		"DELETE FROM article_scores WHERE scored_at < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old scores: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}
	return affected, nil
}

// ScoreExplanation is a structured explanation of why an article received its score
type ScoreExplanation struct {
	RuleMatches      []string               `json:"rule_matches,omitempty"`
	MLConfidence     float64                `json:"ml_confidence,omitempty"`
	SourceReputation float64                `json:"source_reputation,omitempty"`
	RecencyBonus     float64                `json:"recency_bonus,omitempty"`
	Details          map[string]interface{} `json:"details,omitempty"`
}

// SetScoreExplanation sets a structured explanation on an ArticleScore
func (score *ArticleScore) SetScoreExplanation(explanation ScoreExplanation) error {
	data, err := json.Marshal(explanation)
	if err != nil {
		return fmt.Errorf("marshal explanation: %w", err)
	}
	score.Explanation = sql.NullString{String: string(data), Valid: true}
	return nil
}

// GetScoreExplanation retrieves the structured explanation from an ArticleScore
func (score *ArticleScore) GetScoreExplanation() (*ScoreExplanation, error) {
	if !score.Explanation.Valid {
		return nil, nil
	}

	var explanation ScoreExplanation
	err := json.Unmarshal([]byte(score.Explanation.String), &explanation)
	if err != nil {
		return nil, fmt.Errorf("unmarshal explanation: %w", err)
	}
	return &explanation, nil
}
