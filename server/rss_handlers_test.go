package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/server/mocks"
)

func TestServer_rssFeedHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	now := time.Now()
	database := &mocks.DatabaseMock{
		GetClassifiedItemsFunc: func(ctx context.Context, minScore float64, topic string, limit int) ([]domain.ItemWithClassification, error) {
			assert.InEpsilon(t, 5.0, minScore, 0.001) // default score
			assert.Equal(t, "technology", topic)
			assert.Equal(t, 100, limit)

			return []domain.ItemWithClassification{
				{
					GUID:           "guid-1",
					Title:          "Tech News",
					Link:           "https://example.com/tech",
					Published:      now,
					RelevanceScore: 8.5,
					Explanation:    "Tech related",
					Topics:         []string{"technology"},
				},
			}, nil
		},
	}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	// create test request with path parameter
	req := httptest.NewRequest("GET", "/rss/technology", http.NoBody)
	req.SetPathValue("topic", "technology")
	w := httptest.NewRecorder()

	// call handler directly
	srv.rssHandler(w, req)

	// check response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/rss+xml; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `<title>Newscope - technology (Score ≥ 5.0)</title>`)
	assert.Contains(t, w.Body.String(), `<title>[8.5] Tech News</title>`)
}

func TestServer_rssHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	now := time.Now()
	classifiedAt := now

	database := &mocks.DatabaseMock{
		GetClassifiedItemsFunc: func(ctx context.Context, minScore float64, topic string, limit int) ([]domain.ItemWithClassification, error) {
			// verify parameters
			assert.InEpsilon(t, 7.0, minScore, 0.001)
			assert.Equal(t, "technology", topic)
			assert.Equal(t, 100, limit)

			return []domain.ItemWithClassification{
				{
					GUID:           "guid-1",
					Title:          "AI Breakthrough & More",
					Link:           "https://example.com/ai-news",
					Description:    "Major advances in AI",
					Author:         "John Doe",
					Published:      now,
					ID:             1,
					FeedName:       "Tech News",
					RelevanceScore: 9.5,
					Explanation:    "Highly relevant to AI developments",
					Topics:         []string{"ai", "technology"},
					ClassifiedAt:   &classifiedAt,
				},
				{
					GUID:           "guid-2",
					Title:          "Cloud Computing <Updates>",
					Link:           "https://example.com/cloud",
					Description:    "New cloud services",
					Published:      now.Add(-1 * time.Hour),
					ID:             2,
					FeedName:       "Cloud Weekly",
					RelevanceScore: 7.5,
					Explanation:    "Important cloud updates",
					Topics:         []string{"cloud", "infrastructure"},
					ClassifiedAt:   &classifiedAt,
				},
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	// test RSS request
	req := httptest.NewRequest("GET", "/rss?topic=technology&min_score=7.0", http.NoBody)
	w := httptest.NewRecorder()

	srv.rssHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/rss+xml; charset=utf-8", w.Header().Get("Content-Type"))

	rss := w.Body.String()

	// check RSS structure
	assert.Contains(t, rss, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, rss, `<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">`)
	assert.Contains(t, rss, `<channel>`)
	assert.Contains(t, rss, `<title>Newscope - technology (Score ≥ 7.0)</title>`)
	assert.Contains(t, rss, `<link>http://localhost:8080/</link>`)
	assert.Contains(t, rss, `<description>AI-curated articles with relevance score ≥ 7.0</description>`)

	// check first item
	assert.Contains(t, rss, `<title>[9.5] AI Breakthrough &amp; More</title>`)
	assert.Contains(t, rss, `<link>https://example.com/ai-news</link>`)
	assert.Contains(t, rss, `<guid>guid-1</guid>`)
	assert.Contains(t, rss, `Score: 9.5/10 - Highly relevant to AI developments`)
	assert.Contains(t, rss, `Topics: ai, technology`)
	assert.Contains(t, rss, `<author>John Doe</author>`)
	assert.Contains(t, rss, `<category>ai</category>`)
	assert.Contains(t, rss, `<category>technology</category>`)

	// check second item with XML escaping
	assert.Contains(t, rss, `<title>[7.5] Cloud Computing &lt;Updates&gt;</title>`)
	assert.Contains(t, rss, `<link>https://example.com/cloud</link>`)

	// check it's valid XML structure
	assert.Contains(t, rss, `</channel>`)
	assert.Contains(t, rss, `</rss>`)
}

func TestServer_generateRSSFeed(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	now := time.Now()
	items := []domain.ItemWithClassification{
		{
			GUID:           "test-guid",
			Title:          "Test & Article",
			Link:           "https://example.com/test",
			Description:    "Test description with <special> chars",
			Author:         "Test Author",
			Published:      now,
			RelevanceScore: 8.0,
			Explanation:    "Test explanation",
			Topics:         []string{"test", "example"},
		},
	}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	rss := srv.buildRSSFeed("testing", 5.0, items)

	// verify RSS structure
	assert.Contains(t, rss, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, rss, `<title>Newscope - testing (Score ≥ 5.0)</title>`)
	assert.Contains(t, rss, `<title>[8.0] Test &amp; Article</title>`)
	assert.Contains(t, rss, `Test description with &lt;special&gt; chars`)
	assert.Contains(t, rss, `<category>test</category>`)
	assert.Contains(t, rss, `<category>example</category>`)

	// test empty topic
	rss = srv.buildRSSFeed("", 7.5, items)
	assert.Contains(t, rss, `<title>Newscope - All Topics (Score ≥ 7.5)</title>`)
}

func TestServer_RSSHandler_DatabaseError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		GetClassifiedItemsFunc: func(ctx context.Context, minScore float64, topic string, limit int) ([]domain.ItemWithClassification, error) {
			return nil, errors.New("database query failed")
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/rss/technology", http.NoBody)
	req.SetPathValue("topic", "technology")
	w := httptest.NewRecorder()

	srv.rssHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to generate RSS feed")
}
