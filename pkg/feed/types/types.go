package types

import "time"

// FeedItem represents a single article from an RSS/Atom feed
type FeedItem struct {
	FeedName    string
	Title       string
	URL         string
	Description string
	Content     string // content from RSS feed
	Published   time.Time
	GUID        string
}

// ExtractedItem represents a feed item with extracted full content
type ExtractedItem struct {
	FeedItem
	FullContent      string    // extracted full article content
	ContentExtracted bool      // whether content extraction was attempted
	ExtractedAt      time.Time // when extraction occurred
}
