package content

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/markusmobius/go-trafilatura"
)

// ExtractResult contains the result of content extraction
type ExtractResult struct {
	Content string    // extracted content
	Title   string    // article title if available
	URL     string    // original URL
	Date    time.Time // publication date if available
}

// HTTPExtractor extracts article content from URLs using trafilatura
type HTTPExtractor struct {
	timeout       time.Duration
	userAgent     string
	fallbackURL   string
	minTextLength int
	includeImages bool
	includeLinks  bool
	client        *http.Client
}

// NewHTTPExtractor creates a new content extractor
func NewHTTPExtractor(timeout time.Duration, userAgent string) *HTTPExtractor {
	return &HTTPExtractor{
		timeout:       timeout,
		userAgent:     userAgent,
		minTextLength: 100,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// SetFallbackURL sets the fallback trafilatura API URL
func (e *HTTPExtractor) SetFallbackURL(fallbackURL string) {
	e.fallbackURL = fallbackURL
}

// SetOptions configures extraction options
func (e *HTTPExtractor) SetOptions(minTextLength int, includeImages, includeLinks bool) {
	e.minTextLength = minTextLength
	e.includeImages = includeImages
	e.includeLinks = includeLinks
}

// Extract retrieves and extracts text content from the given URL
func (e *HTTPExtractor) Extract(ctx context.Context, urlStr string) (*ExtractResult, error) {
	// validate URL
	if urlStr == "" {
		return nil, fmt.Errorf("empty URL")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid URL: %s", urlStr)
	}

	// create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// set user agent
	req.Header.Set("User-Agent", e.userAgent)

	// fetch content
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch URL %s: %w", urlStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// configure trafilatura options
	opts := trafilatura.Options{
		EnableFallback:  true,
		ExcludeComments: true,
		ExcludeTables:   false,
		IncludeImages:   e.includeImages,
		IncludeLinks:    e.includeLinks,
		Deduplicate:     true,
		OriginalURL:     parsedURL,
	}

	// extract content
	result, err := trafilatura.Extract(resp.Body, opts)
	if err != nil {
		return nil, fmt.Errorf("extract content from %s: %w", urlStr, err)
	}

	if result == nil {
		return nil, fmt.Errorf("no content extracted")
	}

	// get main content
	content := result.ContentText
	if content == "" {
		return nil, fmt.Errorf("no content extracted")
	}

	// clean up content
	content = strings.TrimSpace(content)

	// check minimum text length
	if len(content) < e.minTextLength {
		return nil, fmt.Errorf("content too short: %d chars (minimum %d)", len(content), e.minTextLength)
	}

	// build result
	extractResult := &ExtractResult{
		Content: content,
		Title:   result.Metadata.Title,
		URL:     urlStr,
	}

	// use metadata date if available
	if !result.Metadata.Date.IsZero() {
		extractResult.Date = result.Metadata.Date
	}

	return extractResult, nil
}
