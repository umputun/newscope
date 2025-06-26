package feed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/domain"
)

func TestGenerator_GenerateRSS(t *testing.T) {
	generator := NewGenerator("https://example.com")

	pubTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	items := []domain.ClassifiedItem{
		{
			Item: &domain.Item{
				ID:          1,
				Title:       "Test Article 1",
				Link:        "https://example.com/article1",
				GUID:        "guid1",
				Description: "This is test article 1",
				Author:      "John Doe",
				Published:   pubTime,
			},
			Classification: &domain.Classification{
				Score:       8.5,
				Explanation: "Highly relevant to AI topics",
				Topics:      []string{"AI", "Technology"},
			},
		},
		{
			Item: &domain.Item{
				ID:          2,
				Title:       "Test Article 2",
				Link:        "https://example.com/article2",
				GUID:        "guid2",
				Description: "This is test article 2",
				Author:      "Jane Smith",
				Published:   pubTime.Add(1 * time.Hour),
			},
			Classification: &domain.Classification{
				Score:       7.2,
				Explanation: "Moderately relevant",
				Topics:      []string{"Science"},
			},
		},
	}

	t.Run("generate RSS for all topics", func(t *testing.T) {
		rss, err := generator.GenerateRSS(items, "", 5.0)
		require.NoError(t, err)

		// check basic structure
		assert.Contains(t, rss, `<?xml version="1.0" encoding="UTF-8"?>`)
		assert.Contains(t, rss, `<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">`)
		assert.Contains(t, rss, `<title>Newscope - All Topics (Score ≥ 5.0)</title>`)
		assert.Contains(t, rss, `<link>https://example.com/</link>`)
		assert.Contains(t, rss, `<description>AI-curated articles with relevance score ≥ 5.0</description>`)

		// check atom self link (namespace is on the link element)
		assert.Contains(t, rss, `<link xmlns="http://www.w3.org/2005/Atom" href="https://example.com/rss" rel="self" type="application/rss+xml"></link>`)

		// check items
		assert.Contains(t, rss, `<title>[8.5] Test Article 1</title>`)
		assert.Contains(t, rss, `<link>https://example.com/article1</link>`)
		assert.Contains(t, rss, `<guid>guid1</guid>`)
		assert.Contains(t, rss, `<author>John Doe</author>`)
		assert.Contains(t, rss, `Score: 8.5/10 - Highly relevant to AI topics`)
		assert.Contains(t, rss, `Topics: AI, Technology`)
		assert.Contains(t, rss, `<category>AI</category>`)
		assert.Contains(t, rss, `<category>Technology</category>`)

		// check second item
		assert.Contains(t, rss, `<title>[7.2] Test Article 2</title>`)
		assert.Contains(t, rss, `<author>Jane Smith</author>`)
	})

	t.Run("generate RSS for specific topic", func(t *testing.T) {
		rss, err := generator.GenerateRSS(items, "AI", 7.0)
		require.NoError(t, err)

		assert.Contains(t, rss, `<title>Newscope - AI (Score ≥ 7.0)</title>`)
		assert.Contains(t, rss, `<link xmlns="http://www.w3.org/2005/Atom" href="https://example.com/rss/AI" rel="self" type="application/rss+xml"></link>`)
	})

	t.Run("empty items", func(t *testing.T) {
		rss, err := generator.GenerateRSS([]domain.ClassifiedItem{}, "", 5.0)
		require.NoError(t, err)

		assert.Contains(t, rss, `<channel>`)
		assert.NotContains(t, rss, `<item>`)
	})

	t.Run("generator with trailing slash in base URL", func(t *testing.T) {
		gen := NewGenerator("https://example.com/")
		rss, err := gen.GenerateRSS(items[:1], "", 5.0)
		require.NoError(t, err)

		// should not have double slashes
		assert.Contains(t, rss, `<link>https://example.com/</link>`)
		assert.Contains(t, rss, `href="https://example.com/rss"`)
		assert.NotContains(t, rss, `https://example.com//`)
	})
}

func TestGenerator_convertToRSSItem(t *testing.T) {
	generator := NewGenerator("https://example.com")

	item := domain.ClassifiedItem{
		Item: &domain.Item{
			Title:       "Test Article",
			Link:        "https://example.com/article",
			GUID:        "guid123",
			Description: "Article description",
			Author:      "Test Author",
			Published:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		Classification: &domain.Classification{
			Score:       9.0,
			Explanation: "Very relevant",
			Topics:      []string{"Tech", "AI"},
		},
	}

	rssItem := generator.convertToRSSItem(item)

	assert.Equal(t, "[9.0] Test Article", rssItem.Title)
	assert.Equal(t, "https://example.com/article", rssItem.Link)
	assert.Equal(t, "guid123", rssItem.GUID)
	assert.Equal(t, "Test Author", rssItem.Author)
	assert.Equal(t, []string{"Tech", "AI"}, rssItem.Categories)
	assert.Contains(t, rssItem.Description, "Score: 9.0/10 - Very relevant")
	assert.Contains(t, rssItem.Description, "Topics: Tech, AI")
	assert.Contains(t, rssItem.Description, "Article description")
}

func TestGenerator_GenerateOPML(t *testing.T) {
	generator := NewGenerator("https://example.com")

	feeds := []domain.Feed{
		{
			ID:      1,
			Title:   "Tech News",
			URL:     "https://technews.com/feed.xml",
			Enabled: true,
		},
		{
			ID:      2,
			Title:   "Science Daily",
			URL:     "https://sciencedaily.com/rss",
			Enabled: true,
		},
		{
			ID:      3,
			Title:   "Disabled Feed",
			URL:     "https://disabled.com/feed",
			Enabled: false,
		},
	}

	opml, err := generator.GenerateOPML(feeds)
	require.NoError(t, err)

	// check basic structure
	assert.Contains(t, opml, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, opml, `<opml version="2.0">`)
	assert.Contains(t, opml, `<title>Newscope Feed Subscriptions</title>`)

	// check enabled feeds are included
	assert.Contains(t, opml, `text="Tech News"`)
	assert.Contains(t, opml, `title="Tech News"`)
	assert.Contains(t, opml, `type="rss"`)
	assert.Contains(t, opml, `xmlUrl="https://technews.com/feed.xml"`)
	assert.Contains(t, opml, `htmlUrl="https://technews.com/feed.xml"`)

	assert.Contains(t, opml, `text="Science Daily"`)
	assert.Contains(t, opml, `xmlUrl="https://sciencedaily.com/rss"`)

	// check disabled feed is not included
	assert.NotContains(t, opml, "Disabled Feed")
	assert.NotContains(t, opml, "disabled.com")
}

func TestRSSXMLStructure(t *testing.T) {
	// test that the XML structure is correctly formed
	generator := NewGenerator("https://example.com")

	items := []domain.ClassifiedItem{
		{
			Item: &domain.Item{
				Title:     "Test & Article <with> Special Characters",
				Link:      "https://example.com/article",
				GUID:      "guid1",
				Author:    "Author & Co.",
				Published: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			Classification: &domain.Classification{
				Score:       8.0,
				Explanation: "Test explanation with <html> tags",
				Topics:      []string{"Tech & Science"},
			},
		},
	}

	rss, err := generator.GenerateRSS(items, "", 5.0)
	require.NoError(t, err)

	// XML special characters should be escaped
	assert.Contains(t, rss, "Test &amp; Article &lt;with&gt; Special Characters")
	assert.Contains(t, rss, "Author &amp; Co.")
	assert.Contains(t, rss, "Test explanation with &lt;html&gt; tags")
	assert.Contains(t, rss, "Tech &amp; Science")

	// verify it's valid XML by checking key elements are present and properly nested
	assert.Regexp(t, `(?s)<rss[^>]*>.*<channel>.*</channel>.*</rss>`, rss)
}
