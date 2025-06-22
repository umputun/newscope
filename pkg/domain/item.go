package domain

import "time"

// TopicWithScore represents a topic with its statistics
type TopicWithScore struct {
	Topic     string
	AvgScore  float64
	ItemCount int
}

// Item represents a core news article/item
type Item struct {
	ID          int64
	FeedID      int64
	GUID        string
	Title       string
	Link        string
	Description string
	Content     string
	Author      string
	Published   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ExtractedContent represents extracted article content
type ExtractedContent struct {
	PlainText   string
	RichHTML    string
	ExtractedAt time.Time
	Error       string
}

// Classification represents LLM classification results
type Classification struct {
	GUID         string
	Score        float64
	Explanation  string
	Topics       []string
	Summary      string
	ClassifiedAt time.Time
}

// Feedback represents user feedback on an item
type Feedback struct {
	Type      FeedbackType
	Timestamp time.Time
}

// FeedbackType represents the type of user feedback
type FeedbackType string

// feedback types
const (
	FeedbackLike    FeedbackType = "like"
	FeedbackDislike FeedbackType = "dislike"
)

// ClassifiedItem represents an item with all processing completed
type ClassifiedItem struct {
	*Item
	FeedName       string
	FeedURL        string
	Extraction     *ExtractedContent
	Classification *Classification
	UserFeedback   *Feedback
}

// ItemFilter represents filtering criteria for items
type ItemFilter struct {
	MinScore       float64
	Topic          string
	FeedName       string
	SortBy         string
	Limit          int
	Offset         int
	OnlyClassified bool
	ShowLikedOnly  bool
}

// ArticlesRequest holds parameters for fetching articles
type ArticlesRequest struct {
	MinScore      float64
	Topic         string
	FeedName      string
	SortBy        string
	Limit         int
	Page          int
	ShowLikedOnly bool
}

// PaginatedResponse represents a paginated response with metadata
type PaginatedResponse struct {
	Items       []ClassifiedItem `json:"items"`
	TotalCount  int              `json:"total_count"`
	CurrentPage int              `json:"current_page"`
	PageSize    int              `json:"page_size"`
	TotalPages  int              `json:"total_pages"`
	HasNext     bool             `json:"has_next"`
	HasPrev     bool             `json:"has_prev"`
}

// FeedbackExample represents feedback for LLM training
type FeedbackExample struct {
	Title       string
	Description string
	Content     string
	Feedback    FeedbackType
	Topics      []string
}

// ParsedFeed represents a feed parsed from RSS/Atom (before database storage)
type ParsedFeed struct {
	Title       string
	Description string
	Link        string
	Items       []ParsedItem
}

// ParsedItem represents an item parsed from RSS/Atom (before database storage)
type ParsedItem struct {
	FeedName    string // name of the feed this item belongs to (optional)
	GUID        string
	Title       string
	Link        string
	Description string
	Content     string // content from RSS feed (if available)
	Author      string
	Published   time.Time
}

// ItemWithClassification represents an item with all processing data for UI display
type ItemWithClassification struct {
	ID                   int64  // database ID for actions
	FeedID               int64  // feed ID
	FeedName             string // name of the feed
	GUID                 string
	Title                string
	Link                 string
	Description          string
	Content              string
	Author               string
	Published            time.Time
	ExtractedContent     string     // extracted content as plain text
	ExtractedRichContent string     // extracted content with HTML formatting
	ExtractionError      string     // extraction error if any
	RelevanceScore       float64    // LLM classification score (0-10)
	Explanation          string     // LLM explanation for the score
	Topics               []string   // topics identified by LLM
	Summary              string     // AI-generated summary
	ClassifiedAt         *time.Time // when classified
	UserFeedback         string     // user feedback: like, dislike
}
