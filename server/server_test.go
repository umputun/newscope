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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/feed/types"
	"github.com/umputun/newscope/server/mocks"
)

func TestServer_New(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	db := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, db, scheduler, "1.0.0", false)
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

	db := &mocks.DatabaseMock{
		GetFeedsFunc: func(ctx context.Context) ([]types.Feed, error) {
			return []types.Feed{}, nil
		},
		GetItemsFunc: func(ctx context.Context, limit, offset int) ([]types.Item, error) {
			return []types.Item{}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, db, scheduler, "1.0.0", false)

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
	db := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, db, scheduler, "1.2.3", false)

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
	db := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, db, scheduler, "1.0.0", false)

	// create test request with path parameter
	req := httptest.NewRequest("GET", "/feed/technology", http.NoBody)
	req.SetPathValue("topic", "technology")
	w := httptest.NewRecorder()

	// call handler directly
	srv.rssFeedHandler(w, req)

	// check response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "RSS feed for topic: technology")
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
	
	db := &mocks.DatabaseMock{
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
	srv := New(cfg, db, scheduler, "1.0.0", false)

	// test request
	req := httptest.NewRequest("GET", "/articles?score=5.0&topic=tech", http.NoBody)
	w := httptest.NewRecorder()

	srv.articlesHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Test Article")
	assert.Contains(t, w.Body.String(), "Test Feed")
	assert.Contains(t, w.Body.String(), "Score: 8.5/10")
}

func TestServer_feedbackHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	feedbackCalled := false
	db := &mocks.DatabaseMock{
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
	srv := New(cfg, db, scheduler, "1.0.0", false)

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

	db := &mocks.DatabaseMock{
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

	srv := New(cfg, db, scheduler, "1.0.0", false)

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

	db := &mocks.DatabaseMock{}
	srv := New(cfg, db, scheduler, "1.0.0", false)

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

	db := &mocks.DatabaseMock{
		GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*types.ItemWithClassification, error) {
			assert.Equal(t, int64(789), itemID)
			return &types.ItemWithClassification{
				ExtractedContent: "This is the full article content.",
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}
	srv := New(cfg, db, scheduler, "1.0.0", false)

	req := httptest.NewRequest("GET", "/api/v1/articles/789/content", http.NoBody)
	req.SetPathValue("id", "789")
	w := httptest.NewRecorder()

	srv.articleContentHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Full Article")
	assert.Contains(t, w.Body.String(), "This is the full article content.")
	assert.Contains(t, w.Body.String(), "Close")
}