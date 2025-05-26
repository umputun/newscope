package types

import "time"

// Item represents a single article from a feed
type Item struct {
	FeedName    string
	Title       string
	URL         string
	Description string
	Content     string
	Published   time.Time
	GUID        string
}