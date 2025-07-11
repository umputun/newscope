package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/newscope/pkg/domain"
)

// ClassificationRepository handles classification-related database operations
type ClassificationRepository struct {
	db *sqlx.DB
}

// itemWithFeedSQL represents an item with feed information for SQL operations
type itemWithFeedSQL struct {
	ID          int64     `db:"id"`
	FeedID      int64     `db:"feed_id"`
	GUID        string    `db:"guid"`
	Title       string    `db:"title"`
	Link        string    `db:"link"`
	Description string    `db:"description"`
	Content     string    `db:"content"`
	Author      string    `db:"author"`
	Published   time.Time `db:"published"`

	// extracted content
	ExtractedContent     string     `db:"extracted_content"`
	ExtractedRichContent string     `db:"extracted_rich_content"`
	ExtractedAt          *time.Time `db:"extracted_at"`
	ExtractionError      string     `db:"extraction_error"`

	// LLM classification
	RelevanceScore float64           `db:"relevance_score"`
	Explanation    string            `db:"explanation"`
	Topics         classificationSQL `db:"topics"`
	Summary        string            `db:"summary"`
	ClassifiedAt   *time.Time        `db:"classified_at"`

	// user feedback
	UserFeedback string     `db:"user_feedback"`
	FeedbackAt   *time.Time `db:"feedback_at"`

	// metadata
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	// joined data (not stored in DB, populated by queries)
	FeedTitle string `db:"feed_title"`
	FeedURL   string `db:"feed_url"`
}

// classificationSQL is a JSON array of topic strings for SQL operations
type classificationSQL []string

// Value implements driver.Valuer for database storage
func (c classificationSQL) Value() (driver.Value, error) {
	if c == nil {
		return "[]", nil
	}
	return json.Marshal(c)
}

// Scan implements sql.Scanner for database retrieval
func (c *classificationSQL) Scan(value interface{}) error {
	if value == nil {
		*c = classificationSQL{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return json.Unmarshal([]byte("[]"), c)
	}

	return json.Unmarshal(data, c)
}

// NewClassificationRepository creates a new classification repository
func NewClassificationRepository(database *sqlx.DB) *ClassificationRepository {
	return &ClassificationRepository{db: database}
}

// GetClassifiedItems returns classified items with feed information
func (r *ClassificationRepository) GetClassifiedItems(ctx context.Context, filter *domain.ItemFilter) ([]*domain.ClassifiedItem, error) {
	query := `
		SELECT 
			i.*,
			f.title as feed_title,
			f.url as feed_url
		FROM items i
		JOIN feeds f ON i.feed_id = f.id
		WHERE i.relevance_score >= ?
		AND i.classified_at IS NOT NULL`

	args := []interface{}{filter.MinScore}

	// add topic filter if specified
	if filter.Topic != "" {
		query += ` AND EXISTS (
			SELECT 1 FROM json_each(i.topics) 
			WHERE json_each.value = ?
		)`
		args = append(args, filter.Topic)
	}

	// add feed filter if specified
	if filter.FeedName != "" {
		query += ` AND (f.title = ? OR f.title = '' AND ? LIKE '%' || REPLACE(REPLACE(SUBSTR(f.url, INSTR(f.url, '://') + 3), 'www.', ''), '/', '') || '%')`
		args = append(args, filter.FeedName, filter.FeedName)
	}

	// add liked only filter if specified
	if filter.ShowLikedOnly {
		query += ` AND i.user_feedback = 'like'`
	}

	// add sorting
	switch filter.SortBy {
	case "score":
		query += ` ORDER BY i.relevance_score DESC, i.published DESC`
	case "source+date":
		query += ` ORDER BY f.title ASC, i.published DESC`
	case "source+score":
		query += ` ORDER BY f.title ASC, i.relevance_score DESC, i.published DESC`
	default:
		query += ` ORDER BY i.published DESC`
	}

	// add pagination
	if filter.Offset > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, filter.Limit, filter.Offset)
	} else {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	var sqlItems []itemWithFeedSQL
	if err := r.db.SelectContext(ctx, &sqlItems, query, args...); err != nil {
		return nil, fmt.Errorf("get classified items: %w", err)
	}

	items := make([]*domain.ClassifiedItem, len(sqlItems))
	for i, sqlItem := range sqlItems {
		items[i] = r.toDomainClassifiedItem(&sqlItem)
	}
	return items, nil
}

// GetClassifiedItem returns a single classified item with feed information
func (r *ClassificationRepository) GetClassifiedItem(ctx context.Context, itemID int64) (*domain.ClassifiedItem, error) {
	query := `
		SELECT 
			i.*,
			f.title as feed_title,
			f.url as feed_url
		FROM items i
		JOIN feeds f ON i.feed_id = f.id
		WHERE i.id = ?
	`

	var sqlItem itemWithFeedSQL
	if err := r.db.GetContext(ctx, &sqlItem, query, itemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("item not found")
		}
		return nil, fmt.Errorf("get classified item: %w", err)
	}
	return r.toDomainClassifiedItem(&sqlItem), nil
}

// GetTopics returns all unique topics from classified items
func (r *ClassificationRepository) GetTopics(ctx context.Context) ([]string, error) {
	return r.GetTopicsFiltered(ctx, 0.0)
}

// GetTopicsFiltered returns unique topics from items with score >= minScore
func (r *ClassificationRepository) GetTopicsFiltered(ctx context.Context, minScore float64) ([]string, error) {
	// optimized query that can use the JSON index
	query := `
		WITH filtered_items AS (
			SELECT topics
			FROM items
			WHERE classified_at IS NOT NULL
			AND relevance_score >= ?
			AND topics != '[]'
		)
		SELECT DISTINCT value 
		FROM filtered_items, json_each(filtered_items.topics)
		ORDER BY value
	`

	var topics []string
	if err := r.db.SelectContext(ctx, &topics, query, minScore); err != nil {
		return nil, fmt.Errorf("get topics filtered: %w", err)
	}
	return topics, nil
}

// TopicWithScore represents a topic with its average relevance score
type TopicWithScore struct {
	Topic     string  `db:"topic"`
	AvgScore  float64 `db:"avg_score"`
	ItemCount int     `db:"item_count"`
}

// GetTopTopicsByScore returns topics ordered by average relevance score (highest first)
func (r *ClassificationRepository) GetTopTopicsByScore(ctx context.Context, minScore float64, limit int) ([]TopicWithScore, error) {
	// optimized query using CTE for better performance
	query := `
		WITH filtered_items AS (
			SELECT topics, relevance_score
			FROM items
			WHERE classified_at IS NOT NULL
			AND relevance_score >= ?
			AND topics != '[]'
		),
		topic_scores AS (
			SELECT 
				json_each.value as topic,
				filtered_items.relevance_score
			FROM filtered_items, json_each(filtered_items.topics)
		)
		SELECT 
			topic,
			AVG(relevance_score) as avg_score,
			COUNT(*) as item_count
		FROM topic_scores
		GROUP BY topic
		ORDER BY avg_score DESC, item_count DESC
		LIMIT ?
	`

	var topics []TopicWithScore
	if err := r.db.SelectContext(ctx, &topics, query, minScore, limit); err != nil {
		return nil, fmt.Errorf("get top topics by score: %w", err)
	}
	return topics, nil
}

// UpdateItemFeedback updates user feedback on an item and adjusts its score
func (r *ClassificationRepository) UpdateItemFeedback(ctx context.Context, itemID int64, feedback *domain.Feedback) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// update feedback
	query := `
		UPDATE items 
		SET user_feedback = ?, feedback_at = datetime('now')
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, query, string(feedback.Type), itemID); err != nil {
		return fmt.Errorf("update item feedback: %w", err)
	}

	// adjust score based on feedback
	var scoreAdjustment float64
	switch feedback.Type {
	case domain.FeedbackLike:
		scoreAdjustment = 1.0 // increase score by 1
	case domain.FeedbackDislike:
		scoreAdjustment = -2.0 // decrease score by 2 (stronger signal)
	default:
		// no score adjustment for other feedback types
		return tx.Commit()
	}

	// update score, ensuring it stays within 0-10 range
	scoreQuery := `
		UPDATE items 
		SET relevance_score = MAX(0, MIN(10, relevance_score + ?))
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, scoreQuery, scoreAdjustment, itemID); err != nil {
		return fmt.Errorf("update item score: %w", err)
	}

	return tx.Commit()
}

// GetRecentFeedback retrieves recent user feedback for LLM context
func (r *ClassificationRepository) GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
	var query string
	var args []interface{}

	if feedbackType == "" {
		// get both likes and dislikes
		query = `
			SELECT title, description, 
			       SUBSTR(extracted_content, 1, 500) as content,
			       summary,
			       user_feedback as feedback, 
			       topics
			FROM items 
			WHERE user_feedback IN ('like', 'dislike')
			AND feedback_at IS NOT NULL
			ORDER BY feedback_at DESC
			LIMIT ?
		`
		args = []interface{}{limit}
	} else {
		// get specific feedback type
		query = `
			SELECT title, description, 
			       SUBSTR(extracted_content, 1, 500) as content,
			       summary,
			       user_feedback as feedback, 
			       topics
			FROM items 
			WHERE user_feedback = ?
			AND feedback_at IS NOT NULL
			ORDER BY feedback_at DESC
			LIMIT ?
		`
		args = []interface{}{feedbackType, limit}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query recent feedback: %w", err)
	}
	defer rows.Close()

	var examples []domain.FeedbackExample
	for rows.Next() {
		var example domain.FeedbackExample
		var topics classificationSQL
		var feedbackStr string
		err := rows.Scan(&example.Title, &example.Description, &example.Content, &example.Summary, &feedbackStr, &topics)
		if err != nil {
			return nil, fmt.Errorf("scan feedback row: %w", err)
		}
		example.Feedback = domain.FeedbackType(feedbackStr)
		example.Topics = []string(topics)
		examples = append(examples, example)
	}

	return examples, nil
}

// GetFeedbackCount returns the total number of feedback items (likes and dislikes)
func (r *ClassificationRepository) GetFeedbackCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM items WHERE user_feedback IN ('like', 'dislike')")
	if err != nil {
		return 0, fmt.Errorf("get feedback count: %w", err)
	}
	return count, nil
}

// GetFeedbackSince retrieves feedback items after a certain count offset
func (r *ClassificationRepository) GetFeedbackSince(ctx context.Context, offset int64, limit int) ([]domain.FeedbackExample, error) {
	query := `
		SELECT title, description, 
		       SUBSTR(extracted_content, 1, 500) as content,
		       summary,
		       user_feedback as feedback, 
		       topics
		FROM items 
		WHERE user_feedback IN ('like', 'dislike')
		AND feedback_at IS NOT NULL
		ORDER BY feedback_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query feedback since offset: %w", err)
	}
	defer rows.Close()

	var examples []domain.FeedbackExample
	for rows.Next() {
		var example domain.FeedbackExample
		var topics classificationSQL
		var feedbackStr string
		err := rows.Scan(&example.Title, &example.Description, &example.Content, &example.Summary, &feedbackStr, &topics)
		if err != nil {
			return nil, fmt.Errorf("scan feedback row: %w", err)
		}
		example.Feedback = domain.FeedbackType(feedbackStr)
		example.Topics = []string(topics)
		examples = append(examples, example)
	}

	return examples, nil
}

// toDomainClassifiedItem converts itemWithFeedSQL to domain.ClassifiedItem
func (r *ClassificationRepository) toDomainClassifiedItem(sqlItem *itemWithFeedSQL) *domain.ClassifiedItem {
	item := &domain.ClassifiedItem{
		Item: &domain.Item{
			ID:          sqlItem.ID,
			FeedID:      sqlItem.FeedID,
			GUID:        sqlItem.GUID,
			Title:       sqlItem.Title,
			Link:        sqlItem.Link,
			Description: sqlItem.Description,
			Content:     sqlItem.Content,
			Author:      sqlItem.Author,
			Published:   sqlItem.Published,
			CreatedAt:   sqlItem.CreatedAt,
			UpdatedAt:   sqlItem.UpdatedAt,
		},
		FeedName: sqlItem.FeedTitle,
		FeedURL:  sqlItem.FeedURL,
	}

	// add extraction if available
	if sqlItem.ExtractedAt != nil {
		item.Extraction = &domain.ExtractedContent{
			PlainText:   sqlItem.ExtractedContent,
			RichHTML:    sqlItem.ExtractedRichContent,
			ExtractedAt: *sqlItem.ExtractedAt,
			Error:       sqlItem.ExtractionError,
		}
	}

	// add classification if available
	if sqlItem.ClassifiedAt != nil {
		item.Classification = &domain.Classification{
			Score:        sqlItem.RelevanceScore,
			Explanation:  sqlItem.Explanation,
			Topics:       []string(sqlItem.Topics),
			Summary:      sqlItem.Summary,
			ClassifiedAt: *sqlItem.ClassifiedAt,
		}
	}

	// add feedback if available
	if sqlItem.UserFeedback != "" {
		item.UserFeedback = &domain.Feedback{
			Type:      domain.FeedbackType(sqlItem.UserFeedback),
			Timestamp: *sqlItem.FeedbackAt,
		}
	}

	return item
}

// GetClassifiedItemsCount returns the total count of classified items matching the filter
func (r *ClassificationRepository) GetClassifiedItemsCount(ctx context.Context, filter *domain.ItemFilter) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM items i
		JOIN feeds f ON i.feed_id = f.id
		WHERE i.relevance_score >= ?
		AND i.classified_at IS NOT NULL`

	args := []interface{}{filter.MinScore}

	// add topic filter if specified
	if filter.Topic != "" {
		query += ` AND EXISTS (
			SELECT 1 FROM json_each(i.topics) 
			WHERE json_each.value = ?
		)`
		args = append(args, filter.Topic)
	}

	// add feed filter if specified
	if filter.FeedName != "" {
		query += ` AND (f.title = ? OR f.title = '' AND ? LIKE '%' || REPLACE(REPLACE(SUBSTR(f.url, INSTR(f.url, '://') + 3), 'www.', ''), '/', '') || '%')`
		args = append(args, filter.FeedName, filter.FeedName)
	}

	// add liked only filter if specified
	if filter.ShowLikedOnly {
		query += ` AND i.user_feedback = 'like'`
	}

	var count int
	if err := r.db.GetContext(ctx, &count, query, args...); err != nil {
		return 0, fmt.Errorf("get classified items count: %w", err)
	}

	return count, nil
}

// buildSearchWhereClause builds the common WHERE clause for search queries
func (r *ClassificationRepository) buildSearchWhereClause(searchQuery string, filter *domain.ItemFilter) (whereClause string, args []interface{}) {
	// sanitize search query for FTS5 - escape double quotes but allow other operators
	// this allows OR, AND, NOT operators while preventing injection via quotes
	sanitizedQuery := strings.ReplaceAll(searchQuery, `"`, `""`)

	// for simple single-word queries, use a hybrid approach:
	// try FTS5 match first, then fallback to LIKE for substring matching
	isSingleWord := !strings.Contains(sanitizedQuery, " ") &&
		!strings.Contains(sanitizedQuery, "OR") &&
		!strings.Contains(sanitizedQuery, "AND") &&
		!strings.Contains(sanitizedQuery, "NOT") &&
		!strings.Contains(sanitizedQuery, "*") &&
		!strings.Contains(sanitizedQuery, "\"")

	if isSingleWord {
		// for single words, use LIKE for substring matching
		// this allows finding "GPT" within "ChatGPT"
		whereClause = `
			FROM items i
			JOIN feeds f ON i.feed_id = f.id
			WHERE (
				i.title LIKE ? OR 
				i.description LIKE ? OR 
				i.content LIKE ? OR
				i.extracted_content LIKE ? OR
				i.summary LIKE ?
			)
			AND i.classified_at IS NOT NULL`

		likePattern := "%" + sanitizedQuery + "%"
		args = []interface{}{likePattern, likePattern, likePattern, likePattern, likePattern}
	} else {
		// use FTS5 for complex queries
		whereClause = `
			FROM items i
			JOIN feeds f ON i.feed_id = f.id
			JOIN items_fts ON items_fts.rowid = i.id
			WHERE items_fts MATCH ?
			AND i.classified_at IS NOT NULL`

		args = []interface{}{sanitizedQuery}
	}

	// add score filter
	if filter.MinScore > 0 {
		whereClause += ` AND i.relevance_score >= ?`
		args = append(args, filter.MinScore)
	}

	// add topic filter if specified
	if filter.Topic != "" {
		whereClause += ` AND EXISTS (
			SELECT 1 FROM json_each(i.topics) 
			WHERE json_each.value = ?
		)`
		args = append(args, filter.Topic)
	}

	// add feed filter if specified
	if filter.FeedName != "" {
		whereClause += ` AND (f.title = ? OR f.title = '' AND ? LIKE '%' || REPLACE(REPLACE(SUBSTR(f.url, INSTR(f.url, '://') + 3), 'www.', ''), '/', '') || '%')`
		args = append(args, filter.FeedName, filter.FeedName)
	}

	// add liked only filter if specified
	if filter.ShowLikedOnly {
		whereClause += ` AND i.user_feedback = 'like'`
	}

	return whereClause, args
}

// SearchItems searches for items using full-text search
func (r *ClassificationRepository) SearchItems(ctx context.Context, searchQuery string, filter *domain.ItemFilter) ([]*domain.ClassifiedItem, error) {
	// build the common WHERE clause
	where, args := r.buildSearchWhereClause(searchQuery, filter)

	// build the full query
	query := `
		SELECT 
			i.*,
			f.title as feed_title,
			f.url as feed_url` + where

	// add sorting
	switch filter.SortBy {
	case "score":
		query += ` ORDER BY i.relevance_score DESC, i.published DESC`
	case "source+date":
		query += ` ORDER BY f.title ASC, i.published DESC`
	case "source+score":
		query += ` ORDER BY f.title ASC, i.relevance_score DESC, i.published DESC`
	case "published":
		query += ` ORDER BY i.published DESC`
	default:
		// default to date sorting for everything
		query += ` ORDER BY i.published DESC`
	}

	// add pagination
	if filter.Offset > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, filter.Limit, filter.Offset)
	} else {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	var sqlItems []itemWithFeedSQL
	if err := r.db.SelectContext(ctx, &sqlItems, query, args...); err != nil {
		return nil, fmt.Errorf("search items: %w", err)
	}

	items := make([]*domain.ClassifiedItem, len(sqlItems))
	for i, sqlItem := range sqlItems {
		items[i] = r.toDomainClassifiedItem(&sqlItem)
	}
	return items, nil
}

// GetSearchItemsCount returns the total count of items matching the search query
func (r *ClassificationRepository) GetSearchItemsCount(ctx context.Context, searchQuery string, filter *domain.ItemFilter) (int, error) {
	// build the common WHERE clause
	where, args := r.buildSearchWhereClause(searchQuery, filter)

	// build the count query
	query := "SELECT COUNT(*)" + where

	var count int
	if err := r.db.GetContext(ctx, &count, query, args...); err != nil {
		return 0, fmt.Errorf("get search items count: %w", err)
	}

	return count, nil
}
