package types

import "time"

// Feed represents an RSS/Atom feed
type Feed struct {
	Title       string
	Description string
	Link        string
	Items       []Item
}

// Item represents a single article from an RSS/Atom feed
type Item struct {
	FeedName    string // name of the feed this item belongs to (optional)
	GUID        string
	Title       string
	Link        string
	Description string
	Content     string // content from RSS feed (if available)
	Author      string
	Published   time.Time
}

// ItemWithContent represents an item with extracted full content
type ItemWithContent struct {
	Item
	ExtractedContent string    // extracted full article content
	ContentExtracted bool      // whether content extraction was successful
	ExtractedAt      time.Time // when extraction occurred
}

// ItemWithClassification represents an item with LLM classification data
type ItemWithClassification struct {
	Item
	ID                   int64      // database ID for actions
	FeedName             string     // name of the feed
	ExtractedContent     string     // extracted content as plain text
	ExtractedRichContent string     // extracted content with HTML formatting
	ExtractionError      string     // extraction error if any
	RelevanceScore       float64    // LLM classification score (0-10)
	Explanation          string     // LLM explanation for the score
	Topics               []string   // topics identified by LLM
	ClassifiedAt         *time.Time // when classified
	UserFeedback         string     // user feedback: like, dislike
}

// FeedInfo represents feed information for UI display
type FeedInfo struct {
	ID            int64
	URL           string
	Title         string
	Description   string
	LastFetched   *time.Time
	NextFetch     *time.Time
	FetchInterval int
	ErrorCount    int
	LastError     string
	Enabled       bool
}
