package domain

import "time"

// Feed represents a news feed source
type Feed struct {
	ID            int64
	URL           string
	Title         string
	Description   string
	LastFetched   *time.Time
	NextFetch     *time.Time
	FetchInterval int // seconds
	ErrorCount    int
	LastError     string
	Enabled       bool
	CreatedAt     time.Time
}

