package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPFetcher_Fetch(t *testing.T) {
	t.Run("valid rss feed", func(t *testing.T) {
		rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
	<channel>
		<title>Test Feed</title>
		<link>https://example.com</link>
		<description>Test feed description</description>
		<item>
			<title>Test Article 1</title>
			<link>https://example.com/article1</link>
			<description>Article 1 description</description>
			<guid>article1</guid>
			<pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
		</item>
		<item>
			<title>Test Article 2</title>
			<link>https://example.com/article2</link>
			<description>Article 2 description</description>
			<content:encoded><![CDATA[<p>Article 2 content</p>]]></content:encoded>
			<guid>article2</guid>
			<pubDate>Tue, 03 Jan 2006 15:04:05 -0700</pubDate>
		</item>
	</channel>
</rss>`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write([]byte(rssContent))
		}))
		defer server.Close()

		fetcher := NewHTTPFetcher(5 * time.Second)
		items, err := fetcher.Fetch(context.Background(), server.URL, "TestFeed")
		require.NoError(t, err)
		assert.Len(t, items, 2)

		// check first item
		assert.Equal(t, "TestFeed", items[0].FeedName)
		assert.Equal(t, "Test Article 1", items[0].Title)
		assert.Equal(t, "https://example.com/article1", items[0].URL)
		assert.Equal(t, "Article 1 description", items[0].Description)
		assert.Equal(t, "article1", items[0].GUID)
		assert.False(t, items[0].Published.IsZero())

		// check second item
		assert.Equal(t, "TestFeed", items[1].FeedName)
		assert.Equal(t, "Test Article 2", items[1].Title)
		assert.Equal(t, "https://example.com/article2", items[1].URL)
		assert.Equal(t, "Article 2 description", items[1].Description)
		assert.Equal(t, "<p>Article 2 content</p>", items[1].Content)
		assert.Equal(t, "article2", items[1].GUID)
		assert.False(t, items[1].Published.IsZero())
	})

	t.Run("atom feed", func(t *testing.T) {
		atomContent := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
	<title>Test Atom Feed</title>
	<link href="https://example.com/"/>
	<updated>2006-01-02T15:04:05Z</updated>
	<entry>
		<title>Atom Entry 1</title>
		<link href="https://example.com/entry1"/>
		<id>entry1</id>
		<updated>2006-01-02T15:04:05Z</updated>
		<summary>Entry 1 summary</summary>
	</entry>
</feed>`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/atom+xml")
			w.Write([]byte(atomContent))
		}))
		defer server.Close()

		fetcher := NewHTTPFetcher(5 * time.Second)
		items, err := fetcher.Fetch(context.Background(), server.URL, "AtomFeed")
		require.NoError(t, err)
		assert.Len(t, items, 1)

		assert.Equal(t, "AtomFeed", items[0].FeedName)
		assert.Equal(t, "Atom Entry 1", items[0].Title)
		assert.Equal(t, "https://example.com/entry1", items[0].URL)
		assert.Equal(t, "Entry 1 summary", items[0].Description)
		assert.Equal(t, "entry1", items[0].GUID)
		assert.False(t, items[0].Published.IsZero())
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		fetcher := NewHTTPFetcher(10 * time.Millisecond)
		items, err := fetcher.Fetch(context.Background(), server.URL, "TimeoutFeed")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
		assert.Nil(t, items)
	})

	t.Run("invalid url", func(t *testing.T) {
		fetcher := NewHTTPFetcher(5 * time.Second)
		items, err := fetcher.Fetch(context.Background(), "not-a-valid-url", "InvalidFeed")
		require.Error(t, err)
		assert.Nil(t, items)
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		fetcher := NewHTTPFetcher(5 * time.Second)
		items, err := fetcher.Fetch(context.Background(), server.URL, "ErrorFeed")
		require.Error(t, err)
		assert.Nil(t, items)
	})

	t.Run("invalid feed content", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not xml content"))
		}))
		defer server.Close()

		fetcher := NewHTTPFetcher(5 * time.Second)
		items, err := fetcher.Fetch(context.Background(), server.URL, "InvalidFeed")
		require.Error(t, err)
		assert.Nil(t, items)
	})
}