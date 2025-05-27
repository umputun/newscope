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

func TestParser_Parse(t *testing.T) {
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
	<title>Test Feed</title>
	<link>http://example.com</link>
	<description>Test Description</description>
	<item>
		<title>Test Article 1</title>
		<link>http://example.com/article1</link>
		<description>Article 1 description</description>
		<content:encoded><![CDATA[<p>Full content of article 1</p>]]></content:encoded>
		<pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
		<guid>http://example.com/article1</guid>
		<author>test@example.com (Test Author)</author>
	</item>
	<item>
		<title>Test Article 2</title>
		<link>http://example.com/article2</link>
		<description>Article 2 description</description>
		<pubDate>Tue, 03 Jan 2006 15:04:05 -0700</pubDate>
	</item>
</channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssContent))
	}))
	defer server.Close()

	parser := NewParser(5 * time.Second)
	feed, err := parser.Parse(context.Background(), server.URL)
	require.NoError(t, err)

	assert.Equal(t, "Test Feed", feed.Title)
	assert.Equal(t, "Test Description", feed.Description)
	assert.Equal(t, "http://example.com", feed.Link)

	require.Len(t, feed.Items, 2)

	// check first item
	item1 := feed.Items[0]
	assert.Equal(t, "Test Article 1", item1.Title)
	assert.Equal(t, "http://example.com/article1", item1.Link)
	assert.Equal(t, "Article 1 description", item1.Description)
	assert.Equal(t, "<p>Full content of article 1</p>", item1.Content)
	assert.Equal(t, "http://example.com/article1", item1.GUID)
	assert.Equal(t, "Test Author", item1.Author)
	assert.False(t, item1.Published.IsZero())

	// check second item - should generate GUID from link
	item2 := feed.Items[1]
	assert.Equal(t, "Test Article 2", item2.Title)
	assert.Equal(t, "http://example.com/article2", item2.Link)
	assert.Equal(t, "http://example.com/article2", item2.GUID)
}

func TestParser_Parse_AtomFeed(t *testing.T) {
	atomContent := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
	<title>Test Atom Feed</title>
	<link href="http://example.com"/>
	<subtitle>Test Subtitle</subtitle>
	<entry>
		<title>Atom Entry 1</title>
		<link href="http://example.com/entry1"/>
		<id>urn:uuid:1225c695-cfb8-4ebb-aaaa-80da344efa6a</id>
		<updated>2006-01-02T15:04:05Z</updated>
		<summary>Entry 1 summary</summary>
		<author>
			<name>John Doe</name>
		</author>
	</entry>
</feed>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(atomContent))
	}))
	defer server.Close()

	parser := NewParser(5 * time.Second)
	feed, err := parser.Parse(context.Background(), server.URL)
	require.NoError(t, err)

	assert.Equal(t, "Test Atom Feed", feed.Title)
	assert.Equal(t, "Test Subtitle", feed.Description)

	require.Len(t, feed.Items, 1)
	item := feed.Items[0]
	assert.Equal(t, "Atom Entry 1", item.Title)
	assert.Equal(t, "http://example.com/entry1", item.Link)
	assert.Equal(t, "urn:uuid:1225c695-cfb8-4ebb-aaaa-80da344efa6a", item.GUID)
	assert.Equal(t, "John Doe", item.Author)
}

func TestParser_Parse_Errors(t *testing.T) {
	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		parser := NewParser(5 * time.Second)
		_, err := parser.Parse(context.Background(), server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code: 500")
	})

	t.Run("Invalid XML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not xml"))
		}))
		defer server.Close()

		parser := NewParser(5 * time.Second)
		_, err := parser.Parse(context.Background(), server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse feed")
	})

	t.Run("Timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.Write([]byte("too late"))
		}))
		defer server.Close()

		parser := NewParser(100 * time.Millisecond)
		_, err := parser.Parse(context.Background(), server.URL)
		require.Error(t, err)
	})

	t.Run("Invalid URL", func(t *testing.T) {
		parser := NewParser(5 * time.Second)
		_, err := parser.Parse(context.Background(), "not-a-url")
		require.Error(t, err)
	})
}

func TestParser_Parse_NoGUID(t *testing.T) {
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
	<title>Test Feed</title>
	<item>
		<title>No GUID Article</title>
		<description>Article without GUID or link</description>
	</item>
</channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssContent))
	}))
	defer server.Close()

	parser := NewParser(5 * time.Second)
	feed, err := parser.Parse(context.Background(), server.URL)
	require.NoError(t, err)

	require.Len(t, feed.Items, 1)
	item := feed.Items[0]
	// should generate GUID from feed title and item title
	assert.Equal(t, "Test Feed-No GUID Article", item.GUID)
}

