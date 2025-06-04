package server

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-pkgz/routegroup"
	"github.com/stretchr/testify/assert"

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

	// test liked filter
	database.GetClassifiedItemsWithFiltersFunc = func(ctx context.Context, req domain.ArticlesRequest) ([]domain.ItemWithClassification, error) {
		// verify that ShowLikedOnly is passed correctly
		if req.ShowLikedOnly {
			return []domain.ItemWithClassification{
				{
					Title:          "Liked Article",
					Link:           "https://example.com/liked",
					Description:    "A liked article",
					Published:      now,
					ID:             2,
					FeedName:       "Test Feed",
					RelevanceScore: 9.0,
					Explanation:    "User liked this",
					Topics:         []string{"favorites"},
					ClassifiedAt:   &classifiedAt,
					UserFeedback:   "like",
				},
			}, nil
		}
		return []domain.ItemWithClassification{}, nil
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
}

// template error tests for render functions

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
	})

	t.Run("SettingsHandler template error", func(t *testing.T) {
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
	})

	t.Run("ArticleContentHandler template error", func(t *testing.T) {
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
