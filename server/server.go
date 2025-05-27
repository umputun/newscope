package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
	"github.com/go-pkgz/routegroup"

	"github.com/umputun/newscope/pkg/feed/types"
)

//go:generate moq -out mocks/config.go -pkg mocks -skip-ensure -fmt goimports . ConfigProvider
//go:generate moq -out mocks/database.go -pkg mocks -skip-ensure -fmt goimports . Database
//go:generate moq -out mocks/scheduler.go -pkg mocks -skip-ensure -fmt goimports . Scheduler

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server represents HTTP server instance
type Server struct {
	config    ConfigProvider
	db        Database
	scheduler Scheduler
	version   string
	debug     bool
	templates *template.Template

	lock       sync.Mutex
	httpServer *http.Server
	router     *routegroup.Bundle
}

// Database interface for server operations
type Database interface {
	GetFeeds(ctx context.Context) ([]types.Feed, error)
	GetItems(ctx context.Context, limit, offset int) ([]types.Item, error)
	GetClassifiedItems(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error)
	GetClassifiedItem(ctx context.Context, itemID int64) (*types.ItemWithClassification, error)
	UpdateItemFeedback(ctx context.Context, itemID int64, feedback string) error
	GetTopics(ctx context.Context) ([]string, error)
}

// Scheduler interface for on-demand operations
type Scheduler interface {
	UpdateFeedNow(ctx context.Context, feedID int64) error
	ExtractContentNow(ctx context.Context, itemID int64) error
	ClassifyNow(ctx context.Context) error
}

// ConfigProvider provides server configuration
type ConfigProvider interface {
	GetServerConfig() (listen string, timeout time.Duration)
}

// New initializes a new server instance
func New(cfg ConfigProvider, db Database, scheduler Scheduler, version string, debug bool) *Server {
	// template functions
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"printf": fmt.Sprintf,
	}

	// parse templates
	templates, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Printf("[ERROR] failed to parse templates: %v", err)
	}

	s := &Server{
		config:    cfg,
		db:        db,
		scheduler: scheduler,
		version:   version,
		debug:     debug,
		router:    routegroup.New(http.NewServeMux()),
		templates: templates,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// Run starts the HTTP server and handles graceful shutdown
func (s *Server) Run(ctx context.Context) error {
	listen, timeout := s.config.GetServerConfig()
	log.Printf("[INFO] starting server on %s", listen)

	s.lock.Lock()
	s.httpServer = &http.Server{
		Addr:         listen,
		Handler:      s.router,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
	}
	s.lock.Unlock()

	go func() {
		<-ctx.Done()
		log.Printf("[INFO] shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("[WARN] server shutdown error: %v", err)
		}
	}()

	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
	s.router.Use(rest.Throttle(100))
	s.router.Use(rest.SizeLimit(1024 * 1024)) // 1MB
}

// setupRoutes configures application routes
func (s *Server) setupRoutes() {
	// serve static files
	s.router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// web UI routes
	s.router.HandleFunc("GET /", s.articlesHandler)
	s.router.HandleFunc("GET /articles", s.articlesHandler)
	s.router.HandleFunc("GET /feeds", s.feedsHandler)
	s.router.HandleFunc("GET /settings", s.settingsHandler)

	// API routes
	s.router.Mount("/api/v1").Route(func(r *routegroup.Bundle) {
		r.HandleFunc("GET /status", s.statusHandler)
		r.HandleFunc("POST /feedback/{id}/{action}", s.feedbackHandler)
		r.HandleFunc("POST /extract/{id}", s.extractHandler)
		r.HandleFunc("POST /classify-now", s.classifyNowHandler)
		r.HandleFunc("GET /articles/{id}/content", s.articleContentHandler)
	})

	// RSS routes
	s.router.HandleFunc("GET /rss/{topic}", s.rssFeedHandler)
}

// statusHandler returns server status
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":  "ok",
		"version": s.version,
		"time":    time.Now().UTC(),
	}
	RenderJSON(w, r, http.StatusOK, status)
}

// rssFeedHandler serves RSS feed for a specific topic
func (s *Server) rssFeedHandler(w http.ResponseWriter, r *http.Request) {
	topic := r.PathValue("topic")

	// TODO: implement actual RSS generation based on topic
	fmt.Fprintf(w, "RSS feed for topic: %s", topic)
}

// RenderJSON sends JSON response
func RenderJSON(w http.ResponseWriter, _ *http.Request, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("[ERROR] can't encode response to JSON: %v", err)
		}
	}
}

// renderArticleCard renders a single article card as HTML
func (s *Server) renderArticleCard(w http.ResponseWriter, article *types.ItemWithClassification) {
	if err := s.templates.ExecuteTemplate(w, "article-card.html", article); err != nil {
		log.Printf("[ERROR] failed to render article card: %v", err)
		http.Error(w, "Failed to render article", http.StatusInternalServerError)
	}
}

// RenderError sends error response as JSON
func RenderError(w http.ResponseWriter, r *http.Request, err error, code int) {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	RenderJSON(w, r, code, map[string]string{"error": errMsg})
}

// articlesHandler displays the main articles page
func (s *Server) articlesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get query parameters
	minScore := 0.0
	if scoreStr := r.URL.Query().Get("score"); scoreStr != "" {
		if score, err := strconv.ParseFloat(scoreStr, 64); err == nil {
			minScore = score
		}
	}
	topic := r.URL.Query().Get("topic")

	// get articles with classification
	articles, err := s.db.GetClassifiedItems(ctx, minScore, topic, 100)
	if err != nil {
		log.Printf("[ERROR] failed to get classified items: %v", err)
		http.Error(w, "Failed to load articles", http.StatusInternalServerError)
		return
	}

	// get all topics for filter
	topics, err := s.db.GetTopics(ctx)
	if err != nil {
		log.Printf("[ERROR] failed to get topics: %v", err)
		topics = []string{} // continue with empty topics
	}

	// prepare template data
	data := struct {
		ActivePage     string
		Articles       []types.ItemWithClassification
		Topics         []string
		MinScore       float64
		SelectedTopic  string
	}{
		ActivePage:    "home",
		Articles:      articles,
		Topics:        topics,
		MinScore:      minScore,
		SelectedTopic: topic,
	}

	if err := s.templates.ExecuteTemplate(w, "articles.html", data); err != nil {
		log.Printf("[ERROR] failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// feedsHandler displays the feeds management page
func (s *Server) feedsHandler(w http.ResponseWriter, _ *http.Request) {
	// TODO: implement feeds page
	http.Error(w, "Feeds page not implemented yet", http.StatusNotImplemented)
}

// settingsHandler displays the settings page
func (s *Server) settingsHandler(w http.ResponseWriter, _ *http.Request) {
	// TODO: implement settings page
	http.Error(w, "Settings page not implemented yet", http.StatusNotImplemented)
}

// feedbackHandler handles user feedback (like/dislike)
func (s *Server) feedbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	idStr := r.PathValue("id")
	action := r.PathValue("action")
	
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		RenderError(w, r, fmt.Errorf("invalid item ID"), http.StatusBadRequest)
		return
	}

	// validate action
	if action != "like" && action != "dislike" {
		RenderError(w, r, fmt.Errorf("invalid action"), http.StatusBadRequest)
		return
	}

	// update feedback
	if err := s.db.UpdateItemFeedback(ctx, id, action); err != nil {
		log.Printf("[ERROR] failed to update feedback: %v", err)
		RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// get the updated article
	article, err := s.db.GetClassifiedItem(ctx, id)
	if err != nil {
		log.Printf("[ERROR] failed to get article after feedback: %v", err)
		http.Error(w, "Failed to reload article", http.StatusInternalServerError)
		return
	}

	// for HTMX, return the updated article card HTML
	s.renderArticleCard(w, article)
}

// extractHandler triggers content extraction for an item
func (s *Server) extractHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		RenderError(w, r, fmt.Errorf("invalid item ID"), http.StatusBadRequest)
		return
	}

	// trigger extraction
	if err := s.scheduler.ExtractContentNow(ctx, id); err != nil {
		log.Printf("[ERROR] failed to extract content: %v", err)
		RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// get the updated article
	article, err := s.db.GetClassifiedItem(ctx, id)
	if err != nil {
		log.Printf("[ERROR] failed to get article after extraction: %v", err)
		http.Error(w, "Failed to reload article", http.StatusInternalServerError)
		return
	}

	// for HTMX, return the updated article card HTML
	s.renderArticleCard(w, article)
}

// classifyNowHandler triggers immediate classification
func (s *Server) classifyNowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// trigger classification
	if err := s.scheduler.ClassifyNow(ctx); err != nil {
		log.Printf("[ERROR] failed to trigger classification: %v", err)
		RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// articleContentHandler returns extracted content for an article
func (s *Server) articleContentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	// get the article with classification
	article, err := s.db.GetClassifiedItem(ctx, id)
	if err != nil {
		log.Printf("[ERROR] failed to get article: %v", err)
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// return the content as HTML for HTMX
	if article.ExtractedContent != "" {
		fmt.Fprintf(w, `<div class="extracted-content">
			<h4>Full Article</h4>
			<div class="content-text">%s</div>
			<button onclick="this.parentElement.style.display='none'" class="close-btn">Close</button>
		</div>`, template.HTMLEscapeString(article.ExtractedContent))
	} else if article.ExtractionError != "" {
		fmt.Fprintf(w, `<div class="extraction-error">
			<p>Failed to extract content: %s</p>
		</div>`, template.HTMLEscapeString(article.ExtractionError))
	} else {
		fmt.Fprint(w, `<div class="no-content">
			<p>No extracted content available. Click "Extract Content" to fetch the full article.</p>
		</div>`)
	}
}
