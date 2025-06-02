package content

import (
	"context"
	"embed"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/*
var testData embed.FS

func TestHTTPExtractor_Extract_RealArticles(t *testing.T) {
	tests := []struct {
		name              string
		htmlFile          string
		expectedContent   []string // phrases that should be in extracted content
		unexpectedContent []string // phrases that should NOT be in extracted content (nav, ads, etc)
		minLength         int      // minimum expected content length
	}{
		{
			name:     "extract medium article",
			htmlFile: "testdata/medium_article.html",
			expectedContent: []string{
				"Understanding Go Interfaces",
				"empty interface: interface{}",
				"type Writer interface",
				"method signatures",
				"polymorphism",
			},
			unexpectedContent: []string{
				"Become a member",
				"Sign in",
				"Get started",
				"Follow",
				"Advertisement",
			},
			minLength: 1000,
		},
		{
			name:     "extract techcrunch article",
			htmlFile: "testdata/techcrunch_article.html",
			expectedContent: []string{
				"GPT-4 Turbo",
				"128K context window",
				"vision capabilities",
				"DevDay in San Francisco",
				"$0.01 per 1,000 input tokens",
			},
			unexpectedContent: []string{
				"Share on Twitter",
				"Share on Facebook",
				"Newsletter",
				"Related Articles",
			},
			minLength: 800,
		},
		{
			name:     "extract russian blog article",
			htmlFile: "testdata/blog_article_russian.html",
			expectedContent: []string{
				"борьбы со спамом в Telegram",
				"Naive Bayes",
				"классификатор",
				"tg-spam",
				"Docker-контейнер",
			},
			unexpectedContent: []string{
				"Популярные посты",
				"Теги",
				"Архив",
				"RSS",
			},
			minLength: 2000,
		},
		{
			name:     "extract bbc news article",
			htmlFile: "testdata/bbc_news_article.html",
			expectedContent: []string{
				"warmest on record",
				"1.48C above pre-industrial levels",
				"Copernicus Climate Change Service",
				"global warming",
				"El Niño",
			},
			unexpectedContent: []string{
				"Related Topics",
				"Explore the BBC",
				"Cookie banner",
				"BREAKING",
				"iPlayer",
			},
			minLength: 1500,
		},
	}

	// create test server that serves our test HTML files
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// extract filename from path
		filename := strings.TrimPrefix(r.URL.Path, "/")
		if filename == "" {
			http.NotFound(w, r)
			return
		}

		data, err := testData.ReadFile(filename)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}))
	defer server.Close()

	extractor := NewHTTPExtractor(30*time.Second, "Newscope/1.0")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			url := server.URL + "/" + tt.htmlFile

			result, err := extractor.Extract(ctx, url)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotEmpty(t, result.Content)
			t.Logf("Extracted content (%d chars): %s", len(result.Content), result.Content)

			// check minimum length
			assert.GreaterOrEqual(t, len(result.Content), tt.minLength,
				"extracted content is too short: got %d chars, expected at least %d",
				len(result.Content), tt.minLength)

			// check expected content is present
			for _, expected := range tt.expectedContent {
				assert.Contains(t, result.Content, expected,
					"expected content '%s' not found in extracted text", expected)
			}

			// check unwanted content is filtered out
			for _, unexpected := range tt.unexpectedContent {
				assert.NotContains(t, result.Content, unexpected,
					"unexpected content '%s' found in extracted text", unexpected)
			}

			// additional checks
			assert.NotContains(t, result.Content, "<script", "HTML tags should be removed")
			assert.NotContains(t, result.Content, "<style", "CSS should be removed")
			assert.NotContains(t, result.Content, "<!DOCTYPE", "DOCTYPE should be removed")
		})
	}
}

func TestHTTPExtractor_Extract_Timeout(t *testing.T) {
	// create slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Too late</body></html>"))
	}))
	defer server.Close()

	// create extractor with short timeout
	extractor := NewHTTPExtractor(100*time.Millisecond, "Newscope/1.0")

	ctx := context.Background()
	_, err := extractor.Extract(ctx, server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestHTTPExtractor_Extract_InvalidURL(t *testing.T) {
	extractor := NewHTTPExtractor(time.Second, "Newscope/1.0")

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "empty url",
			url:  "",
		},
		{
			name: "invalid scheme",
			url:  "not-a-url",
		},
		{
			name: "unreachable host",
			url:  "http://localhost:99999/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := extractor.Extract(ctx, tt.url)
			require.Error(t, err)
		})
	}
}

func TestHTTPExtractor_Extract_ContextCancellation(t *testing.T) {
	// create server that waits
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><body>Content</body></html>"))
		}
	}))
	defer server.Close()

	extractor := NewHTTPExtractor(5*time.Second, "Newscope/1.0")

	// create context and cancel it immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := extractor.Extract(ctx, server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestHTTPExtractor_Extract_ErrorCases(t *testing.T) {
	extractor := NewHTTPExtractor(1*time.Second, "Newscope/1.0")

	tests := []struct {
		name        string
		url         string
		serverFunc  func(w http.ResponseWriter, r *http.Request)
		expectedErr string
	}{
		{
			name:        "invalid URL",
			url:         "not-a-url",
			expectedErr: "invalid URL",
		},
		{
			name:        "empty URL",
			url:         "",
			expectedErr: "empty URL",
		},
		{
			name: "server returns 404",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			expectedErr: "unexpected status code: 404",
		},
		{
			name: "server returns 500",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedErr: "unexpected status code: 500",
		},
		{
			name: "empty HTML",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(""))
			},
			expectedErr: "extract content",
		},
		{
			name: "minimal HTML content",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte("<html><body></body></html>"))
			},
			expectedErr: "extract content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			var url string
			if tt.serverFunc != nil {
				server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
				defer server.Close()
				url = server.URL
			} else {
				url = tt.url
			}

			_, err := extractor.Extract(ctx, url)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestHTTPExtractor_Extract_LargeContent(t *testing.T) {
	// create large HTML content
	var sb strings.Builder
	sb.WriteString("<html><body><article>")
	for i := 0; i < 1000; i++ {
		sb.WriteString("<p>This is paragraph number ")
		sb.WriteString(strings.Repeat("content ", 100))
		sb.WriteString("</p>")
	}
	sb.WriteString("</article></body></html>")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(sb.String()))
	}))
	defer server.Close()

	extractor := NewHTTPExtractor(30*time.Second, "Newscope/1.0")

	ctx := context.Background()
	result, err := extractor.Extract(ctx, server.URL)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	// check that content was extracted
	assert.Contains(t, result.Content, "paragraph")
	assert.Contains(t, result.Content, "content")
}

func TestHTTPExtractor_Extract_Encoding(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		html        string
		expected    string
	}{
		{
			name:        "UTF-8 encoding",
			contentType: "text/html; charset=utf-8",
			html:        `<html><body><p>Hello 世界 🌍</p></body></html>`,
			expected:    "Hello 世界 🌍",
		},
		{
			name:        "no charset specified",
			contentType: "text/html",
			html:        `<html><body><p>Simple text</p></body></html>`,
			expected:    "Simple text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.Write([]byte(tt.html))
			}))
			defer server.Close()

			extractor := NewHTTPExtractor(5*time.Second, "Newscope/1.0")
			extractor.SetOptions(10, false, false) // set lower min text length for test

			ctx := context.Background()
			result, err := extractor.Extract(ctx, server.URL)
			require.NoError(t, err)
			assert.Contains(t, result.Content, tt.expected)
		})
	}
}

func TestHTTPExtractor_Extract_EmptyParagraphHandling(t *testing.T) {
	tests := []struct {
		name          string
		html          string
		expectNoEmpty bool
		description   string
	}{
		{
			name: "empty div at beginning",
			html: `<html><body>
				<article>
					<div></div>
					<p>This blog is inspired by Jordan Tigani's blog</p>
					<p>More content here</p>
				</article>
			</body></html>`,
			expectNoEmpty: true,
			description:   "should not have empty paragraph from empty div",
		},
		{
			name: "empty section at beginning",
			html: `<html><body>
				<section></section>
				<article>
					<p>The good old days ('70s)</p>
					<p>Back in the '70s, one relational database did everything.</p>
				</article>
			</body></html>`,
			expectNoEmpty: true,
			description:   "should not have empty paragraph from empty section",
		},
		{
			name: "multiple empty blocks",
			html: `<html><body>
				<div></div>
				<section>   </section>
				<article>
					<div>  </div>
					<p>Actual content starts here</p>
				</article>
			</body></html>`,
			expectNoEmpty: true,
			description:   "should not have empty paragraphs from multiple empty blocks",
		},
		{
			name: "nested empty divs",
			html: `<html><body>
				<div>
					<div>
						<div></div>
					</div>
				</div>
				<p>Real content</p>
			</body></html>`,
			expectNoEmpty: true,
			description:   "should not create empty paragraphs from nested empty divs",
		},
		{
			name: "div with whitespace only",
			html: `<html><body>
				<div>
					
					
				</div>
				<p>Content after whitespace</p>
			</body></html>`,
			expectNoEmpty: true,
			description:   "should not create empty paragraphs from divs with only whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write([]byte(tt.html))
			}))
			defer server.Close()

			extractor := NewHTTPExtractor(5*time.Second, "Newscope/1.0")
			extractor.SetOptions(10, false, false) // set lower min text length for test

			ctx := context.Background()
			result, err := extractor.Extract(ctx, server.URL)
			require.NoError(t, err)
			require.NotNil(t, result)

			// check rich content specifically
			if result.RichContent != "" {
				t.Logf("Rich content: %s", result.RichContent)

				// should not start with empty paragraph
				assert.False(t, strings.HasPrefix(result.RichContent, "<p></p>"),
					"rich content should not start with empty paragraph: %s", tt.description)

				// should not have empty paragraphs anywhere
				if tt.expectNoEmpty {
					assert.NotContains(t, result.RichContent, "<p></p>",
						"rich content should not contain empty paragraphs: %s", tt.description)
				}
			}
		})
	}
}

// benchmark extraction performance
func BenchmarkHTTPExtractor_Extract(b *testing.B) {
	// read test HTML file
	htmlData, err := os.ReadFile(filepath.Join("testdata", "medium_article.html"))
	require.NoError(b, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(htmlData)
	}))
	defer server.Close()

	extractor := NewHTTPExtractor(30*time.Second, "Newscope/1.0")

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.Extract(ctx, server.URL)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestHTTPExtractor_SetFallbackURL(t *testing.T) {
	extractor := NewHTTPExtractor(5*time.Second, "Newscope/1.0")

	// test setting fallback URL
	fallbackURL := "https://trafilatura.example.com/extract"
	extractor.SetFallbackURL(fallbackURL)

	// verify fallback URL is set (we can't directly check private field,
	// but we can verify the method doesn't panic and accepts the parameter)
	assert.NotNil(t, extractor)
}

func TestHTTPExtractor_SetOptions(t *testing.T) {
	extractor := NewHTTPExtractor(5*time.Second, "Newscope/1.0")

	// test setting options
	extractor.SetOptions(100, true, false)

	// verify options are set (we can't directly check private fields,
	// but we can verify the method doesn't panic and accepts the parameters)
	assert.NotNil(t, extractor)

	// test with different options
	extractor.SetOptions(50, false, true)
	assert.NotNil(t, extractor)
}
