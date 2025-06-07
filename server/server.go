package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
	"github.com/go-pkgz/routegroup"
	"github.com/microcosm-cc/bluemonday"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/domain"
)

const (
	// server configuration
	defaultThrottleLimit = 100
	defaultSizeLimit     = 1024 * 1024 // 1MB

	// RSS feed defaults
	defaultMinScore = 5.0
	defaultRSSLimit = 100
	defaultBaseURL  = "http://localhost:8080"

	// feed defaults
	defaultFetchInterval = 1800 // 30 minutes in seconds
	minutesToSeconds     = 60

	// article pagination
)

//go:generate moq -out mocks/config.go -pkg mocks -skip-ensure -fmt goimports . ConfigProvider
//go:generate moq -out mocks/database.go -pkg mocks -skip-ensure -fmt goimports . Database
//go:generate moq -out mocks/scheduler.go -pkg mocks -skip-ensure -fmt goimports . Scheduler

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server represents HTTP server instance
type Server struct {
	config        ConfigProvider
	db            Database
	scheduler     Scheduler
	version       string
	debug         bool
	templates     *template.Template
	pageTemplates map[string]*template.Template
	router        *routegroup.Bundle
}

// Database interface for server operations
type Database interface {
	GetFeeds(ctx context.Context) ([]domain.Feed, error)
	GetItems(ctx context.Context, limit, offset int) ([]domain.Item, error)
	GetClassifiedItems(ctx context.Context, minScore float64, topic string, limit int) ([]domain.ItemWithClassification, error)
	GetClassifiedItemsWithFilters(ctx context.Context, req domain.ArticlesRequest) ([]domain.ItemWithClassification, error)
	GetClassifiedItemsCount(ctx context.Context, req domain.ArticlesRequest) (int, error)
	GetClassifiedItem(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error)
	UpdateItemFeedback(ctx context.Context, itemID int64, feedback string) error
	GetTopics(ctx context.Context) ([]string, error)
	GetTopicsFiltered(ctx context.Context, minScore float64) ([]string, error)
	GetTopTopicsByScore(ctx context.Context, minScore float64, limit int) ([]domain.TopicWithScore, error)
	GetActiveFeedNames(ctx context.Context, minScore float64) ([]string, error)
	GetAllFeeds(ctx context.Context) ([]domain.Feed, error)
	CreateFeed(ctx context.Context, feed *domain.Feed) error
	UpdateFeed(ctx context.Context, feedID int64, title string, fetchInterval int) error
	UpdateFeedStatus(ctx context.Context, feedID int64, enabled bool) error
	DeleteFeed(ctx context.Context, feedID int64) error
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Scheduler interface for on-demand operations
type Scheduler interface {
	UpdateFeedNow(ctx context.Context, feedID int64) error
	ExtractContentNow(ctx context.Context, itemID int64) error
	UpdatePreferenceSummary(ctx context.Context) error
	TriggerPreferenceUpdate()
}

// ConfigProvider provides server configuration
type ConfigProvider interface {
	GetServerConfig() (listen string, timeout time.Duration)
	GetFullConfig() *config.Config // returns the full config struct for display
}

// GetPageSize returns the configured page size for pagination
func (s *Server) GetPageSize() int {
	cfg := s.config.GetFullConfig()
	return cfg.Server.PageSize
}

// generatePageNumbers creates a slice of page numbers for pagination display
func generatePageNumbers(currentPage, totalPages int) []int {
	if totalPages <= 0 {
		return []int{}
	}

	var pages []int

	// show up to 5 page numbers centered around current page
	start := currentPage - 2
	end := currentPage + 2

	// adjust bounds
	if start < 1 {
		start = 1
		end = start + 4
	}
	if end > totalPages {
		end = totalPages
		start = end - 4
		if start < 1 {
			start = 1
		}
	}

	for i := start; i <= end; i++ {
		pages = append(pages, i)
	}

	return pages
}

// New initializes a new server instance
func New(cfg ConfigProvider, database Database, scheduler Scheduler, version string, debug bool) *Server {
	// create bluemonday policy for HTML sanitization
	htmlPolicy := bluemonday.UGCPolicy()
	// allow additional safe elements that might be in article content
	htmlPolicy.AllowAttrs("class").OnElements("div", "span", "p", "code", "pre")
	htmlPolicy.AllowElements("figure", "figcaption", "blockquote", "cite")

	// preserve whitespace in pre and code blocks for proper formatting
	htmlPolicy.AllowAttrs("style").OnElements("pre", "code")
	htmlPolicy.RequireParseableURLs(false) // allow data: URLs for syntax highlighting

	// template functions
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"printf": fmt.Sprintf,
		"safeHTML": func(s string) template.HTML {
			// fix common content extraction issues before sanitization

			// 1. handle double code tags - these should be pre+code blocks
			codeBlockRe := regexp.MustCompile(`<code><code>([\s\S]*?)</code></code>`)
			s = codeBlockRe.ReplaceAllString(s, "<pre><code>$1</code></pre>")

			// 2. fix standalone multi-line code blocks that should be in pre tags
			// look for code blocks that contain newlines or look like code (has braces, semicolons, etc)
			standaloneCodeRe := regexp.MustCompile(`<code>((?:[^<]*(?:[\n\r]|[{};])[^<]*)+)</code>`)
			s = standaloneCodeRe.ReplaceAllStringFunc(s, func(match string) string {
				// extract the content
				content := match[6 : len(match)-7] // remove <code> and </code>
				// check if it looks like a code block (has newlines or typical code syntax)
				if strings.Contains(content, "\n") ||
					(strings.Contains(content, "{") && strings.Contains(content, "}")) ||
					strings.Contains(content, ");") {
					return "<pre><code>" + content + "</code></pre>"
				}
				return match // leave inline code as is
			})

			// 3. ensure proper nesting - no code directly inside code
			s = strings.ReplaceAll(s, "<code><code>", "<code>")
			s = strings.ReplaceAll(s, "</code></code>", "</code>")

			// sanitize HTML content before rendering
			sanitized := htmlPolicy.Sanitize(s)
			return template.HTML(sanitized) //nolint:gosec // content is sanitized by bluemonday
		},
	}

	// parse component templates only
	templates := template.New("").Funcs(funcMap)

	// parse component templates that can be reused
	templates, err := templates.ParseFS(templateFS,
		"templates/article-card.html",
		"templates/feed-card.html",
		"templates/article-content.html",
		"templates/pagination.html",
		"templates/topic-tags.html",
		"templates/topic-dropdowns.html")
	if err != nil {
		log.Printf("[ERROR] failed to parse templates: %v", err)
	}

	// parse page templates
	pageTemplates := make(map[string]*template.Template)
	pageNames := []string{"articles.html", "feeds.html", "settings.html", "rss-help.html"}

	for _, pageName := range pageNames {
		tmpl := template.New("").Funcs(funcMap)
		tmpl, err = tmpl.ParseFS(templateFS,
			"templates/base.html",
			"templates/"+pageName,
			"templates/article-card.html",
			"templates/feed-card.html",
			"templates/pagination.html")
		if err != nil {
			log.Printf("[ERROR] failed to parse %s: %v", pageName, err)
			continue
		}
		pageTemplates[pageName] = tmpl
	}

	s := &Server{
		config:        cfg,
		db:            database,
		scheduler:     scheduler,
		version:       version,
		debug:         debug,
		router:        routegroup.New(http.NewServeMux()),
		templates:     templates,
		pageTemplates: pageTemplates,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// Run starts the HTTP server and handles graceful shutdown
func (s *Server) Run(ctx context.Context) error {
	listen, timeout := s.config.GetServerConfig()
	log.Printf("[INFO] starting server on %s", listen)

	httpServer := &http.Server{
		Addr:         listen,
		Handler:      s.router,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
	}

	go func() {
		<-ctx.Done()
		log.Printf("[INFO] shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("[WARN] server shutdown error: %v", err)
		}
	}()

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server error: %w", err)
	}

	return nil
}

// setupMiddleware configures standard middleware for the server
func (s *Server) setupMiddleware() {
	s.router.Use(rest.AppInfo("newscope", "umputun", s.version))
	s.router.Use(rest.Ping)

	if s.debug {
		s.router.Use(logger.New(logger.Log(lgr.Default()), logger.Prefix("[DEBUG]")).Handler)
	}

	s.router.Use(rest.Recoverer(lgr.Default()))
	s.router.Use(rest.Throttle(defaultThrottleLimit))
	s.router.Use(rest.SizeLimit(defaultSizeLimit))
}

// setupRoutes configures application routes
func (s *Server) setupRoutes() {
	// serve static files
	s.router.HandleFunc("GET /static/{path...}", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/static/")
		if path == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, staticFS, "static/"+path)
	})

	// web UI routes
	s.router.HandleFunc("GET /", s.articlesHandler)
	s.router.HandleFunc("GET /articles", s.articlesHandler)
	s.router.HandleFunc("GET /feeds", s.feedsHandler)
	s.router.HandleFunc("GET /settings", s.settingsHandler)
	s.router.HandleFunc("GET /rss-help", s.rssHelpHandler)
	s.router.HandleFunc("GET /api/v1/rss-builder", s.rssBuilderHandler)

	// API routes
	s.router.Mount("/api/v1").Route(func(r *routegroup.Bundle) {
		r.HandleFunc("GET /status", s.statusHandler)
		r.HandleFunc("POST /feedback/{id}/{action}", s.feedbackHandler)
		r.HandleFunc("POST /extract/{id}", s.extractHandler)
		r.HandleFunc("GET /articles/{id}/content", s.articleContentHandler)
		r.HandleFunc("GET /articles/{id}/hide", s.hideContentHandler)

		// feed management
		r.HandleFunc("POST /feeds", s.createFeedHandler)
		r.HandleFunc("PUT /feeds/{id}", s.updateFeedHandler)
		r.HandleFunc("POST /feeds/{id}/enable", s.enableFeedHandler)
		r.HandleFunc("POST /feeds/{id}/disable", s.disableFeedHandler)
		r.HandleFunc("POST /feeds/{id}/fetch", s.fetchFeedHandler)
		r.HandleFunc("DELETE /feeds/{id}", s.deleteFeedHandler)

		// topic preferences management
		r.HandleFunc("POST /topics", s.addTopicHandler)
		r.HandleFunc("DELETE /topics/{topic}", s.deleteTopicHandler)
	})

	// RSS routes
	s.router.HandleFunc("GET /rss/{topic}", s.rssHandler)
	s.router.HandleFunc("GET /rss", s.rssHandler)
}
