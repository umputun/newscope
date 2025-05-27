package db

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// Feed represents an RSS feed source
type Feed struct {
	ID            int64      `db:"id"`
	URL           string     `db:"url"`
	Title         string     `db:"title"`
	Description   string     `db:"description"`
	LastFetched   *time.Time `db:"last_fetched"`
	NextFetch     *time.Time `db:"next_fetch"`
	FetchInterval int        `db:"fetch_interval"` // seconds
	ErrorCount    int        `db:"error_count"`
	LastError     string     `db:"last_error"`
	Enabled       bool       `db:"enabled"`
	CreatedAt     time.Time  `db:"created_at"`
}

// Item represents a feed article with LLM classification
type Item struct {
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
	ExtractedContent string     `db:"extracted_content"`
	ExtractedAt      *time.Time `db:"extracted_at"`
	ExtractionError  string     `db:"extraction_error"`

	// LLM classification
	RelevanceScore float64    `db:"relevance_score"`
	Explanation    string     `db:"explanation"`
	Topics         Topics     `db:"topics"`
	ClassifiedAt   *time.Time `db:"classified_at"`

	// user feedback
	UserFeedback string     `db:"user_feedback"` // 'like', 'dislike', 'spam'
	FeedbackAt   *time.Time `db:"feedback_at"`

	// metadata
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// Setting represents a key-value configuration
type Setting struct {
	Key       string    `db:"key"`
	Value     string    `db:"value"`
	UpdatedAt time.Time `db:"updated_at"`
}

// Topics is a JSON array of topic strings
type Topics []string

// Value implements driver.Valuer for database storage
func (t Topics) Value() (driver.Value, error) {
	if t == nil {
		return "[]", nil
	}
	return json.Marshal(t)
}

// Scan implements sql.Scanner for database retrieval
func (t *Topics) Scan(value interface{}) error {
	if value == nil {
		*t = Topics{}
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

// Classification result from LLM
type Classification struct {
	GUID        string   `json:"guid"`
	Score       float64  `json:"score"`
	Explanation string   `json:"explanation"`
	Topics      []string `json:"topics"`
}

// FeedbackExample for LLM prompt context
type FeedbackExample struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Content     string   `json:"content,omitempty"`
	Feedback    string   `json:"feedback"`
	Topics      []string `json:"topics,omitempty"`
}
