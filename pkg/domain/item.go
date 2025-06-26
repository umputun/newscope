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

// GetRelevanceScore returns the relevance score or 0 if not classified.
// This method is safe to call even when Classification is nil.
func (c *ClassifiedItem) GetRelevanceScore() float64 {
	if c.Classification != nil {
		return c.Classification.Score
	}
	return 0
}

// GetExplanation returns the classification explanation or empty string
func (c *ClassifiedItem) GetExplanation() string {
	if c.Classification != nil {
		return c.Classification.Explanation
	}
	return ""
}

// GetTopics returns the topics or empty slice
func (c *ClassifiedItem) GetTopics() []string {
	if c.Classification != nil {
		return c.Classification.Topics
	}
	return []string{}
}

// GetSummary returns the summary or empty string
func (c *ClassifiedItem) GetSummary() string {
	if c.Classification != nil {
		return c.Classification.Summary
	}
	return ""
}

// GetClassifiedAt returns when the item was classified or nil
func (c *ClassifiedItem) GetClassifiedAt() *time.Time {
	if c.Classification != nil {
		t := c.Classification.ClassifiedAt
		return &t
	}
	return nil
}

// GetExtractedContent returns extracted plain text or empty string
func (c *ClassifiedItem) GetExtractedContent() string {
	if c.Extraction != nil {
		return c.Extraction.PlainText
	}
	return ""
}

// GetExtractedRichContent returns extracted HTML or empty string
func (c *ClassifiedItem) GetExtractedRichContent() string {
	if c.Extraction != nil {
		return c.Extraction.RichHTML
	}
	return ""
}

// GetExtractionError returns extraction error or empty string
func (c *ClassifiedItem) GetExtractionError() string {
	if c.Extraction != nil {
		return c.Extraction.Error
	}
	return ""
}

// GetUserFeedback returns user feedback as string or empty string
func (c *ClassifiedItem) GetUserFeedback() string {
	if c.UserFeedback != nil {
		return string(c.UserFeedback.Type)
	}
	return ""
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
	Summary     string
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
