package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// feedback-related database operations

// CreateUserFeedback creates or updates user feedback for an article
func (db *DB) CreateUserFeedback(ctx context.Context, feedback *UserFeedback) error {
	query := `
		INSERT OR REPLACE INTO user_feedback (
			article_id, feedback_type, feedback_value, feedback_at,
			time_spent, used_for_training
		) VALUES (?, ?, ?, ?, ?, ?)`

	_, err := db.conn.ExecContext(ctx, query,
		feedback.ArticleID,
		feedback.FeedbackType,
		feedback.FeedbackValue,
		feedback.FeedbackAt,
		feedback.TimeSpent,
		feedback.UsedForTraining,
	)
	if err != nil {
		return fmt.Errorf("create user feedback: %w", err)
	}
	return nil
}

// GetUserFeedback retrieves feedback for a specific article
func (db *DB) GetUserFeedback(ctx context.Context, articleID int64) (*UserFeedback, error) {
	var feedback UserFeedback
	query := `SELECT * FROM user_feedback WHERE article_id = ?`

	err := db.conn.GetContext(ctx, &feedback, query, articleID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user feedback: %w", err)
	}
	return &feedback, nil
}

// GetUnusedFeedback retrieves feedback that hasn't been used for training
func (db *DB) GetUnusedFeedback(ctx context.Context, limit int) ([]UserFeedback, error) {
	query := `
		SELECT * FROM user_feedback 
		WHERE used_for_training = 0 
		ORDER BY feedback_at DESC
		LIMIT ?`

	var feedbacks []UserFeedback
	err := db.conn.SelectContext(ctx, &feedbacks, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get unused feedback: %w", err)
	}
	return feedbacks, nil
}

// MarkFeedbackUsed marks feedback as used for training
func (db *DB) MarkFeedbackUsed(ctx context.Context, feedbackIDs []int64) error {
	if len(feedbackIDs) == 0 {
		return nil
	}

	query, args, err := sqlx.In(
		"UPDATE user_feedback SET used_for_training = 1 WHERE id IN (?)",
		feedbackIDs,
	)
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	query = db.conn.Rebind(query)
	_, err = db.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("mark feedback used: %w", err)
	}
	return nil
}

// GetFeedbackStats returns feedback statistics
func (db *DB) GetFeedbackStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// get feedback counts by type
	query := `
		SELECT feedback_type, COUNT(*) as count
		FROM user_feedback
		GROUP BY feedback_type`

	var typeCounts []struct {
		Type  string `db:"feedback_type"`
		Count int64  `db:"count"`
	}

	err := db.conn.SelectContext(ctx, &typeCounts, query)
	if err != nil {
		return nil, fmt.Errorf("get feedback type counts: %w", err)
	}

	feedbackByType := make(map[string]int64)
	for _, tc := range typeCounts {
		feedbackByType[tc.Type] = tc.Count
	}
	stats["feedback_by_type"] = feedbackByType

	// get average feedback value
	var avgValue sql.NullFloat64
	err = db.conn.GetContext(ctx, &avgValue,
		"SELECT AVG(feedback_value) FROM user_feedback WHERE feedback_value IS NOT NULL")
	if err != nil {
		return nil, fmt.Errorf("get average feedback value: %w", err)
	}
	if avgValue.Valid {
		stats["avg_feedback_value"] = avgValue.Float64
	}

	// get unused feedback count
	var unusedCount int64
	err = db.conn.GetContext(ctx, &unusedCount,
		"SELECT COUNT(*) FROM user_feedback WHERE used_for_training = 0")
	if err != nil {
		return nil, fmt.Errorf("get unused feedback count: %w", err)
	}
	stats["unused_feedback_count"] = unusedCount

	return stats, nil
}

// CreateUserAction records a user action on an article
func (db *DB) CreateUserAction(ctx context.Context, action *UserAction) error {
	query := `
		INSERT INTO user_actions (article_id, action, action_at, context)
		VALUES (?, ?, ?, ?)`

	_, err := db.conn.ExecContext(ctx, query,
		action.ArticleID,
		action.Action,
		action.ActionAt,
		action.Context,
	)
	if err != nil {
		return fmt.Errorf("create user action: %w", err)
	}
	return nil
}

// GetUserActions retrieves actions for a specific article
func (db *DB) GetUserActions(ctx context.Context, articleID int64) ([]UserAction, error) {
	query := `
		SELECT * FROM user_actions 
		WHERE article_id = ? 
		ORDER BY action_at DESC`

	var actions []UserAction
	err := db.conn.SelectContext(ctx, &actions, query, articleID)
	if err != nil {
		return nil, fmt.Errorf("get user actions: %w", err)
	}
	return actions, nil
}

// GetRecentActions retrieves recent user actions
func (db *DB) GetRecentActions(ctx context.Context, limit int, since time.Time) ([]UserAction, error) {
	query := `
		SELECT * FROM user_actions 
		WHERE action_at > ?
		ORDER BY action_at DESC
		LIMIT ?`

	var actions []UserAction
	err := db.conn.SelectContext(ctx, &actions, query, since, limit)
	if err != nil {
		return nil, fmt.Errorf("get recent actions: %w", err)
	}
	return actions, nil
}

// GetActionStats returns action statistics
func (db *DB) GetActionStats(ctx context.Context, since time.Time) (map[string]int64, error) {
	query := `
		SELECT action, COUNT(*) as count
		FROM user_actions
		WHERE action_at > ?
		GROUP BY action`

	var actionCounts []struct {
		Action string `db:"action"`
		Count  int64  `db:"count"`
	}

	err := db.conn.SelectContext(ctx, &actionCounts, query, since)
	if err != nil {
		return nil, fmt.Errorf("get action stats: %w", err)
	}

	stats := make(map[string]int64)
	for _, ac := range actionCounts {
		stats[ac.Action] = ac.Count
	}
	return stats, nil
}

// DeleteOldActions removes actions older than the specified duration
func (db *DB) DeleteOldActions(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := db.conn.ExecContext(ctx,
		"DELETE FROM user_actions WHERE action_at < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old actions: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}
	return affected, nil
}
