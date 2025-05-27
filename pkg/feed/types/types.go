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
