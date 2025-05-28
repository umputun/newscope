package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
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
		GetFeedsFunc: func(ctx context.Context) ([]types.Feed, error) {
			return []types.Feed{}, nil
		},
		GetItemsFunc: func(ctx context.Context, limit, offset int) ([]types.Item, error) {
			return []types.Item{}, nil
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
		GetClassifiedItemsFunc: func(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error) {
			assert.InEpsilon(t, 5.0, minScore, 0.001) // default score
			assert.Equal(t, "technology", topic)
			assert.Equal(t, 100, limit)

			return []types.ItemWithClassification{
				{
					Item: types.Item{
						GUID:      "guid-1",
						Title:     "Tech News",
						Link:      "https://example.com/tech",
						Published: now,
					},
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

	RenderJSON(w, req, http.StatusOK, data)

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

			RenderError(w, req, tt.err, tt.expectedCode)

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
	}

	now := time.Now()
	classifiedAt := now

	database := &mocks.DatabaseMock{
		GetClassifiedItemsFunc: func(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error) {
			return []types.ItemWithClassification{
				{
					Item: types.Item{
						GUID:        "guid-1",
						Title:       "Test Article",
						Link:        "https://example.com/article",
						Description: "A test article",
						Published:   now,
					},
					ID:             1,
					FeedName:       "Test Feed",
					RelevanceScore: 8.5,
					Explanation:    "Very relevant",
					Topics:         []string{"tech", "ai"},
					ClassifiedAt:   &classifiedAt,
				},
			}, nil
		},
		GetTopicsFunc: func(ctx context.Context) ([]string, error) {
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
	assert.Contains(t, w.Body.String(), "<html") // should contain full HTML

	// test HTMX request (partial update)
	req2 := httptest.NewRequest("GET", "/articles?score=5.0&topic=tech", http.NoBody)
	req2.Header.Set("HX-Request", "true")
	w2 := httptest.NewRecorder()

	srv.articlesHandler(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "Test Article")
	assert.Contains(t, w2.Body.String(), "Test Feed")
	assert.Contains(t, w2.Body.String(), "Score: 8.5/10")
	assert.NotContains(t, w2.Body.String(), "<html") // should NOT contain full HTML for HTMX request

	// test HTMX request with no articles
	database.GetClassifiedItemsFunc = func(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error) {
		return []types.ItemWithClassification{}, nil
	}

	req3 := httptest.NewRequest("GET", "/articles?score=10.0", http.NoBody)
	req3.Header.Set("HX-Request", "true")
	w3 := httptest.NewRecorder()

	srv.articlesHandler(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Contains(t, w3.Body.String(), "No articles found")
	assert.NotContains(t, w3.Body.String(), "<html") // should NOT contain full HTML
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
		GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*types.ItemWithClassification, error) {
			return &types.ItemWithClassification{
				Item: types.Item{
					Title:     "Test Article",
					Link:      "https://example.com",
					Published: time.Now(),
				},
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
		GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*types.ItemWithClassification, error) {
			return &types.ItemWithClassification{
				Item: types.Item{
					Title:     "Test Article",
					Link:      "https://example.com",
					Published: time.Now(),
				},
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

func TestServer_classifyNowHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	classifyCalled := false
	scheduler := &mocks.SchedulerMock{
		ClassifyNowFunc: func(ctx context.Context) error {
			classifyCalled = true
			return nil
		},
	}

	database := &mocks.DatabaseMock{}
	srv := New(cfg, database, scheduler, "1.0.0", false)

	req := httptest.NewRequest("POST", "/api/v1/classify-now", http.NoBody)
	w := httptest.NewRecorder()

	srv.classifyNowHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, classifyCalled)
}

func TestServer_articleContentHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*types.ItemWithClassification, error) {
			assert.Equal(t, int64(789), itemID)
			return &types.ItemWithClassification{
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
		GetClassifiedItemsFunc: func(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error) {
			// verify parameters
			assert.InEpsilon(t, 7.0, minScore, 0.001)
			assert.Equal(t, "technology", topic)
			assert.Equal(t, 100, limit)

			return []types.ItemWithClassification{
				{
					Item: types.Item{
						GUID:        "guid-1",
						Title:       "AI Breakthrough & More",
						Link:        "https://example.com/ai-news",
						Description: "Major advances in AI",
						Author:      "John Doe",
						Published:   now,
					},
					ID:             1,
					FeedName:       "Tech News",
					RelevanceScore: 9.5,
					Explanation:    "Highly relevant to AI developments",
					Topics:         []string{"ai", "technology"},
					ClassifiedAt:   &classifiedAt,
				},
				{
					Item: types.Item{
						GUID:        "guid-2",
						Title:       "Cloud Computing <Updates>",
						Link:        "https://example.com/cloud",
						Description: "New cloud services",
						Published:   now.Add(-1 * time.Hour),
					},
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

	srv := New(cfg, database, scheduler, "1.0.0", false)

	now := time.Now()
	items := []types.ItemWithClassification{
		{
			Item: types.Item{
				GUID:        "test-guid",
				Title:       "Test & Article",
				Link:        "https://example.com/test",
				Description: "Test description with <special> chars",
				Author:      "Test Author",
				Published:   now,
			},
			RelevanceScore: 8.0,
			Explanation:    "Test explanation",
			Topics:         []string{"test", "example"},
		},
	}

	rss := srv.generateRSSFeed("testing", 5.0, items)

	// verify RSS structure
	assert.Contains(t, rss, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, rss, `<title>Newscope - testing (Score ≥ 5.0)</title>`)
	assert.Contains(t, rss, `<title>[8.0] Test &amp; Article</title>`)
	assert.Contains(t, rss, `Test description with &lt;special&gt; chars`)
	assert.Contains(t, rss, `<category>test</category>`)
	assert.Contains(t, rss, `<category>example</category>`)

	// test empty topic
	rss = srv.generateRSSFeed("", 7.5, items)
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
		GetAllFeedsFunc: func(ctx context.Context) ([]db.Feed, error) {
			return []db.Feed{
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
		CreateFeedFunc: func(ctx context.Context, feed *db.Feed) error {
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
		GetAllFeedsFunc: func(ctx context.Context) ([]db.Feed, error) {
			return []db.Feed{
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
