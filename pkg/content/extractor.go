package content

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/markusmobius/go-trafilatura"
	"golang.org/x/net/html"
)

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

// ExtractResult contains the result of content extraction
type ExtractResult struct {
	Content     string    // extracted content as plain text
	RichContent string    // extracted content with simplified HTML formatting
	Title       string    // article title if available
	URL         string    // original URL
	Date        time.Time // publication date if available
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

	// extract rich content with simplified HTML if available
	richContent := ""
	if result.ContentNode != nil {
		richContent = extractRichContent(result.ContentNode)
	}

	// check minimum text length
	if len(content) < e.minTextLength {
		return nil, fmt.Errorf("content too short: %d chars (minimum %d)", len(content), e.minTextLength)
	}

	// build result
	extractResult := &ExtractResult{
		Content:     content,
		RichContent: richContent,
		Title:       result.Metadata.Title,
		URL:         urlStr,
	}

	// use metadata date if available
	if !result.Metadata.Date.IsZero() {
		extractResult.Date = result.Metadata.Date
	}

	return extractResult, nil
}

// extractRichContent extracts content from HTML node preserving simplified HTML structure
func extractRichContent(node *html.Node) string {
	var buf bytes.Buffer
	extractRichContentRecursive(node, &buf)

	// clean up the result
	result := buf.String()
	result = strings.TrimSpace(result)

	return result
}

// extractRichContentRecursive recursively extracts content with simplified HTML
func extractRichContentRecursive(node *html.Node, buf *bytes.Buffer) {
	if node == nil {
		return
	}

	// handle text nodes
	if node.Type == html.TextNode {
		text := strings.TrimSpace(node.Data)
		if text != "" {
			// escape HTML entities
			text = html.EscapeString(text)
			buf.WriteString(text)
			if !strings.HasSuffix(text, ".") && !strings.HasSuffix(text, "!") && !strings.HasSuffix(text, "?") {
				buf.WriteString(" ")
			}
		}
		return
	}

	// handle element nodes
	if node.Type == html.ElementNode {
		// allowed tags for rich content
		allowedTags := map[string]string{
			"p":          "p",
			"h1":         "h3", // downgrade headers for article display
			"h2":         "h4",
			"h3":         "h5",
			"h4":         "h6",
			"h5":         "h6",
			"h6":         "h6",
			"ul":         "ul",
			"ol":         "ol",
			"li":         "li",
			"blockquote": "blockquote",
			"strong":     "strong",
			"b":          "strong",
			"em":         "em",
			"i":          "em",
			"code":       "code",
			"pre":        "pre",
			"br":         "br",
		}

		outputTag, isAllowed := allowedTags[node.Data]

		if isAllowed {
			// write opening tag
			buf.WriteString("<")
			buf.WriteString(outputTag)
			buf.WriteString(">")

			// process children
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				extractRichContentRecursive(child, buf)
			}

			// write closing tag (except for self-closing tags)
			if outputTag != "br" {
				buf.WriteString("</")
				buf.WriteString(outputTag)
				buf.WriteString(">")
			}
		} else {
			// for non-allowed tags, just process children
			// but add paragraph breaks for block-level elements
			blockElements := map[string]bool{
				"div": true, "section": true, "article": true,
				"table": true, "tr": true, "td": true, "th": true,
			}

			if blockElements[node.Data] {
				buf.WriteString("<p>")
			}

			for child := node.FirstChild; child != nil; child = child.NextSibling {
				extractRichContentRecursive(child, buf)
			}

			if blockElements[node.Data] {
				buf.WriteString("</p>")
			}
		}
		return
	}

	// process other node types (document, etc.)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		extractRichContentRecursive(child, buf)
	}
}
