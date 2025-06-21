package feed

import (
	"math/rand"
	"net/http"
)

// acceptLanguages contains common browser Accept-Language values
var acceptLanguages = []string{
	"en-US,en;q=0.9",
	"en-GB,en;q=0.9",
	"en-US,en;q=0.9,es;q=0.8",
	"en-US,en;q=0.9,fr;q=0.8",
	"en-US,en;q=0.9,de;q=0.8",
}

// addBrowserHeaders adds browser-like headers for feed fetching
// feeds are often fetched by browsers too, so we want to look legitimate
func addBrowserHeaders(req *http.Request) {
	// accept header for feeds - include both RSS and HTML
	req.Header.Set("Accept", "application/rss+xml,application/atom+xml,application/xml;q=0.9,text/xml;q=0.8,text/html;q=0.7,*/*;q=0.5")
	// don't request compression for feeds - simpler to handle
	req.Header.Set("Cache-Control", "no-cache")

	// randomized language
	req.Header.Set("Accept-Language", acceptLanguages[rand.Intn(len(acceptLanguages))]) //nolint:gosec // non-cryptographic randomness is fine for header variation

	// connection header
	req.Header.Set("Connection", "keep-alive")

	// dnt - 30% chance
	if rand.Float32() < 0.3 { //nolint:gosec // non-cryptographic randomness is fine
		req.Header.Set("DNT", "1")
	}
}

