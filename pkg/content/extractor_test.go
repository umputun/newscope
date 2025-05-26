package content

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPExtractor_Extract(t *testing.T) {
	tests := []struct {
		name        string
		htmlContent string
		wantContent string
		wantErr     bool
		statusCode  int
	}{
		{
			name: "successful extraction",
			htmlContent: `<!DOCTYPE html>
				<html>
				<head><title>Test Article</title></head>
				<body>
					<article>
						<h1>Test Article Title</h1>
						<p>This is the main content of the article.</p>
						<p>It has multiple paragraphs.</p>
					</article>
				</body>
				</html>`,
			wantContent: "Test Article Title",
			statusCode:  http.StatusOK,
		},
		{
			name: "extraction with minimal content",
			htmlContent: `<!DOCTYPE html>
				<html>
				<body>
					<p>Short content</p>
				</body>
				</html>`,
			wantContent: "Short content",
			statusCode:  http.StatusOK,
		},
		{
			name:        "server error",
			htmlContent: "error",
			wantErr:     true,
			statusCode:  http.StatusInternalServerError,
		},
		{
			name:        "not found",
			htmlContent: "not found",
			wantErr:     true,
			statusCode:  http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					w.Header().Set("Content-Type", "text/html")
				}
				_, _ = w.Write([]byte(tt.htmlContent))
			}))
			defer server.Close()

			// create extractor
			extractor := NewHTTPExtractor(10 * time.Second)

			// test extraction
			ctx := context.Background()
			content, err := extractor.Extract(ctx, server.URL)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Contains(t, content, tt.wantContent)
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
