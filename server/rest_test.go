package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/server/mocks"
)

func TestServer_statusHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
		},
	}

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

	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
		},
	}
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
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
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

func TestServer_createFeedHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	feedCreated := false
	var mu sync.Mutex
	fetchTriggered := false

	database := &mocks.DatabaseMock{
		CreateFeedFunc: func(ctx context.Context, feed *domain.Feed) error {
			feedCreated = true
			assert.Equal(t, "https://newsite.com/feed", feed.URL)
			assert.Equal(t, "New Site", feed.Title)
			assert.Equal(t, 30*time.Minute, feed.FetchInterval)
			assert.True(t, feed.Enabled)
			feed.ID = 99 // simulate DB assigning ID
			return nil
		},
	}

	scheduler := &mocks.SchedulerMock{
		UpdateFeedNowFunc: func(ctx context.Context, feedID int64) error {
			mu.Lock()
			fetchTriggered = true
			mu.Unlock()
			assert.Equal(t, int64(99), feedID)
			return nil
		},
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
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
	mu.Lock()
	assert.True(t, fetchTriggered)
	mu.Unlock()

	// response should contain feed card HTML
	assert.Contains(t, w.Body.String(), "New Site")
	assert.Contains(t, w.Body.String(), "https://newsite.com/feed")
}

func TestServer_updateFeedHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	testFeed := domain.Feed{
		ID:            123,
		URL:           "https://example.com/feed.xml",
		Title:         "Updated Test Feed",
		FetchInterval: 2400, // 40 minutes in seconds
		Enabled:       true,
	}

	database := &mocks.DatabaseMock{
		UpdateFeedFunc: func(ctx context.Context, feedID int64, title string, fetchInterval time.Duration) error {
			assert.Equal(t, int64(123), feedID)
			assert.Equal(t, "New Title", title)
			assert.Equal(t, 40*time.Minute, fetchInterval) // 40 minutes
			return nil
		},
		GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{testFeed}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
		},
	}
	srv := New(cfg, database, scheduler, "1.0.0", false)

	// create form data
	form := "title=New+Title&fetch_interval=40"
	req := httptest.NewRequest("PUT", "/api/v1/feeds/123", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	srv.updateFeedHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Updated Test Feed") // feed card should be rendered
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

	scheduler := &mocks.SchedulerMock{
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
		},
	}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("POST", "/api/v1/feeds/42/enable", http.NoBody)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	srv.enableFeedHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, statusUpdated)
	assert.Contains(t, w.Body.String(), "Updated Feed")
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
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
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

// error tests

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

		scheduler := &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {
				// do nothing in tests
			},
		}
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

		scheduler := &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {
				// do nothing in tests
			},
		}
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
	scheduler := &mocks.SchedulerMock{
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
		},
	}
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

func TestServer_UpdateFeedHandler_Errors(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("invalid ID", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		scheduler := &mocks.SchedulerMock{}
		srv := New(cfg, database, scheduler, "1.0.0", false)

		req := httptest.NewRequest("PUT", "/api/v1/feeds/invalid", strings.NewReader(""))
		req.SetPathValue("id", "invalid")
		w := httptest.NewRecorder()

		srv.updateFeedHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid feed ID")
	})

	t.Run("database error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			UpdateFeedFunc: func(ctx context.Context, feedID int64, title string, fetchInterval time.Duration) error {
				return fmt.Errorf("database error")
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := New(cfg, database, scheduler, "1.0.0", false)

		form := "title=New+Title&fetch_interval=40"
		req := httptest.NewRequest("PUT", "/api/v1/feeds/123", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "123")
		w := httptest.NewRecorder()

		srv.updateFeedHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "database error")
	})

	t.Run("feed not found", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			UpdateFeedFunc: func(ctx context.Context, feedID int64, title string, fetchInterval time.Duration) error {
				return nil
			},
			GetAllFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
				return []domain.Feed{}, nil // no feeds
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := New(cfg, database, scheduler, "1.0.0", false)

		form := "title=New+Title&fetch_interval=40"
		req := httptest.NewRequest("PUT", "/api/v1/feeds/123", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "123")
		w := httptest.NewRecorder()

		srv.updateFeedHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "Feed not found\n", w.Body.String())
	})
}

func TestServer_UpdateFeedStatus_Errors(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("database error", func(t *testing.T) {
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
	})

	t.Run("get feeds error", func(t *testing.T) {
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
	})

	t.Run("feed not found", func(t *testing.T) {
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
	})
}

func TestServer_FetchFeedHandler_Errors(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("invalid ID", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		scheduler := &mocks.SchedulerMock{}

		srv := New(cfg, database, scheduler, "1.0.0", false)

		req := httptest.NewRequest("POST", "/api/v1/feeds/invalid/fetch", http.NoBody)
		req.SetPathValue("id", "invalid")
		w := httptest.NewRecorder()

		srv.fetchFeedHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid feed ID")
	})

	t.Run("scheduler error", func(t *testing.T) {
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
	})
}

func TestServer_DeleteFeedHandler_Error(t *testing.T) {
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
