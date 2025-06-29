package server

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-pkgz/routegroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/server/mocks"
)

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
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
			}
		},
	}

	now := time.Now()
	classifiedAt := now

	database := &mocks.DatabaseMock{
		GetClassifiedItemsWithFiltersFunc: func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ClassifiedItem, error) {
			return []domain.ClassifiedItem{
				{
					Item: &domain.Item{
						ID:          1,
						GUID:        "guid-1",
						Title:       "Test Article",
						Link:        "https://example.com/article",
						Description: "A test article",
						Published:   now,
					},
					FeedName: "Test Feed",
					Classification: &domain.Classification{
						Score:        8.5,
						Explanation:  "Very relevant",
						Topics:       []string{"tech", "ai"},
						ClassifiedAt: classifiedAt,
					},
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

	scheduler := &mocks.SchedulerMock{
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
		},
	}
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
	database.GetClassifiedItemsWithFiltersFunc = func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ClassifiedItem, error) {
		return []domain.ClassifiedItem{}, nil
	}

	req3 := httptest.NewRequest("GET", "/articles?score=10.0", http.NoBody)
	req3.Header.Set("HX-Request", "true")
	w3 := httptest.NewRecorder()

	srv.articlesHandler(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Contains(t, w3.Body.String(), "No articles found")
	assert.NotContains(t, w3.Body.String(), "<html")                                                                       // should NOT contain full HTML
	assert.Contains(t, w3.Body.String(), `<span id="article-count" class="article-count" hx-swap-oob="true">(0/1)</span>`) // should show 0 count

	// test liked filter
	database.GetClassifiedItemsWithFiltersFunc = func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ClassifiedItem, error) {
		// verify that ShowLikedOnly is passed correctly
		if req.ShowLikedOnly {
			return []domain.ClassifiedItem{
				{
					Item: &domain.Item{
						ID:          2,
						Title:       "Liked Article",
						Link:        "https://example.com/liked",
						Description: "A liked article",
						Published:   now,
					},
					FeedName: "Test Feed",
					Classification: &domain.Classification{
						Score:        9.0,
						Explanation:  "User liked this",
						Topics:       []string{"favorites"},
						ClassifiedAt: classifiedAt,
					},
					UserFeedback: &domain.Feedback{
						Type: domain.FeedbackLike,
					},
				},
			}, nil
		}
		return []domain.ClassifiedItem{}, nil
	}

	// test with liked filter on
	req4 := httptest.NewRequest("GET", "/articles?liked=on", http.NoBody)
	w4 := httptest.NewRecorder()

	srv.articlesHandler(w4, req4)

	assert.Equal(t, http.StatusOK, w4.Code)
	assert.Contains(t, w4.Body.String(), "Liked Article")
	assert.Contains(t, w4.Body.String(), "★ Liked") // check that button is rendered

	// test with liked filter using "true" value
	req5 := httptest.NewRequest("GET", "/articles?liked=true", http.NoBody)
	w5 := httptest.NewRecorder()

	srv.articlesHandler(w5, req5)

	assert.Equal(t, http.StatusOK, w5.Code)
	assert.Contains(t, w5.Body.String(), "Liked Article")
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

	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
	}
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
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
			}
		},
	}

	database := &mocks.DatabaseMock{
		GetSettingFunc: func(ctx context.Context, key string) (string, error) {
			// return empty strings for topic preferences
			return "", nil
		},
		GetTopicsFunc: func(ctx context.Context) ([]string, error) {
			// return some sample topics for testing
			return []string{"technology", "science", "business"}, nil
		},
	}
	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
	}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/settings", http.NoBody)
	w := httptest.NewRecorder()

	srv.settingsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Settings")
	assert.Contains(t, w.Body.String(), "gpt-4")
	assert.Contains(t, w.Body.String(), ":8080")
}

func TestServer_articleContentHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{
		GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ClassifiedItem, error) {
			assert.Equal(t, int64(789), itemID)
			return &domain.ClassifiedItem{
				Item: &domain.Item{
					Title: "Full Article",
				},
				Extraction: &domain.ExtractedContent{
					PlainText: "This is the full article content.",
				},
			}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
	}
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

func TestServer_HideContentHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
	}
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
	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
	}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/api/v1/articles/invalid/hide", http.NoBody)
	req.SetPathValue("id", "invalid")
	w := httptest.NewRecorder()

	srv.hideContentHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid article ID")
}

// error scenario tests

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
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
			}
		},
	}

	t.Run("database error on get classified items", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemsWithFiltersFunc: func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ClassifiedItem, error) {
				return nil, errors.New("database connection failed")
			},
		}

		scheduler := &mocks.SchedulerMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
				return nil
			},
		}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/articles", http.NoBody)
		w := httptest.NewRecorder()

		srv.articlesHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to load articles")
	})

	t.Run("database error on get count", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemsWithFiltersFunc: func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ClassifiedItem, error) {
				return []domain.ClassifiedItem{}, nil
			},
			GetClassifiedItemsCountFunc: func(ctx context.Context, req domain.ArticlesRequest) (int, error) {
				return 0, errors.New("count query failed")
			},
		}

		scheduler := &mocks.SchedulerMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
				return nil
			},
		}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/articles", http.NoBody)
		w := httptest.NewRecorder()

		srv.articlesHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to load articles count")
	})
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

	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
	}
	srv := testServer(t, cfg, database, scheduler)

	req := httptest.NewRequest("GET", "/feeds", http.NoBody)
	w := httptest.NewRecorder()

	srv.feedsHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to load feeds")
}

func TestServer_ArticleContentHandler_Errors(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("invalid article ID", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		scheduler := &mocks.SchedulerMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
				return nil
			},
		}
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
			GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ClassifiedItem, error) {
				return nil, errors.New("article not found")
			},
		}
		scheduler := &mocks.SchedulerMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
				return nil
			},
		}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/api/v1/articles/123/content", http.NoBody)
		req.SetPathValue("id", "123")
		w := httptest.NewRecorder()

		srv.articleContentHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Article not found")
	})
}

// template error tests for render functions

func TestServer_RenderFeedCard_TemplateError(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
	}

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
	scheduler := &mocks.SchedulerMock{
		UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
			return nil
		},
	}

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

	article := &domain.ClassifiedItem{
		Item: &domain.Item{
			ID:    1,
			Title: "Test Article",
		},
		Classification: &domain.Classification{
			Score: 8.5,
		},
	}

	w := httptest.NewRecorder()
	srv.renderArticleCard(w, article)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to render article")
}

func TestServer_TemplateErrors(t *testing.T) {
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
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
			}
		},
	}

	now := time.Now()

	t.Run("ArticlesHandler template error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemsWithFiltersFunc: func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ClassifiedItem, error) {
				return []domain.ClassifiedItem{
					{
						Item: &domain.Item{
							ID:        1,
							Title:     "Test Article",
							Published: now,
						},
						Classification: &domain.Classification{
							Score: 8.5,
						},
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

		scheduler := &mocks.SchedulerMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
				return nil
			},
		}

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
	})

	t.Run("FeedsHandler template error", func(t *testing.T) {
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

		scheduler := &mocks.SchedulerMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
				return nil
			},
		}

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
	})

	t.Run("SettingsHandler template error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				// return empty strings for topic preferences
				return "", nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{}, nil
			},
		}
		scheduler := &mocks.SchedulerMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
				return nil
			},
		}

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
	})

	t.Run("ArticleContentHandler template error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetClassifiedItemFunc: func(ctx context.Context, itemID int64) (*domain.ClassifiedItem, error) {
				return &domain.ClassifiedItem{
					Item: &domain.Item{
						ID:    123,
						Title: "Test Article",
					},
					Extraction: &domain.ExtractedContent{
						PlainText: "Test content",
					},
				}, nil
			},
		}
		scheduler := &mocks.SchedulerMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context) error {
				return nil
			},
		}

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

func TestServer_AddTopicHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("add preferred topic successfully", func(t *testing.T) {
		var database *mocks.DatabaseMock
		database = &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				callCount := len(database.GetSettingCalls())
				switch callCount {
				case 1: // first call for addTopicHandler
					assert.Equal(t, domain.SettingPreferredTopics, key)
					return `["golang", "rust"]`, nil
				case 2: // second call for renderTopicsListWithDropdowns - preferred
					assert.Equal(t, domain.SettingPreferredTopics, key)
					return `["golang", "rust", "python"]`, nil
				case 3: // third call for renderTopicsListWithDropdowns - avoided
					assert.Equal(t, domain.SettingAvoidedTopics, key)
					return `[]`, nil
				default:
					return "", nil
				}
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				assert.Equal(t, domain.SettingPreferredTopics, key)
				// verify the new topic was added
				var topics []string
				err := json.Unmarshal([]byte(value), &topics)
				require.NoError(t, err)
				assert.Contains(t, topics, "golang")
				assert.Contains(t, topics, "rust")
				assert.Contains(t, topics, "python")
				return nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"golang", "rust", "python", "javascript", "database"}, nil
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "python")
		form.Add("type", "preferred")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "golang")
		assert.Contains(t, w.Body.String(), "rust")
		assert.Contains(t, w.Body.String(), "python")
		// verify dropdowns have available topics
		assert.Contains(t, w.Body.String(), "javascript")
		assert.Contains(t, w.Body.String(), "database")
	})

	t.Run("add avoided topic successfully", func(t *testing.T) {
		var database *mocks.DatabaseMock
		database = &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				callCount := len(database.GetSettingCalls())
				switch callCount {
				case 1: // first call for addTopicHandler
					assert.Equal(t, domain.SettingAvoidedTopics, key)
					return `[]`, nil
				case 2: // second call for renderTopicsListWithDropdowns - preferred
					assert.Equal(t, domain.SettingPreferredTopics, key)
					return `[]`, nil
				case 3: // third call for renderTopicsListWithDropdowns - avoided
					assert.Equal(t, domain.SettingAvoidedTopics, key)
					return `["sports"]`, nil
				default:
					return "", nil
				}
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				assert.Equal(t, domain.SettingAvoidedTopics, key)
				assert.Equal(t, `["sports"]`, value)
				return nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"golang", "sports", "python", "javascript"}, nil
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "sports")
		form.Add("type", "avoided")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "sports")
		// verify dropdowns exclude sports
		assert.Contains(t, w.Body.String(), "golang")
		assert.Contains(t, w.Body.String(), "javascript")
	})

	t.Run("add duplicate topic", func(t *testing.T) {
		var database *mocks.DatabaseMock
		database = &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				callCount := len(database.GetSettingCalls())
				switch callCount {
				case 1: // first call for addTopicHandler
					return `["golang", "rust"]`, nil
				case 2: // second call for renderTopicsListWithDropdowns - preferred
					return `["golang", "rust"]`, nil
				case 3: // third call for renderTopicsListWithDropdowns - avoided
					return `[]`, nil
				default:
					return "", nil
				}
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"golang", "rust", "python", "javascript"}, nil
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "golang")
		form.Add("type", "preferred")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		// should return existing list without duplicates
		assert.Contains(t, w.Body.String(), "golang")
		assert.Contains(t, w.Body.String(), "rust")
	})

	t.Run("invalid topic type", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "test")
		form.Add("type", "invalid")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("add topic with spaces", func(t *testing.T) {
		var database *mocks.DatabaseMock
		database = &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				callCount := len(database.GetSettingCalls())
				switch callCount {
				case 1: // first call for addTopicHandler
					return `[]`, nil
				case 2: // second call for renderTopicsListWithDropdowns - preferred
					return `["machine learning"]`, nil
				case 3: // third call for renderTopicsListWithDropdowns - avoided
					return `[]`, nil
				default:
					return "", nil
				}
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				// verify the topic with spaces is properly handled
				assert.Equal(t, `["machine learning"]`, value)
				return nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"golang", "machine learning", "python"}, nil
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "machine learning")
		form.Add("type", "preferred")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "machine learning")
	})

	t.Run("add empty topic after trim", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "   ") // only spaces
		form.Add("type", "preferred")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("add topic with invalid characters", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "test@#$%^&*") // invalid characters
		form.Add("type", "preferred")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid topic name format")
	})

	t.Run("add topic with Unicode characters", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				if key == domain.SettingPreferredTopics {
					return `["golang"]`, nil
				}
				return `[]`, nil
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				assert.Equal(t, domain.SettingPreferredTopics, key)
				// verify that Unicode topic was added
				var topics []string
				err := json.Unmarshal([]byte(value), &topics)
				require.NoError(t, err)
				assert.Contains(t, topics, "технологии")
				return nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"golang", "technology", "science"}, nil
			},
		}

		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "технологии") // russian Unicode
		form.Add("type", "preferred")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Len(t, database.SetSettingCalls(), 1)
	})

	t.Run("add topic with Chinese characters", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				if key == domain.SettingAvoidedTopics {
					return `[]`, nil
				}
				return `[]`, nil
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				assert.Equal(t, domain.SettingAvoidedTopics, key)
				// verify that Chinese topic was added
				var topics []string
				err := json.Unmarshal([]byte(value), &topics)
				require.NoError(t, err)
				assert.Contains(t, topics, "科技新闻")
				return nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"golang", "technology", "science"}, nil
			},
		}

		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", "科技新闻") // chinese Unicode
		form.Add("type", "avoided")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Len(t, database.SetSettingCalls(), 1)
	})

	t.Run("add topic exceeding max length", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		form := url.Values{}
		form.Add("topic", strings.Repeat("a", 51)) // exceeds 50 character limit
		form.Add("type", "preferred")

		req := httptest.NewRequest("POST", "/api/v1/topics", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.addTopicHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid topic name format")
	})
}

func TestServer_RssHelpHandler(t *testing.T) {
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
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
					BaseURL:  "http://example.com",
				},
			}
		},
	}

	t.Run("render RSS help page successfully", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetTopTopicsByScoreFunc: func(ctx context.Context, minScore float64, limit int) ([]domain.TopicWithScore, error) {
				assert.InDelta(t, 5.0, minScore, 0.01)
				assert.Equal(t, 10, limit)
				return []domain.TopicWithScore{
					{Topic: "technology", AvgScore: 8.5, ItemCount: 150},
					{Topic: "science", AvgScore: 8.2, ItemCount: 120},
					{Topic: "business", AvgScore: 7.8, ItemCount: 90},
				}, nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"technology", "science", "business", "politics", "health"}, nil
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{})

		req := httptest.NewRequest("GET", "/rss-help", http.NoBody)
		w := httptest.NewRecorder()

		srv.rssHelpHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "RSS Feeds")
		assert.Contains(t, body, "http://example.com") // base URL
		assert.Contains(t, body, "technology")
		assert.Contains(t, body, "science")
		assert.Contains(t, body, "business")
		// check that all topics are available in dropdowns
		assert.Contains(t, body, "politics")
		assert.Contains(t, body, "health")
	})

	t.Run("handle GetTopTopicsByScore error gracefully", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetTopTopicsByScoreFunc: func(ctx context.Context, minScore float64, limit int) ([]domain.TopicWithScore, error) {
				return nil, errors.New("database error")
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"technology", "science"}, nil
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{})

		req := httptest.NewRequest("GET", "/rss-help", http.NoBody)
		w := httptest.NewRecorder()

		srv.rssHelpHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "RSS Feeds")
		// should still render the page even without top topics
		assert.Contains(t, body, "http://example.com")
	})

	t.Run("handle GetTopics error gracefully", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetTopTopicsByScoreFunc: func(ctx context.Context, minScore float64, limit int) ([]domain.TopicWithScore, error) {
				return []domain.TopicWithScore{
					{Topic: "technology", AvgScore: 8.5, ItemCount: 150},
				}, nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return nil, errors.New("database error")
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{})

		req := httptest.NewRequest("GET", "/rss-help", http.NoBody)
		w := httptest.NewRecorder()

		srv.rssHelpHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "RSS Feeds")
		assert.Contains(t, body, "technology")
		// should still render even without all topics list
	})

	t.Run("template error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetTopTopicsByScoreFunc: func(ctx context.Context, minScore float64, limit int) ([]domain.TopicWithScore, error) {
				return []domain.TopicWithScore{}, nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{}, nil
			},
		}

		// create server with broken page templates
		srv := &Server{
			config:        cfg,
			db:            database,
			scheduler:     &mocks.SchedulerMock{},
			version:       "test",
			debug:         false,
			router:        routegroup.New(http.NewServeMux()),
			templates:     template.New("test"),
			pageTemplates: map[string]*template.Template{}, // empty page templates
		}

		req := httptest.NewRequest("GET", "/rss-help", http.NoBody)
		w := httptest.NewRecorder()

		srv.rssHelpHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to render page")
	})
}

func TestServer_DeleteTopicHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("delete preferred topic successfully", func(t *testing.T) {
		var database *mocks.DatabaseMock
		database = &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				callCount := len(database.GetSettingCalls())
				switch callCount {
				case 1: // first call for deleteTopicHandler
					assert.Equal(t, domain.SettingPreferredTopics, key)
					return `["golang", "rust", "python"]`, nil
				case 2: // second call for renderTopicsListWithDropdowns - preferred
					assert.Equal(t, domain.SettingPreferredTopics, key)
					return `["golang", "python"]`, nil
				case 3: // third call for renderTopicsListWithDropdowns - avoided
					assert.Equal(t, domain.SettingAvoidedTopics, key)
					return `[]`, nil
				default:
					return "", nil
				}
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				assert.Equal(t, domain.SettingPreferredTopics, key)
				// verify rust was removed
				var topics []string
				err := json.Unmarshal([]byte(value), &topics)
				require.NoError(t, err)
				assert.NotContains(t, topics, "rust")
				assert.Contains(t, topics, "golang")
				assert.Contains(t, topics, "python")
				return nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"golang", "rust", "python", "javascript", "database"}, nil
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		req := httptest.NewRequest("DELETE", "/api/v1/topics/rust?type=preferred", http.NoBody)
		req.SetPathValue("topic", "rust")
		w := httptest.NewRecorder()

		srv.deleteTopicHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		// check topic tags don't contain rust
		assert.Contains(t, body, "golang")
		assert.Contains(t, body, "python")
		assert.NotContains(t, body, `<span class="topic-tag topic-preferred">
    <i class="fas fa-heart"></i>
    rust`)
		// verify rust is now available in dropdowns
		assert.Contains(t, body, `<option value="rust">rust</option>`)
	})

	t.Run("delete non-existent topic", func(t *testing.T) {
		var database *mocks.DatabaseMock
		database = &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				callCount := len(database.GetSettingCalls())
				switch callCount {
				case 1: // first call for deleteTopicHandler
					return `["golang"]`, nil
				case 2: // second call for renderTopicsListWithDropdowns - preferred
					return `["golang"]`, nil
				case 3: // third call for renderTopicsListWithDropdowns - avoided
					return `[]`, nil
				default:
					return "", nil
				}
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				// should still be called with unchanged list
				assert.Equal(t, `["golang"]`, value)
				return nil
			},
			GetTopicsFunc: func(ctx context.Context) ([]string, error) {
				return []string{"golang", "rust", "python", "javascript"}, nil
			},
		}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		req := httptest.NewRequest("DELETE", "/api/v1/topics/nonexistent?type=preferred", http.NoBody)
		req.SetPathValue("topic", "nonexistent")
		w := httptest.NewRecorder()

		srv.deleteTopicHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "golang")
	})

	t.Run("invalid topic type", func(t *testing.T) {
		database := &mocks.DatabaseMock{}
		srv := testServer(t, cfg, database, &mocks.SchedulerMock{
			TriggerPreferenceUpdateFunc: func() {},
		})

		req := httptest.NewRequest("DELETE", "/api/v1/topics/test?type=invalid", http.NoBody)
		req.SetPathValue("topic", "test")
		w := httptest.NewRecorder()

		srv.deleteTopicHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestServer_PreferenceHandlers(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}

	t.Run("preferenceViewHandler", func(t *testing.T) {
		now := time.Now().UTC()
		database := &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				switch key {
				case domain.SettingPreferenceSummary:
					return "User prefers technical articles about Go programming", nil
				case domain.SettingPreferenceSummaryEnabled:
					return "true", nil
				case domain.SettingLastSummaryFeedbackCount:
					return "150", nil
				case domain.SettingPreferenceSummaryLastUpdate:
					return now.Format(time.RFC3339), nil
				default:
					return "", nil
				}
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/api/v1/preferences/view", http.NoBody)
		w := httptest.NewRecorder()

		srv.preferenceViewHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "User prefers technical articles about Go programming")
		assert.Contains(t, w.Body.String(), "150")
		assert.Contains(t, w.Body.String(), `checked`)
		assert.Contains(t, w.Body.String(), `<textarea name="summary" class="preference-summary-textarea" readonly`) // should be in view mode
	})

	t.Run("preferenceEditHandler", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				switch key {
				case domain.SettingPreferenceSummary:
					return "Existing preference summary", nil
				case domain.SettingPreferenceSummaryEnabled:
					return "false", nil
				case domain.SettingLastSummaryFeedbackCount:
					return "50", nil
				default:
					return "", nil
				}
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("GET", "/api/v1/preferences/edit", http.NoBody)
		w := httptest.NewRecorder()

		srv.preferenceEditHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Existing preference summary")
		assert.NotContains(t, w.Body.String(), `readonly`) // textarea should be editable
		assert.NotContains(t, w.Body.String(), `checked`)  // should not be checked when false
	})

	t.Run("preferenceSaveHandler", func(t *testing.T) {
		setCalls := make(map[string]string)
		database := &mocks.DatabaseMock{
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				setCalls[key] = value
				return nil
			},
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				// after save, return the updated values
				switch key {
				case domain.SettingPreferenceSummary:
					return "Updated preference summary", nil
				case domain.SettingPreferenceSummaryEnabled:
					return "true", nil
				case domain.SettingLastSummaryFeedbackCount:
					return "100", nil
				case domain.SettingPreferenceSummaryLastUpdate:
					return time.Now().UTC().Format(time.RFC3339), nil
				default:
					return "", nil
				}
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		form := url.Values{}
		form.Add("summary", "Updated preference summary")
		form.Add("enabled", "on")

		req := httptest.NewRequest("POST", "/api/v1/preferences/save", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.preferenceSaveHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "Updated preference summary", setCalls[domain.SettingPreferenceSummary])
		assert.Equal(t, "true", setCalls[domain.SettingPreferenceSummaryEnabled])
		assert.NotEmpty(t, setCalls[domain.SettingPreferenceSummaryLastUpdate])

		// should return view mode
		assert.Contains(t, w.Body.String(), "Updated preference summary")
		assert.Contains(t, w.Body.String(), `readonly`)
	})

	t.Run("preferenceSaveHandler with disabled", func(t *testing.T) {
		setCalls := make(map[string]string)
		database := &mocks.DatabaseMock{
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				setCalls[key] = value
				return nil
			},
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				return "", nil
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		form := url.Values{}
		form.Add("summary", "New summary")
		// no "enabled" field means disabled

		req := httptest.NewRequest("POST", "/api/v1/preferences/save", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.preferenceSaveHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "false", setCalls[domain.SettingPreferenceSummaryEnabled])
	})

	t.Run("preferenceSaveHandler database error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				if key == domain.SettingPreferenceSummary {
					return errors.New("database error")
				}
				return nil
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		form := url.Values{}
		form.Add("summary", "New summary")

		req := httptest.NewRequest("POST", "/api/v1/preferences/save", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		srv.preferenceSaveHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to save preferences")
	})

	t.Run("preferenceResetHandler", func(t *testing.T) {
		setCalls := make(map[string]string)
		database := &mocks.DatabaseMock{
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				setCalls[key] = value
				return nil
			},
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				// after reset, everything should be empty/default
				return "", nil
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("DELETE", "/api/v1/preferences/reset", http.NoBody)
		w := httptest.NewRecorder()

		srv.preferenceResetHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, setCalls[domain.SettingPreferenceSummary])
		assert.Equal(t, "0", setCalls[domain.SettingLastSummaryFeedbackCount])
		assert.Empty(t, setCalls[domain.SettingPreferenceSummaryLastUpdate])

		// should return empty view
		assert.Contains(t, w.Body.String(), "No preference summary yet")
	})

	t.Run("preferenceToggleHandler enable to disable", func(t *testing.T) {
		callCount := 0
		database := &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				callCount++
				if callCount == 1 && key == domain.SettingPreferenceSummaryEnabled {
					return "true", nil // currently enabled
				}
				if key == domain.SettingPreferenceSummaryEnabled {
					return "false", nil // after toggle
				}
				return "", nil
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				assert.Equal(t, domain.SettingPreferenceSummaryEnabled, key)
				assert.Equal(t, "false", value)
				return nil
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/preferences/toggle", http.NoBody)
		w := httptest.NewRecorder()

		srv.preferenceToggleHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `checked`) // should be unchecked after toggle
	})

	t.Run("preferenceToggleHandler disable to enable", func(t *testing.T) {
		callCount := 0
		database := &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				callCount++
				if callCount == 1 && key == domain.SettingPreferenceSummaryEnabled {
					return "false", nil // currently disabled
				}
				if key == domain.SettingPreferenceSummaryEnabled {
					return "true", nil // after toggle
				}
				return "", nil
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				assert.Equal(t, domain.SettingPreferenceSummaryEnabled, key)
				assert.Equal(t, "true", value)
				return nil
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/preferences/toggle", http.NoBody)
		w := httptest.NewRecorder()

		srv.preferenceToggleHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `checked`) // should be checked after toggle
	})

	t.Run("preferenceToggleHandler database error", func(t *testing.T) {
		database := &mocks.DatabaseMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				if key == domain.SettingPreferenceSummaryEnabled {
					return "", errors.New("database error")
				}
				return "", nil
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				return nil // won't be called due to early error
			},
		}

		scheduler := &mocks.SchedulerMock{}
		srv := testServer(t, cfg, database, scheduler)

		req := httptest.NewRequest("POST", "/api/v1/preferences/toggle", http.NoBody)
		w := httptest.NewRecorder()

		srv.preferenceToggleHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to get preference enabled status")
	})
}
