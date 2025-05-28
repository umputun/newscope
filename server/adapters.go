package server

import (
	"context"
	"fmt"

	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
)

// DBAdapter adapts db.DB to server.Database interface
type DBAdapter struct {
	*db.DB
}

// GetFeeds adapts db.GetFeeds to return types.Feed
func (d *DBAdapter) GetFeeds(ctx context.Context) ([]types.Feed, error) {
	dbFeeds, err := d.DB.GetFeeds(ctx, false) // get all feeds
	if err != nil {
		return nil, err
	}

	feeds := make([]types.Feed, len(dbFeeds))
	for i, f := range dbFeeds {
		feeds[i] = types.Feed{
			Title:       f.Title,
			Description: f.Description,
			Link:        f.URL,
		}
	}
	return feeds, nil
}

// GetItems adapts db.GetItems to return types.Item
func (d *DBAdapter) GetItems(ctx context.Context, limit, _ int) ([]types.Item, error) {
	// DB uses minScore instead of offset
	// for now, return all items with score >= 0
	dbItems, err := d.DB.GetItems(ctx, limit, 0)
	if err != nil {
		return nil, err
	}

	items := make([]types.Item, len(dbItems))
	for i, item := range dbItems {
		items[i] = types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Author:      item.Author,
			Published:   item.Published,
		}
	}
	return items, nil
}

// GetClassifiedItems returns items with classification data
func (d *DBAdapter) GetClassifiedItems(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error) {
	// create a query that joins items with feeds to get feed names
	query := `
		SELECT 
			i.id, i.feed_id, i.guid, i.title, i.link, i.description,
			i.content, i.author, i.published, i.extracted_content, 
			i.extracted_rich_content, i.extracted_at, i.extraction_error, 
			i.relevance_score, i.explanation, i.topics, i.classified_at, 
			i.user_feedback, i.feedback_at, i.created_at, i.updated_at,
			f.title as feed_title
		FROM items i
		JOIN feeds f ON i.feed_id = f.id
		WHERE i.relevance_score >= ?
		AND i.classified_at IS NOT NULL
		ORDER BY i.published DESC
		LIMIT ?
	`

	rows, err := d.QueryContext(ctx, query, minScore, limit)
	if err != nil {
		return nil, fmt.Errorf("query classified items: %w", err)
	}
	defer rows.Close()

	var result []types.ItemWithClassification
	for rows.Next() {
		var item db.Item
		var feedTitle string

		err := rows.Scan(
			&item.ID, &item.FeedID, &item.GUID, &item.Title, &item.Link,
			&item.Description, &item.Content, &item.Author, &item.Published,
			&item.ExtractedContent, &item.ExtractedRichContent, &item.ExtractedAt,
			&item.ExtractionError, &item.RelevanceScore, &item.Explanation,
			&item.Topics, &item.ClassifiedAt, &item.UserFeedback, &item.FeedbackAt,
			&item.CreatedAt, &item.UpdatedAt, &feedTitle,
		)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}

		// filter by topic if specified
		if topic != "" {
			found := false
			for _, t := range item.Topics {
				if t == topic {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		result = append(result, types.ItemWithClassification{
			Item: types.Item{
				GUID:        item.GUID,
				Title:       item.Title,
				Link:        item.Link,
				Description: item.Description,
				Author:      item.Author,
				Published:   item.Published,
			},
			ID:                   item.ID,
			FeedName:             feedTitle,
			ExtractedContent:     item.ExtractedContent,
			ExtractedRichContent: item.ExtractedRichContent,
			ExtractionError:      item.ExtractionError,
			RelevanceScore:       item.RelevanceScore,
			Explanation:          item.Explanation,
			Topics:               []string(item.Topics),
			ClassifiedAt:         item.ClassifiedAt,
			UserFeedback:         item.UserFeedback,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return result, nil
}

// UpdateItemFeedback updates user feedback for an item
func (d *DBAdapter) UpdateItemFeedback(ctx context.Context, itemID int64, feedback string) error {
	return d.DB.UpdateItemFeedback(ctx, itemID, feedback)
}

// GetClassifiedItem returns a single item with classification data
func (d *DBAdapter) GetClassifiedItem(ctx context.Context, itemID int64) (*types.ItemWithClassification, error) {
	query := `
		SELECT 
			i.id, i.feed_id, i.guid, i.title, i.link, i.description,
			i.content, i.author, i.published, i.extracted_content, 
			i.extracted_rich_content, i.extracted_at, i.extraction_error, 
			i.relevance_score, i.explanation, i.topics, i.classified_at, 
			i.user_feedback, i.feedback_at, i.created_at, i.updated_at,
			f.title as feed_title
		FROM items i
		JOIN feeds f ON i.feed_id = f.id
		WHERE i.id = ?
	`

	var item db.Item
	var feedTitle string

	err := d.QueryRowContext(ctx, query, itemID).Scan(
		&item.ID, &item.FeedID, &item.GUID, &item.Title, &item.Link,
		&item.Description, &item.Content, &item.Author, &item.Published,
		&item.ExtractedContent, &item.ExtractedRichContent, &item.ExtractedAt,
		&item.ExtractionError, &item.RelevanceScore, &item.Explanation,
		&item.Topics, &item.ClassifiedAt, &item.UserFeedback, &item.FeedbackAt,
		&item.CreatedAt, &item.UpdatedAt, &feedTitle,
	)
	if err != nil {
		return nil, fmt.Errorf("get classified item: %w", err)
	}

	result := &types.ItemWithClassification{
		Item: types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Author:      item.Author,
			Published:   item.Published,
		},
		ID:                   item.ID,
		FeedName:             feedTitle,
		ExtractedContent:     item.ExtractedContent,
		ExtractedRichContent: item.ExtractedRichContent,
		ExtractionError:      item.ExtractionError,
		RelevanceScore:       item.RelevanceScore,
		Explanation:          item.Explanation,
		Topics:               []string(item.Topics),
		ClassifiedAt:         item.ClassifiedAt,
		UserFeedback:         item.UserFeedback,
	}

	return result, nil
}

// GetTopics returns all unique topics from classified items
func (d *DBAdapter) GetTopics(ctx context.Context) ([]string, error) {
	// for now, query all classified items and extract unique topics
	// in a real implementation, this could be a specific DB query
	items, err := d.DB.GetItems(ctx, 1000, 0) // get many items
	if err != nil {
		return nil, err
	}

	topicMap := make(map[string]bool)
	for _, item := range items {
		for _, topic := range item.Topics {
			topicMap[topic] = true
		}
	}

	topics := make([]string, 0, len(topicMap))
	for topic := range topicMap {
		topics = append(topics, topic)
	}

	return topics, nil
}

// GetAllFeeds returns all feeds with full details
func (d *DBAdapter) GetAllFeeds(ctx context.Context) ([]db.Feed, error) {
	return d.DB.GetFeeds(ctx, false) // get all feeds, not just enabled
}

// CreateFeed adds a new feed
func (d *DBAdapter) CreateFeed(ctx context.Context, feed *db.Feed) error {
	err := d.DB.CreateFeed(ctx, feed)
	if err != nil {
		return err
	}

	return nil
}

// UpdateFeedStatus enables or disables a feed
func (d *DBAdapter) UpdateFeedStatus(ctx context.Context, feedID int64, enabled bool) error {
	query := "UPDATE feeds SET enabled = ? WHERE id = ?"
	_, err := d.ExecContext(ctx, query, enabled, feedID)
	return err
}

// DeleteFeed removes a feed
func (d *DBAdapter) DeleteFeed(ctx context.Context, feedID int64) error {
	return d.DB.DeleteFeed(ctx, feedID)
}
