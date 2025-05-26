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

// HTTPExtractor extracts article content from URLs using trafilatura
type HTTPExtractor struct {
	timeout time.Duration
	client  *http.Client
}

// NewHTTPExtractor creates a new content extractor
func NewHTTPExtractor(timeout time.Duration) *HTTPExtractor {
	return &HTTPExtractor{
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Extract retrieves and extracts text content from the given URL
func (e *HTTPExtractor) Extract(ctx context.Context, urlStr string) (string, error) {
	// validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", fmt.Errorf("invalid URL: %s", urlStr)
	}

	// create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// set user agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Newscope/1.0)")

	// fetch content
	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch URL %s: %w", urlStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d for URL %s", resp.StatusCode, urlStr)
	}

	// configure trafilatura options
	opts := trafilatura.Options{
		EnableFallback:  true,
		ExcludeComments: true,
		ExcludeTables:   false,
		IncludeImages:   false,
		IncludeLinks:    false,
		Deduplicate:     true,
		OriginalURL:     parsedURL,
	}

	// extract content
	result, err := trafilatura.Extract(resp.Body, opts)
	if err != nil {
		return "", fmt.Errorf("extract content from %s: %w", urlStr, err)
	}

	if result == nil {
		return "", fmt.Errorf("no content extracted from %s", urlStr)
	}

	// get main content
	content := result.ContentText
	if content == "" {
		return "", fmt.Errorf("no text content extracted from %s", urlStr)
	}

	// clean up content
	content = strings.TrimSpace(content)

	return content, nil
}
