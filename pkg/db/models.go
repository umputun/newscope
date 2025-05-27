package db

import (
	"database/sql"
	"time"
)

type (
	// NullString is a type alias for sql.NullString
	NullString = sql.NullString
	// NullTime is a type alias for sql.NullTime
	NullTime = sql.NullTime
	// NullInt64 is a type alias for sql.NullInt64
	NullInt64 = sql.NullInt64
	// NullBool is a type alias for sql.NullBool
	NullBool = sql.NullBool
	// NullFloat64 is a type alias for sql.NullFloat64
	NullFloat64 = sql.NullFloat64
)

// Feed represents an RSS feed source
type Feed struct {
	ID            int64           `db:"id"`
	URL           string          `db:"url"`
	Title         string          `db:"title"`
	Description   sql.NullString  `db:"description"`
	LastFetched   sql.NullTime    `db:"last_fetched"`
	NextFetch     sql.NullTime    `db:"next_fetch"`
	FetchInterval int             `db:"fetch_interval"` // seconds
	LastError     sql.NullString  `db:"last_error"`
	ErrorCount    int             `db:"error_count"`
	Enabled       bool            `db:"enabled"`
	Priority      int             `db:"priority"`
	AvgScore      sql.NullFloat64 `db:"avg_score"`
	Metadata      sql.NullString  `db:"metadata"` // JSON
	CreatedAt     time.Time       `db:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at"`
}

// Item represents a feed item/article
type Item struct {
	ID               int64          `db:"id"`
	FeedID           int64          `db:"feed_id"`
	GUID             string         `db:"guid"`
	Title            string         `db:"title"`
	Link             string         `db:"link"`
	Description      sql.NullString `db:"description"`
	Content          sql.NullString `db:"content"`      // plain text
	ContentHTML      sql.NullString `db:"content_html"` // HTML version
	ContentHash      sql.NullString `db:"content_hash"` // for deduplication
	Published        sql.NullTime   `db:"published"`
	Author           sql.NullString `db:"author"`
	Language         sql.NullString `db:"language"`  // extracted by trafilatura
	ReadTime         sql.NullInt64  `db:"read_time"` // estimated minutes
	MediaCount       sql.NullInt64  `db:"media_count"`
	ExtractionMethod sql.NullString `db:"extraction_method"` // trafilatura, fallback, etc
	ExtractionMode   sql.NullString `db:"extraction_mode"`   // precision, balanced, recall
	ContentExtracted bool           `db:"content_extracted"`
	FetchedAt        time.Time      `db:"fetched_at"`
	CreatedAt        time.Time      `db:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at"`
}

// Content represents extracted full content
type Content struct {
	ID              int64          `db:"id"`
	ItemID          int64          `db:"item_id"`
	FullContent     string         `db:"full_content"`
	ExtractedAt     time.Time      `db:"extracted_at"`
	ExtractionError sql.NullString `db:"extraction_error"`
}

// Category represents classification rules
type Category struct {
	ID         int64         `db:"id"`
	Name       string        `db:"name"`
	Keywords   string        `db:"keywords"` // JSON array
	IsPositive bool          `db:"is_positive"`
	Weight     float64       `db:"weight"`
	ParentID   sql.NullInt64 `db:"parent_id"`
	Active     bool          `db:"active"`
	CreatedAt  time.Time     `db:"created_at"`
	UpdatedAt  time.Time     `db:"updated_at"`
}

// ItemCategory represents the many-to-many relationship between items and categories
type ItemCategory struct {
	ItemID     int64   `db:"item_id"`
	CategoryID int64   `db:"category_id"`
	Confidence float64 `db:"confidence"`
}

// ItemWithContent represents an item with its extracted content
type ItemWithContent struct {
	Item
	FullContent     sql.NullString `db:"full_content"`
	ExtractedAt     sql.NullTime   `db:"extracted_at"`
	ExtractionError sql.NullString `db:"extraction_error"`
}

// ArticleScore represents classification scores for an article
type ArticleScore struct {
	ArticleID    int64           `db:"article_id"`
	RuleScore    sql.NullFloat64 `db:"rule_score"`
	MLScore      sql.NullFloat64 `db:"ml_score"`
	SourceScore  sql.NullFloat64 `db:"source_score"`
	RecencyScore sql.NullFloat64 `db:"recency_score"`
	FinalScore   float64         `db:"final_score"`
	Explanation  sql.NullString  `db:"explanation"` // JSON
	ScoredAt     time.Time       `db:"scored_at"`
	ModelVersion sql.NullInt64   `db:"model_version"`
}

// UserFeedback represents user feedback on articles
type UserFeedback struct {
	ID              int64         `db:"id"`
	ArticleID       int64         `db:"article_id"`
	FeedbackType    string        `db:"feedback_type"`  // interesting, boring, spam
	FeedbackValue   sql.NullInt64 `db:"feedback_value"` // 1-5 scale
	FeedbackAt      time.Time     `db:"feedback_at"`
	TimeSpent       sql.NullInt64 `db:"time_spent"` // seconds
	UsedForTraining bool          `db:"used_for_training"`
}

// UserAction represents user interactions with articles
type UserAction struct {
	ID        int64          `db:"id"`
	ArticleID int64          `db:"article_id"`
	Action    string         `db:"action"` // view, click, share, save
	ActionAt  time.Time      `db:"action_at"`
	Context   sql.NullString `db:"context"` // JSON
}

// MLModel represents stored machine learning models
type MLModel struct {
	ID            int64          `db:"id"`
	ModelType     string         `db:"model_type"`
	ModelData     []byte         `db:"model_data"`
	FeatureConfig sql.NullString `db:"feature_config"` // JSON
	TrainingStats sql.NullString `db:"training_stats"` // JSON
	SampleCount   sql.NullInt64  `db:"sample_count"`
	CreatedAt     time.Time      `db:"created_at"`
	IsActive      bool           `db:"is_active"`
}

// Setting represents system configuration
type Setting struct {
	Key       string    `db:"key"`
	Value     string    `db:"value"` // JSON
	UpdatedAt time.Time `db:"updated_at"`
}

// FeedWithStats represents a feed with statistics
type FeedWithStats struct {
	Feed
	ItemCount         int          `db:"item_count"`
	UnreadCount       int          `db:"unread_count"`
	ExtractedCount    int          `db:"extracted_count"`
	LastItemPublished sql.NullTime `db:"last_item_published"`
}
