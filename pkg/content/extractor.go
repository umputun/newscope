package content

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-pkgz/repeater/v2"
	"github.com/markusmobius/go-trafilatura"
	"golang.org/x/net/html"
)

// clientError represents non-retryable client errors
type clientError struct {
	code    int
	message string
}

func (e *clientError) Error() string {
	if e.message != "" {
		return e.message
	}
	return fmt.Sprintf("client error: %d", e.code)
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

	var result *ExtractResult
	termErr := &clientError{} // terminal error for client errors

	// retry with exponential backoff for network/server errors
	err = repeater.NewBackoff(3, time.Second, repeater.WithMaxDelay(10*time.Second)).
		Do(ctx, func() error {
			// create request with context
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, http.NoBody)
			if err != nil {
				return fmt.Errorf("create request: %w", err)
			}

			// set user agent
			req.Header.Set("User-Agent", e.userAgent)

			// add browser-like headers with randomization
			addBrowserHeaders(req)

			// fetch content
			resp, err := e.client.Do(req)
			if err != nil {
				// network errors are retryable
				return fmt.Errorf("fetch URL %s: %w", urlStr, err)
			}
			defer resp.Body.Close()

			// handle status codes
			switch {
			case resp.StatusCode == http.StatusOK:
				// success, continue to extraction
			case resp.StatusCode >= 500 || resp.StatusCode == 429:
				// server errors and rate limiting are retryable
				return fmt.Errorf("server error: %d", resp.StatusCode)
			default:
				// client errors (4xx) are not retryable
				termErr.code = resp.StatusCode
				return termErr
			}

			// check content type - only process HTML/text content
			contentType := resp.Header.Get("Content-Type")
			if contentType != "" && !strings.Contains(strings.ToLower(contentType), "text/html") && 
				!strings.Contains(strings.ToLower(contentType), "application/xhtml") &&
				!strings.Contains(strings.ToLower(contentType), "text/plain") {
				// non-HTML content (PDF, images, etc) - not retryable
				termErr.code = resp.StatusCode
				termErr.message = fmt.Sprintf("unsupported content type: %s", contentType)
				return termErr
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
			extracted, err := trafilatura.Extract(resp.Body, opts)
			if err != nil {
				return fmt.Errorf("extract content: %w", err)
			}

			if extracted == nil || extracted.ContentText == "" {
				return fmt.Errorf("no content extracted")
			}

			// clean up content
			content := strings.TrimSpace(extracted.ContentText)

			// check minimum text length
			if len(content) < e.minTextLength {
				// too short content is not retryable
				return fmt.Errorf("content too short: %d chars", len(content))
			}

			// extract rich content with simplified HTML if available
			richContent := ""
			if extracted.ContentNode != nil {
				richContent = extractRichContent(extracted.ContentNode)
			}

			// build result
			result = &ExtractResult{
				Content:     content,
				RichContent: richContent,
				Title:       extracted.Metadata.Title,
				URL:         urlStr,
			}

			// use metadata date if available
			if !extracted.Metadata.Date.IsZero() {
				result.Date = extracted.Metadata.Date
			}

			return nil
		}, termErr) // pass termErr to stop on client errors

	if err != nil {
		return nil, err
	}

	return result, nil
}

// extractRichContent extracts content from HTML node preserving simplified HTML structure
func extractRichContent(node *html.Node) string {
	var buf bytes.Buffer
	extractRichContentRecursive(node, &buf)

	// clean up the result
	result := buf.String()
	result = strings.TrimSpace(result)

	// remove any leading empty paragraphs
	for strings.HasPrefix(result, "<p></p>") {
		result = strings.TrimPrefix(result, "<p></p>")
		result = strings.TrimSpace(result)
	}

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
		handleElementNode(node, buf)
		return
	}

	// process other node types (document, etc.)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		extractRichContentRecursive(child, buf)
	}
}

// handleElementNode processes HTML element nodes
func handleElementNode(node *html.Node, buf *bytes.Buffer) {
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
		return
	}

	// for non-allowed tags, just process children
	// but add paragraph breaks for block-level elements
	blockElements := map[string]bool{
		"div": true, "section": true, "article": true,
		"table": true, "tr": true, "td": true, "th": true,
	}

	if !blockElements[node.Data] {
		// for non-block elements, process children normally
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			extractRichContentRecursive(child, buf)
		}
		return
	}

	// temporarily buffer the content to check if it's empty
	var tempBuf bytes.Buffer
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		extractRichContentRecursive(child, &tempBuf)
	}

	// only wrap in <p> tags if there's actual content
	content := strings.TrimSpace(tempBuf.String())
	if content != "" {
		buf.WriteString("<p>")
		buf.WriteString(content)
		buf.WriteString("</p>")
	}
}
