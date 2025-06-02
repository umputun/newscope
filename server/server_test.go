package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-pkgz/routegroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/repository"
	"github.com/umputun/newscope/server/mocks"
)

// testServer creates a server instance using the actual New function
func testServer(t *testing.T, cfg ConfigProvider, database Database, scheduler Scheduler) *Server {
	// use the actual New function which properly loads and separates templates
	return New(cfg, database, scheduler, "test", false)
}

func TestServer_New(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, database, scheduler, "1.0.0", false)
	assert.NotNil(t, srv)
	assert.Equal(t, "1.0.0", srv.version)
	assert.False(t, srv.debug)
}

func TestServer_Run(t *testing.T) {
	// find free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	err = listener.Close()
	require.NoError(t, err)

	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return fmt.Sprintf("127.0.0.1:%d", port), 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		GetFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{}, nil
		},
		GetItemsFunc: func(ctx context.Context, limit, offset int) ([]domain.Item, error) {
			return []domain.Item{}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start server in background
	go func() {
		_ = srv.Run(ctx)
	}()

	// wait for server to start
	time.Sleep(100 * time.Millisecond)

	// make test request
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(body))

	// shutdown server
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestServer_statusHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, database, scheduler, "1.2.3", false)

	// create test request
	req := httptest.NewRequest("GET", "/status", http.NoBody)
	w := httptest.NewRecorder()

	// call handler directly
	srv.statusHandler(w, req)

	// check response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// check response body
	var status map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &status)
	require.NoError(t, err)

	assert.Equal(t, "ok", status["status"])
	assert.Equal(t, "1.2.3", status["version"])
	assert.NotEmpty(t, status["time"])
}

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

func TestRenderJSON(t *testing.T) {
	data := map[string]string{
		"message": "test",
		"status":  "ok",
	}

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	renderJSON(w, req, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, data, result)
}

func TestRenderError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode int
		expectedMsg  string
	}{
		{
			name:         "generic error",
			err:          errors.New("something went wrong"),
			expectedCode: http.StatusInternalServerError,
			expectedMsg:  "something went wrong",
		},
		{
			name:         "nil error",
			err:          nil,
			expectedCode: http.StatusInternalServerError,
			expectedMsg:  "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			w := httptest.NewRecorder()

			renderError(w, req, tt.err, tt.expectedCode)

			assert.Equal(t, tt.expectedCode, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var result map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &result)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedMsg, result["error"])
		})
	}
}

func TestServer_articlesHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
		GetFullConfigFunc: func() *config.Config {
			return &config.Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
				},
			}
		},
	}

	now := time.Now()
	classifiedAt := now

	database := &mocks.DatabaseMock{
		GetClassifiedItemsWithFiltersFunc: func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ItemWithClassification, error) {
			return []domain.ItemWithClassification{
				{
					GUID:           "guid-1",
					Title:          "Test Article",
					Link:           "https://example.com/article",
					Description:    "A test article",
					Published:      now,
					ID:             1,
					FeedName:       "Test Feed",
					RelevanceScore: 8.5,
					Explanation:    "Very relevant",
					Topics:         []string{"tech", "ai"},
					ClassifiedAt:   &classifiedAt,
				},
			}, nil
		},
		GetClassifiedItemsCountFunc: func(ctx context.Context, req domain.ArticlesRequest) (int, error) {
			return 1, nil // return count of 1 item for testing
		},
		GetActiveFeedNamesFunc: func(ctx context.Context, minScore float64) ([]string, error) {
			return []string{"Test Feed", "Example Feed"}, nil
		},
		GetTopicsFunc: func(ctx context.Context) ([]string, error) {
			return []string{"tech", "ai", "science"}, nil
		},
		GetTopicsFilteredFunc: func(ctx context.Context, minScore float64) ([]string, error) {
			// for testing, return topics based on score threshold
			if minScore >= 5.0 {
				return []string{"tech", "ai"}, nil // fewer topics for higher scores
			}
			return []string{"tech", "ai", "science"}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	// test regular request (full page)
	req := httptest.NewRequest("GET", "/articles?score=5.0&topic=tech", http.NoBody)
	w := httptest.NewRecorder()

	srv.articlesHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Test Article")
	assert.Contains(t, w.Body.String(), "Test Feed")
	assert.Contains(t, w.Body.String(), "Score: 8.5/10")
	assert.Contains(t, w.Body.String(), "<html")                                                                    // should contain full HTML
	assert.Contains(t, w.Body.String(), "Articles <span id=\"article-count\" class=\"article-count\">(1/1)</span>") // should show count

	// test HTMX request (partial update)
	req2 := httptest.NewRequest("GET", "/articles?score=5.0&topic=tech", http.NoBody)
	req2.Header.Set("HX-Request", "true")
	w2 := httptest.NewRecorder()

	srv.articlesHandler(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "Test Article")
	assert.Contains(t, w2.Body.String(), "Test Feed")
	assert.Contains(t, w2.Body.String(), "Score: 8.5/10")
	assert.NotContains(t, w2.Body.String(), "<html")                                                                       // should NOT contain full HTML for HTMX request
	assert.Contains(t, w2.Body.String(), `<span id="article-count" class="article-count" hx-swap-oob="true">(1/1)</span>`) // should update count

	// test HTMX request with no articles
	database.GetClassifiedItemsWithFiltersFunc = func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ItemWithClassification, error) {
		return []domain.ItemWithClassification{}, nil
	}

	req3 := httptest.NewRequest("GET", "/articles?score=10.0", http.NoBody)
	req3.Header.Set("HX-Request", "true")
	w3 := httptest.NewRecorder()

	srv.articlesHandler(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Contains(t, w3.Body.String(), "No articles found")
	assert.NotContains(t, w3.Body.String(), "<html")                                                                       // should NOT contain full HTML
	assert.Contains(t, w3.Body.String(), `<span id="article-count" class="article-count" hx-swap-oob="true">(0/1)</span>`) // should show 0 count
}

func TestServer_feedbackHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	feedbackCalled := false
	database := &mocks.DatabaseMock{
		UpdateItemFeedbackFunc: func(ctx context.Context, itemID int64, feedback string) error {
			feedbackCalled = true
			assert.Equal(t, int64(123), itemID)
			assert.Equal(t, "like", feedback)
			return nil
		},
		GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error) {
			return &domain.ItemWithClassification{
				Title:          "Test Article",
				Link:           "https://example.com",
				Published:      time.Now(),
				ID:             itemID,
				FeedName:       "Test Feed",
				RelevanceScore: 7.5,
				UserFeedback:   "like",
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	// test like action
	req := httptest.NewRequest("POST", "/api/v1/feedback/123/like", http.NoBody)
	req.SetPathValue("id", "123")
	req.SetPathValue("action", "like")
	w := httptest.NewRecorder()

	srv.feedbackHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, feedbackCalled)
	assert.Contains(t, w.Body.String(), "Test Article")
	assert.Contains(t, w.Body.String(), "btn-like active") // button should be active
}

func TestServer_extractHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	extractCalled := false
	scheduler := &mocks.SchedulerMock{
		ExtractContentNowFunc: func(ctx context.Context, itemID int64) error {
			extractCalled = true
			assert.Equal(t, int64(456), itemID)
			return nil
		},
	}

	database := &mocks.DatabaseMock{
		GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error) {
			return &domain.ItemWithClassification{

				Title:            "Test Article",
				Published:        time.Now(),
				ID:               itemID,
				FeedName:         "Test Feed",
				ExtractedContent: "Full article content here",
			}, nil
		},
	}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	req := httptest.NewRequest("POST", "/api/v1/extract/456", http.NoBody)
	req.SetPathValue("id", "456")
	w := httptest.NewRecorder()

	srv.extractHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, extractCalled)
	assert.Contains(t, w.Body.String(), "Show Content") // button should change
}

func TestServer_articleContentHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error) {
			assert.Equal(t, int64(789), itemID)
			return &domain.ItemWithClassification{

				Title:            "Full Article",
				ExtractedContent: "This is the full article content.",
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/api/v1/articles/789/content", http.NoBody)
	req.SetPathValue("id", "789")
	w := httptest.NewRecorder()

	srv.articleContentHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Full Article")
	assert.Contains(t, w.Body.String(), "This is the full article content.")
	assert.Contains(t, w.Body.String(), "Close")
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

func TestServer_feedsHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	now := time.Now()
	database := &mocks.DatabaseMock{
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{
				{
					ID:            1,
					URL:           "https://example.com/feed.xml",
					Title:         "Example Feed",
					Description:   "A test feed",
					LastFetched:   &now,
					NextFetch:     &now,
					FetchInterval: 3600,
					ErrorCount:    0,
					Enabled:       true,
				},
				{
					ID:            2,
					URL:           "https://test.com/rss",
					Title:         "Test RSS",
					Description:   "Another feed",
					FetchInterval: 1800,
					ErrorCount:    2,
					LastError:     "Connection timeout",
					Enabled:       false,
				},
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/feeds", http.NoBody)
	w := httptest.NewRecorder()

	srv.feedsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Feed Management")
	assert.Contains(t, w.Body.String(), "Example Feed")
	assert.Contains(t, w.Body.String(), "https://example.com/feed.xml")
	assert.Contains(t, w.Body.String(), "Test RSS")
	assert.Contains(t, w.Body.String(), "Connection timeout")
}

func TestServer_createFeedHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	feedCreated := false
	fetchTriggered := false

	database := &mocks.DatabaseMock{
		CreateFeedFunc: func(ctx context.Context, feed *domain.Feed) error {
			feedCreated = true
			assert.Equal(t, "https://newsite.com/feed", feed.URL)
			assert.Equal(t, "New Site", feed.Title)
			assert.Equal(t, 1800, feed.FetchInterval)
			assert.True(t, feed.Enabled)
			feed.ID = 99 // simulate DB assigning ID
			return nil
		},
	}

	scheduler := &mocks.SchedulerMock{
		UpdateFeedNowFunc: func(ctx context.Context, feedID int64) error {
			fetchTriggered = true
			assert.Equal(t, int64(99), feedID)
			return nil
		},
	}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	form := "url=https://newsite.com/feed&title=New+Site&fetch_interval=30"
	req := httptest.NewRequest("POST", "/api/v1/feeds", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.createFeedHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, feedCreated)

	// wait for async fetch
	time.Sleep(50 * time.Millisecond)
	assert.True(t, fetchTriggered)

	// response should contain feed card HTML
	assert.Contains(t, w.Body.String(), "New Site")
	assert.Contains(t, w.Body.String(), "https://newsite.com/feed")
}

func TestServer_updateFeedStatus(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	statusUpdated := false
	now := time.Now()

	database := &mocks.DatabaseMock{
		UpdateFeedStatusFunc: func(ctx context.Context, feedID int64, enabled bool) error {
			statusUpdated = true
			assert.Equal(t, int64(42), feedID)
			assert.True(t, enabled)
			return nil
		},
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{
				{
					ID:            42,
					URL:           "https://example.com/feed",
					Title:         "Updated Feed",
					Enabled:       true,
					LastFetched:   &now,
					FetchInterval: 3600,
				},
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("POST", "/api/v1/feeds/42/enable", http.NoBody)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	srv.enableFeedHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, statusUpdated)
	assert.Contains(t, w.Body.String(), "Updated Feed")
}

func TestServer_deleteFeedHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	feedDeleted := false
	database := &mocks.DatabaseMock{
		DeleteFeedFunc: func(ctx context.Context, feedID int64) error {
			feedDeleted = true
			assert.Equal(t, int64(123), feedID)
			return nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("DELETE", "/api/v1/feeds/123", http.NoBody)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	srv.deleteFeedHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, feedDeleted)
}

func TestNewRepositoryAdapter(t *testing.T) {
	t.Run("creates adapter with real repos", func(t *testing.T) {
		// create in-memory database repositories
		ctx := context.Background()
		cfg := repository.Config{
			DSN:             ":memory:",
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: 30 * time.Second,
		}

		repos, err := repository.NewRepositories(ctx, cfg)
		require.NoError(t, err)
		defer repos.Close()

		adapter := NewRepositoryAdapter(repos)
		assert.NotNil(t, adapter)

		// verify we can call methods on the adapter
		feeds, err := adapter.GetFeeds(ctx)
		require.NoError(t, err)
		assert.NotNil(t, feeds)
		assert.Empty(t, feeds) // should be empty in new database
	})
}

func TestRepositoryAdapter_GetClassifiedItems(t *testing.T) {
	// create in-memory database repositories
	ctx := context.Background()
	cfg := repository.Config{
		DSN:             ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 30 * time.Second,
	}

	repos, err := repository.NewRepositories(ctx, cfg)
	require.NoError(t, err)
	defer repos.Close()

	adapter := NewRepositoryAdapter(repos)

	t.Run("get classified items from empty database", func(t *testing.T) {
		items, err := adapter.GetClassifiedItems(ctx, 0.0, "", 10)
		require.NoError(t, err)
		assert.NotNil(t, items)
		assert.Empty(t, items)
	})

	t.Run("get classified items with filters", func(t *testing.T) {
		items, err := adapter.GetClassifiedItems(ctx, 5.0, "tech", 5)
		require.NoError(t, err)
		assert.NotNil(t, items)
		assert.Empty(t, items) // should be empty in empty database
	})
}

func TestServer_DisableFeedHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	feedDisabled := false
	testFeed := domain.Feed{
		ID:      456,
		Title:   "Test Feed",
		Enabled: false, // disabled after update
	}

	database := &mocks.DatabaseMock{
		UpdateFeedStatusFunc: func(ctx context.Context, feedID int64, enabled bool) error {
			feedDisabled = true
			assert.Equal(t, int64(456), feedID)
			assert.False(t, enabled) // should be disabled
			return nil
		},
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{testFeed}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("POST", "/feeds/456/disable", http.NoBody)
	req.SetPathValue("id", "456")
	w := httptest.NewRecorder()

	srv.disableFeedHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, feedDisabled)
}

func TestServer_FetchFeedHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	fetchTriggered := false
	now := time.Now()
	testFeed := domain.Feed{
		ID:          123,
		Title:       "Test Feed",
		URL:         "https://example.com/feed.xml",
		Enabled:     true,
		LastFetched: &now,
	}

	database := &mocks.DatabaseMock{
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{testFeed}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{
		UpdateFeedNowFunc: func(ctx context.Context, feedID int64) error {
			fetchTriggered = true
			assert.Equal(t, int64(123), feedID)
			return nil
		},
	}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	req := httptest.NewRequest("POST", "/api/v1/feeds/123/fetch", http.NoBody)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	srv.fetchFeedHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, fetchTriggered)
	assert.Contains(t, w.Body.String(), "Test Feed")
}

func TestServer_FetchFeedHandler_InvalidID(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	req := httptest.NewRequest("POST", "/api/v1/feeds/invalid/fetch", http.NoBody)
	req.SetPathValue("id", "invalid")
	w := httptest.NewRecorder()

	srv.fetchFeedHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid feed ID")
}

func TestServer_FetchFeedHandler_SchedulerError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{
		UpdateFeedNowFunc: func(ctx context.Context, feedID int64) error {
			return fmt.Errorf("scheduler error")
		},
	}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	req := httptest.NewRequest("POST", "/api/v1/feeds/456/fetch", http.NoBody)
	req.SetPathValue("id", "456")
	w := httptest.NewRecorder()

	srv.fetchFeedHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "scheduler error")
}

func TestServer_SettingsHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
		GetFullConfigFunc: func() *config.Config {
			return &config.Config{
				LLM: config.LLMConfig{
					APIKey:   "test-key",
					Model:    "gpt-4",
					Endpoint: "https://api.openai.com/v1",
				},
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
				},
			}
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/settings", http.NoBody)
	w := httptest.NewRecorder()

	srv.settingsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Settings")
	assert.Contains(t, w.Body.String(), "gpt-4")
	assert.Contains(t, w.Body.String(), ":8080")
}

func TestServer_HideContentHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/api/v1/articles/123/hide", http.NoBody)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	srv.hideContentHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// should contain empty response body and out-of-band button update
	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "content-toggle-123")
	assert.Contains(t, responseBody, "hx-swap-oob=\"true\"")
	assert.Contains(t, responseBody, "Show Content")
}

func TestServer_HideContentHandler_InvalidID(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/api/v1/articles/invalid/hide", http.NoBody)
	req.SetPathValue("id", "invalid")
	w := httptest.NewRecorder()

	srv.hideContentHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid article ID")
}

// error scenario tests for server handlers

func TestServer_ArticlesHandler_DatabaseError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
		GetFullConfigFunc: func() *config.Config {
			return &config.Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				}{PageSize: 50},
			}
		},
	}

	t.Run("database error on get classified items", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemsWithFiltersFunc: func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ItemWithClassification, error) {
				return nil, errors.New("database connection failed")
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/articles", http.NoBody)
		w := httptest.NewRecorder()

		srv.articlesHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to load articles")
	})

	t.Run("database error on get count", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemsWithFiltersFunc: func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ItemWithClassification, error) {
				return []domain.ItemWithClassification{}, nil
			},
			GetClassifiedItemsCountFunc: func(ctx context.Context, req domain.ArticlesRequest) (int, error) {
				return 0, errors.New("count query failed")
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/articles", http.NoBody)
		w := httptest.NewRecorder()

		srv.articlesHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to load articles count")
	})
}

func TestServer_RenderFeedCard_TemplateError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	// create server with invalid templates to trigger template error
	srv := &Server{
		config:    cfg,
		db:        database,
		scheduler: scheduler,
		version:   "test",
		debug:     false,
		router:    routegroup.New(http.NewServeMux()),
		templates: template.New("broken"), // empty template without feed-card.html
	}

	feed := &domain.Feed{
		ID:    1,
		Title: "Test Feed",
		URL:   "https://example.com/feed",
	}

	w := httptest.NewRecorder()
	srv.renderFeedCard(w, feed)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to render feed")
}

func TestServer_RenderArticleCard_TemplateError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	// create server with invalid templates to trigger template error
	srv := &Server{
		config:    cfg,
		db:        database,
		scheduler: scheduler,
		version:   "test",
		debug:     false,
		router:    routegroup.New(http.NewServeMux()),
		templates: template.New("broken"), // empty template without article-card.html
	}

	article := &domain.ItemWithClassification{
		ID:             1,
		Title:          "Test Article",
		RelevanceScore: 8.5,
	}

	w := httptest.NewRecorder()
	srv.renderArticleCard(w, article)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to render article")
}

func TestServer_ArticlesHandler_TemplateError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
		GetFullConfigFunc: func() *config.Config {
			return &config.Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				}{PageSize: 50},
			}
		},
	}

	now := time.Now()
	database := &mocks.DatabaseMock{
		GetClassifiedItemsWithFiltersFunc: func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ItemWithClassification, error) {
			return []domain.ItemWithClassification{
				{
					ID:             1,
					Title:          "Test Article",
					Published:      now,
					RelevanceScore: 8.5,
				},
			}, nil
		},
		GetClassifiedItemsCountFunc: func(ctx context.Context, req domain.ArticlesRequest) (int, error) {
			return 1, nil
		},
		GetTopicsFilteredFunc: func(ctx context.Context, minScore float64) ([]string, error) {
			return []string{"tech"}, nil
		},
		GetActiveFeedNamesFunc: func(ctx context.Context, minScore float64) ([]string, error) {
			return []string{"Test Feed"}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}

	// create server with broken page templates
	srv := &Server{
		config:        cfg,
		db:            database,
		scheduler:     scheduler,
		version:       "test",
		debug:         false,
		router:        routegroup.New(http.NewServeMux()),
		templates:     template.New("test"),
		pageTemplates: map[string]*template.Template{}, // empty page templates
	}

	req := httptest.NewRequest("GET", "/articles", http.NoBody)
	w := httptest.NewRecorder()

	srv.articlesHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to render page")
}

func TestServer_FeedsHandler_DatabaseError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return nil, errors.New("database connection failed")
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/feeds", http.NoBody)
	w := httptest.NewRecorder()

	srv.feedsHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to load feeds")
}

func TestServer_FeedsHandler_TemplateError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{
				{
					ID:    1,
					Title: "Test Feed",
					URL:   "https://example.com/feed",
				},
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}

	// create server with broken page templates
	srv := &Server{
		config:        cfg,
		db:            database,
		scheduler:     scheduler,
		version:       "test",
		debug:         false,
		router:        routegroup.New(http.NewServeMux()),
		templates:     template.New("test"),
		pageTemplates: map[string]*template.Template{}, // empty page templates
	}

	req := httptest.NewRequest("GET", "/feeds", http.NoBody)
	w := httptest.NewRecorder()

	srv.feedsHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to render page")
}

func TestServer_SettingsHandler_TemplateError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
		GetFullConfigFunc: func() *config.Config {
			return &config.Config{
				LLM: config.LLMConfig{
					Model: "gpt-4",
				},
			}
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	// create server with broken page templates
	srv := &Server{
		config:        cfg,
		db:            database,
		scheduler:     scheduler,
		version:       "test",
		debug:         false,
		router:        routegroup.New(http.NewServeMux()),
		templates:     template.New("test"),
		pageTemplates: map[string]*template.Template{}, // empty page templates
	}

	req := httptest.NewRequest("GET", "/settings", http.NoBody)
	w := httptest.NewRecorder()

	srv.settingsHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to render page")
}

func TestServer_CreateFeedHandler_DatabaseError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		CreateFeedFunc: func(ctx context.Context, feed *domain.Feed) error {
			return errors.New("database write failed")
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	form := "url=https://example.com/feed&title=Test+Feed"
	req := httptest.NewRequest("POST", "/api/v1/feeds", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.createFeedHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "database write failed")
}

func TestServer_CreateFeedHandler_InvalidForm(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	t.Run("missing URL", func(t *testing.T) {
		form := "title=Test+Feed"
		req := httptest.NewRequest("POST", "/api/v1/feeds", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.createFeedHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "feed URL is required")
	})

	t.Run("invalid form data", func(t *testing.T) {
		// create request with malformed form data to trigger form parsing error
		req := httptest.NewRequest("POST", "/api/v1/feeds", strings.NewReader("invalid%ZZ%form"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.createFeedHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid form data")
	})
}

func TestServer_UpdateFeedStatus_DatabaseError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		UpdateFeedStatusFunc: func(ctx context.Context, feedID int64, enabled bool) error {
			return errors.New("update failed")
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("POST", "/api/v1/feeds/123/enable", http.NoBody)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	srv.enableFeedHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "update failed")
}

func TestServer_UpdateFeedStatus_GetFeedsError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		UpdateFeedStatusFunc: func(ctx context.Context, feedID int64, enabled bool) error {
			return nil // update succeeds
		},
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return nil, errors.New("failed to get feeds")
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("POST", "/api/v1/feeds/123/enable", http.NoBody)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	srv.enableFeedHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to reload feed")
}

func TestServer_UpdateFeedStatus_FeedNotFound(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		UpdateFeedStatusFunc: func(ctx context.Context, feedID int64, enabled bool) error {
			return nil // update succeeds
		},
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{
				{ID: 999, Title: "Other Feed"}, // different ID
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("POST", "/api/v1/feeds/123/enable", http.NoBody)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	srv.enableFeedHandler(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Feed not found")
}

func TestServer_DeleteFeedHandler_DatabaseError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		DeleteFeedFunc: func(ctx context.Context, feedID int64) error {
			return errors.New("delete failed")
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("DELETE", "/api/v1/feeds/123", http.NoBody)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	srv.deleteFeedHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "delete failed")
}

func TestServer_FeedbackHandler_DatabaseErrors(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("feedback update error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			UpdateItemFeedbackFunc: func(ctx context.Context, itemID int64, feedback string) error {
				return errors.New("feedback update failed")
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/feedback/123/like", http.NoBody)
		req.SetPathValue("id", "123")
		req.SetPathValue("action", "like")
		w := httptest.NewRecorder()

		srv.feedbackHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "feedback update failed")
	})

	t.Run("get item error after feedback", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			UpdateItemFeedbackFunc: func(ctx context.Context, itemID int64, feedback string) error {
				return nil // update succeeds
			},
			GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error) {
				return nil, errors.New("item not found")
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/feedback/123/like", http.NoBody)
		req.SetPathValue("id", "123")
		req.SetPathValue("action", "like")
		w := httptest.NewRecorder()

		srv.feedbackHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to reload article")
	})

	t.Run("invalid action", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/feedback/123/invalid", http.NoBody)
		req.SetPathValue("id", "123")
		req.SetPathValue("action", "invalid")
		w := httptest.NewRecorder()

		srv.feedbackHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid action")
	})

	t.Run("invalid item ID", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/feedback/invalid/like", http.NoBody)
		req.SetPathValue("id", "invalid")
		req.SetPathValue("action", "like")
		w := httptest.NewRecorder()

		srv.feedbackHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid item ID")
	})
}

func TestServer_ExtractHandler_Errors(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("invalid item ID", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/extract/invalid", http.NoBody)
		req.SetPathValue("id", "invalid")
		w := httptest.NewRecorder()

		srv.extractHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid item ID")
	})

	t.Run("extraction failed", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		scheduler := &mocks.SchedulerMock{
			ExtractContentNowFunc: func(ctx context.Context, itemID int64) error {
				return errors.New("extraction service unavailable")
			},
		}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/extract/123", http.NoBody)
		req.SetPathValue("id", "123")
		w := httptest.NewRecorder()

		srv.extractHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "extraction service unavailable")
	})

	t.Run("get item error after extraction", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error) {
				return nil, errors.New("item not found")
			},
		}
		scheduler := &mocks.SchedulerMock{
			ExtractContentNowFunc: func(ctx context.Context, itemID int64) error {
				return nil // extraction succeeds
			},
		}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/extract/123", http.NoBody)
		req.SetPathValue("id", "123")
		w := httptest.NewRecorder()

		srv.extractHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to reload article")
	})
}

func TestServer_ArticleContentHandler_Errors(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("invalid article ID", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/api/v1/articles/invalid/content", http.NoBody)
		req.SetPathValue("id", "invalid")
		w := httptest.NewRecorder()

		srv.articleContentHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid article ID")
	})

	t.Run("article not found", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error) {
				return nil, errors.New("article not found")
			},
		}
		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/api/v1/articles/123/content", http.NoBody)
		req.SetPathValue("id", "123")
		w := httptest.NewRecorder()

		srv.articleContentHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Article not found")
	})

	t.Run("template execution error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error) {
				return &domain.ItemWithClassification{
					ID:               123,
					Title:            "Test Article",
					ExtractedContent: "Test content",
				}, nil
			},
		}
		scheduler := &mocks.SchedulerMock{}

		// create server with broken templates
		srv := &Server{
			config:    cfg,
			db:        database,
			scheduler: scheduler,
			version:   "test",
			debug:     false,
			router:    routegroup.New(http.NewServeMux()),
			templates: template.New("broken"), // empty template
		}

		req := httptest.NewRequest("GET", "/api/v1/articles/123/content", http.NoBody)
		req.SetPathValue("id", "123")
		w := httptest.NewRecorder()

		srv.articleContentHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to render content")
	})
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
