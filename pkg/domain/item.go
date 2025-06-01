package domain

import "time"

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

const (
	FeedbackLike    FeedbackType = "like"
	FeedbackDislike FeedbackType = "dislike"
	FeedbackSpam    FeedbackType = "spam"
)

// ClassifiedItem represents an item with all processing completed
type ClassifiedItem struct {
	*Item
	FeedName     string
	FeedURL      string
	Extraction   *ExtractedContent
	Classification *Classification
	UserFeedback *Feedback
}

// ItemFilter represents filtering criteria for items
type ItemFilter struct {
	MinScore     float64
	Topic        string
	FeedName     string
	Limit        int
	OnlyClassified bool
}

// FeedbackExample represents feedback for LLM training
type FeedbackExample struct {
	Title       string
	Description string
	Content     string
	Feedback    FeedbackType
	Topics      []string
}