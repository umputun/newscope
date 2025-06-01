package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-pkgz/repeater/v2"
	"github.com/jmoiron/sqlx"

	"github.com/umputun/newscope/pkg/domain"
)

// ItemRepository handles item-related database operations
type ItemRepository struct {
	db *sqlx.DB
}

// itemSQL represents an item for SQL operations
type itemSQL struct {
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
	RelevanceScore float64    `db:"relevance_score"`
	Explanation    string     `db:"explanation"`
	Topics         topicsSQL  `db:"topics"`
	ClassifiedAt   *time.Time `db:"classified_at"`

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

// topicsSQL is a JSON array of topic strings for SQL operations
type topicsSQL []string

// Value implements driver.Valuer for database storage
func (t topicsSQL) Value() (driver.Value, error) {
	if t == nil {
		return "[]", nil
	}
	return json.Marshal(t)
}

// Scan implements sql.Scanner for database retrieval
func (t *topicsSQL) Scan(value interface{}) error {
	if value == nil {
		*t = topicsSQL{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return json.Unmarshal([]byte("[]"), t)
	}

	return json.Unmarshal(data, t)
}

// NewItemRepository creates a new item repository
func NewItemRepository(database *sqlx.DB) *ItemRepository {
	return &ItemRepository{db: database}
}

// CreateItem inserts a new item
func (r *ItemRepository) CreateItem(ctx context.Context, item *domain.Item) error {
	sqlItem := &itemSQL{
		FeedID:      item.FeedID,
		GUID:        item.GUID,
		Title:       item.Title,
		Link:        item.Link,
		Description: item.Description,
		Content:     item.Content,
		Author:      item.Author,
		Published:   item.Published,
	}

	query := `
		INSERT INTO items (
			feed_id, guid, title, link, description, content, 
			author, published
		) VALUES (
			:feed_id, :guid, :title, :link, :description, :content,
			:author, :published
		)
	`
	result, err := r.db.NamedExecContext(ctx, query, sqlItem)
	if err != nil {
		return fmt.Errorf("create item: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get insert id: %w", err)
	}

	item.ID = id
	return nil
}

// GetItem retrieves an item by ID
func (r *ItemRepository) GetItem(ctx context.Context, id int64) (*domain.Item, error) {
	var sqlItem itemSQL
	err := r.db.GetContext(ctx, &sqlItem, "SELECT * FROM items WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	return r.toDomainItem(&sqlItem), nil
}

// GetItems retrieves items with optional filters
func (r *ItemRepository) GetItems(ctx context.Context, limit int, minScore float64) ([]*domain.Item, error) {
	query := `
		SELECT * FROM items 
		WHERE relevance_score >= ?
		ORDER BY published DESC
		LIMIT ?
	`
	var sqlItems []itemSQL
	err := r.db.SelectContext(ctx, &sqlItems, query, minScore, limit)
	if err != nil {
		return nil, fmt.Errorf("get items: %w", err)
	}

	items := make([]*domain.Item, len(sqlItems))
	for i, item := range sqlItems {
		items[i] = r.toDomainItem(&item)
	}
	return items, nil
}

// GetUnclassifiedItems retrieves items that need classification
func (r *ItemRepository) GetUnclassifiedItems(ctx context.Context, limit int) ([]*domain.Item, error) {
	query := `
		SELECT * FROM items 
		WHERE classified_at IS NULL
		AND extracted_content != ''
		AND extraction_error = ''
		ORDER BY published DESC
		LIMIT ?
	`
	var sqlItems []itemSQL
	err := r.db.SelectContext(ctx, &sqlItems, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get unclassified items: %w", err)
	}

	items := make([]*domain.Item, len(sqlItems))
	for i, item := range sqlItems {
		items[i] = r.toDomainItem(&item)
	}
	return items, nil
}

// GetItemsNeedingExtraction retrieves items that need content extraction
func (r *ItemRepository) GetItemsNeedingExtraction(ctx context.Context, limit int) ([]*domain.Item, error) {
	query := `
		SELECT * FROM items 
		WHERE extracted_at IS NULL
		AND extraction_error = ''
		ORDER BY published DESC
		LIMIT ?
	`
	var sqlItems []itemSQL
	err := r.db.SelectContext(ctx, &sqlItems, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get items needing extraction: %w", err)
	}

	items := make([]*domain.Item, len(sqlItems))
	for i, item := range sqlItems {
		items[i] = r.toDomainItem(&item)
	}
	return items, nil
}

// UpdateItemExtraction updates item after content extraction
func (r *ItemRepository) UpdateItemExtraction(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
	var query string
	var args []interface{}

	if extraction.Error != "" {
		query = `
			UPDATE items 
			SET extraction_error = ?, extracted_at = datetime('now')
			WHERE id = ?
		`
		args = []interface{}{extraction.Error, itemID}
	} else {
		query = `
			UPDATE items 
			SET extracted_content = ?, extracted_rich_content = ?, extracted_at = datetime('now')
			WHERE id = ?
		`
		args = []interface{}{extraction.PlainText, extraction.RichHTML, itemID}
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update item extraction: %w", err)
	}
	return nil
}

// UpdateItemClassification updates item with LLM classification results
func (r *ItemRepository) UpdateItemClassification(ctx context.Context, itemID int64, classification *domain.Classification) error {
	query := `
		UPDATE items 
		SET relevance_score = ?, 
		    explanation = ?,
		    topics = ?,
		    classified_at = datetime('now')
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query, classification.Score, classification.Explanation, topicsSQL(classification.Topics), itemID)
	if err != nil {
		return fmt.Errorf("update item classification: %w", err)
	}
	return nil
}

// UpdateItemProcessed updates item with both extraction and classification results
func (r *ItemRepository) UpdateItemProcessed(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
	retrier := repeater.NewBackoff(5, 50*time.Millisecond, repeater.WithMaxDelay(2*time.Second))

	return retrier.Do(ctx, func() error {
		var query string
		var args []interface{}

		if classification.Summary != "" {
			query = `
				UPDATE items 
				SET extracted_content = ?, 
				    extracted_rich_content = ?, 
				    extracted_at = datetime('now'),
				    relevance_score = ?, 
				    explanation = ?,
				    topics = ?,
				    classified_at = datetime('now'),
				    description = ?
				WHERE id = ?
			`
			args = []interface{}{extraction.PlainText, extraction.RichHTML, classification.Score,
				classification.Explanation, topicsSQL(classification.Topics), classification.Summary, itemID}
		} else {
			query = `
				UPDATE items 
				SET extracted_content = ?, 
				    extracted_rich_content = ?, 
				    extracted_at = datetime('now'),
				    relevance_score = ?, 
				    explanation = ?,
				    topics = ?,
				    classified_at = datetime('now')
				WHERE id = ?
			`
			args = []interface{}{extraction.PlainText, extraction.RichHTML, classification.Score,
				classification.Explanation, topicsSQL(classification.Topics), itemID}
		}

		_, err := r.db.ExecContext(ctx, query, args...)
		if err != nil {
			if isLockError(err) {
				return err // repeater will retry this
			}
			return &criticalError{err: fmt.Errorf("update item processed: %w", err)}
		}

		return nil
	})
}

// ItemExists checks if an item already exists
func (r *ItemRepository) ItemExists(ctx context.Context, feedID int64, guid string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists,
		"SELECT EXISTS(SELECT 1 FROM items WHERE feed_id = ? AND guid = ?)",
		feedID, guid)
	if err != nil {
		return false, fmt.Errorf("check item exists: %w", err)
	}
	return exists, nil
}

// ItemExistsByTitleOrURL checks if an item with the same title or URL already exists in any feed
func (r *ItemRepository) ItemExistsByTitleOrURL(ctx context.Context, title, url string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists,
		"SELECT EXISTS(SELECT 1 FROM items WHERE title = ? OR link = ?)",
		title, url)
	if err != nil {
		return false, fmt.Errorf("check item exists by title or url: %w", err)
	}
	return exists, nil
}

// toDomainItem converts itemSQL to domain.Item
func (r *ItemRepository) toDomainItem(sqlItem *itemSQL) *domain.Item {
	return &domain.Item{
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
	}
}
