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

	extractor := NewHTTPExtractor(30 * time.Second)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			url := server.URL + "/" + tt.htmlFile

			content, err := extractor.Extract(ctx, url)
			require.NoError(t, err)
			require.NotEmpty(t, content)
			t.Logf("Extracted content (%d chars): %s", len(content), content)

			// check minimum length
			assert.GreaterOrEqual(t, len(content), tt.minLength,
				"extracted content is too short: got %d chars, expected at least %d",
				len(content), tt.minLength)

			// check expected content is present
			for _, expected := range tt.expectedContent {
				assert.Contains(t, content, expected,
					"expected content '%s' not found in extracted text", expected)
			}

			// check unwanted content is filtered out
			for _, unexpected := range tt.unexpectedContent {
				assert.NotContains(t, content, unexpected,
					"unexpected content '%s' found in extracted text", unexpected)
			}

			// additional checks
			assert.NotContains(t, content, "<script", "HTML tags should be removed")
			assert.NotContains(t, content, "<style", "CSS should be removed")
			assert.NotContains(t, content, "<!DOCTYPE", "DOCTYPE should be removed")
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
	extractor := NewHTTPExtractor(100 * time.Millisecond)

	ctx := context.Background()
	_, err := extractor.Extract(ctx, server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestHTTPExtractor_Extract_InvalidURL(t *testing.T) {
	extractor := NewHTTPExtractor(time.Second)

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

	extractor := NewHTTPExtractor(5 * time.Second)

	// create context and cancel it immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := extractor.Extract(ctx, server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestHTTPExtractor_Extract_ErrorCases(t *testing.T) {
	extractor := NewHTTPExtractor(1 * time.Second)

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

	extractor := NewHTTPExtractor(30 * time.Second)

	ctx := context.Background()
	content, err := extractor.Extract(ctx, server.URL)
	require.NoError(t, err)
	require.NotEmpty(t, content)

	// check that content was extracted
	assert.Contains(t, content, "paragraph")
	assert.Contains(t, content, "content")
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

			extractor := NewHTTPExtractor(5 * time.Second)

			ctx := context.Background()
			content, err := extractor.Extract(ctx, server.URL)
			require.NoError(t, err)
			assert.Contains(t, content, tt.expected)
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

	extractor := NewHTTPExtractor(30 * time.Second)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.Extract(ctx, server.URL)
		if err != nil {
			b.Fatal(err)
		}
	}
}
