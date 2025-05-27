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
)

// Feed represents an RSS feed source
type Feed struct {
	ID          int64          `db:"id"`
	URL         string         `db:"url"`
	Title       string         `db:"title"`
	Description sql.NullString `db:"description"`
	LastFetched sql.NullTime   `db:"last_fetched"`
	LastError   sql.NullString `db:"last_error"`
	ErrorCount  int            `db:"error_count"`
	Enabled     bool           `db:"enabled"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

// Item represents a feed item/article
type Item struct {
	ID               int64          `db:"id"`
	FeedID           int64          `db:"feed_id"`
	GUID             string         `db:"guid"`
	Title            string         `db:"title"`
	Link             string         `db:"link"`
	Description      sql.NullString `db:"description"`
	Published        sql.NullTime   `db:"published"`
	Author           sql.NullString `db:"author"`
	ContentExtracted bool           `db:"content_extracted"`
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

// Category represents item categories/tags
type Category struct {
	ID       int64  `db:"id"`
	ItemID   int64  `db:"item_id"`
	Category string `db:"category"`
}

// ItemWithContent represents an item with its extracted content
type ItemWithContent struct {
	Item
	FullContent     sql.NullString `db:"full_content"`
	ExtractedAt     sql.NullTime   `db:"extracted_at"`
	ExtractionError sql.NullString `db:"extraction_error"`
}

// FeedWithStats represents a feed with statistics
type FeedWithStats struct {
	Feed
	ItemCount         int          `db:"item_count"`
	UnreadCount       int          `db:"unread_count"`
	ExtractedCount    int          `db:"extracted_count"`
	LastItemPublished sql.NullTime `db:"last_item_published"`
}
