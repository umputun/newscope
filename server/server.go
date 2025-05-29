package server

import (
	"context"
	"embed"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
	"github.com/go-pkgz/routegroup"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
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
	GetFeeds(ctx context.Context) ([]types.Feed, error)
	GetItems(ctx context.Context, limit, offset int) ([]types.Item, error)
	GetClassifiedItems(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error)
	GetClassifiedItem(ctx context.Context, itemID int64) (*types.ItemWithClassification, error)
	UpdateItemFeedback(ctx context.Context, itemID int64, feedback string) error
	GetTopics(ctx context.Context) ([]string, error)
	GetAllFeeds(ctx context.Context) ([]db.Feed, error)
	CreateFeed(ctx context.Context, feed *db.Feed) error
	UpdateFeedStatus(ctx context.Context, feedID int64, enabled bool) error
	DeleteFeed(ctx context.Context, feedID int64) error
}

// Scheduler interface for on-demand operations
type Scheduler interface {
	UpdateFeedNow(ctx context.Context, feedID int64) error
	ExtractContentNow(ctx context.Context, itemID int64) error
}

// ConfigProvider provides server configuration
type ConfigProvider interface {
	GetServerConfig() (listen string, timeout time.Duration)
	GetFullConfig() *config.Config // returns the full config struct for display
}

// New initializes a new server instance
func New(cfg ConfigProvider, database Database, scheduler Scheduler, version string, debug bool) *Server {
	// template functions
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"printf": fmt.Sprintf,
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s) //nolint:gosec // we trust extracted content
		},
	}

	// parse component templates only
	templates := template.New("").Funcs(funcMap)

	// parse component templates that can be reused
	templates, err := templates.ParseFS(templateFS,
		"templates/article-card.html",
		"templates/feed-card.html",
		"templates/article-content.html")
	if err != nil {
		log.Printf("[ERROR] failed to parse templates: %v", err)
	}

	// parse page templates
	pageTemplates := make(map[string]*template.Template)
	pageNames := []string{"articles.html", "feeds.html", "settings.html"}

	for _, pageName := range pageNames {
		tmpl := template.New("").Funcs(funcMap)
		tmpl, err = tmpl.ParseFS(templateFS,
			"templates/base.html",
			"templates/"+pageName,
			"templates/article-card.html",
			"templates/feed-card.html")
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
	s.router.Use(rest.Throttle(100))
	s.router.Use(rest.SizeLimit(1024 * 1024)) // 1MB
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

	// API routes
	s.router.Mount("/api/v1").Route(func(r *routegroup.Bundle) {
		r.HandleFunc("GET /status", s.statusHandler)
		r.HandleFunc("POST /feedback/{id}/{action}", s.feedbackHandler)
		r.HandleFunc("POST /extract/{id}", s.extractHandler)
		r.HandleFunc("GET /articles/{id}/content", s.articleContentHandler)
		r.HandleFunc("GET /articles/{id}/hide", s.hideContentHandler)

		// feed management
		r.HandleFunc("POST /feeds", s.createFeedHandler)
		r.HandleFunc("POST /feeds/{id}/enable", s.enableFeedHandler)
		r.HandleFunc("POST /feeds/{id}/disable", s.disableFeedHandler)
		r.HandleFunc("POST /feeds/{id}/fetch", s.fetchFeedHandler)
		r.HandleFunc("DELETE /feeds/{id}", s.deleteFeedHandler)
	})

	// RSS routes
	s.router.HandleFunc("GET /rss/{topic}", s.rssHandler)
	s.router.HandleFunc("GET /rss", s.rssHandler)
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

// rssHandler serves RSS feed for articles
// Supports both /rss/{topic} and /rss?topic=... patterns
func (s *Server) rssHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get topic from path or query params
	topic := r.PathValue("topic")
	if topic == "" {
		topic = r.URL.Query().Get("topic")
	}

	// get min score from query params, default to 5.0
	minScore := 5.0
	if scoreStr := r.URL.Query().Get("min_score"); scoreStr != "" {
		if score, err := strconv.ParseFloat(scoreStr, 64); err == nil {
			minScore = score
		}
	}

	// get classified items
	items, err := s.db.GetClassifiedItems(ctx, minScore, topic, 100)
	if err != nil {
		log.Printf("[ERROR] failed to get items for RSS: %v", err)
		http.Error(w, "Failed to generate RSS feed", http.StatusInternalServerError)
		return
	}

	// create RSS feed
	rss := s.generateRSSFeed(topic, minScore, items)

	// set content type and write RSS
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	if _, err := w.Write([]byte(rss)); err != nil {
		log.Printf("[ERROR] failed to write RSS response: %v", err)
	}
}

// rss structs for XML encoding
type rssChannel struct {
	XMLName       xml.Name  `xml:"channel"`
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	AtomLink      atomLink  `xml:"http://www.w3.org/2005/Atom link"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	Description string   `xml:"description"`
	Author      string   `xml:"author,omitempty"`
	PubDate     string   `xml:"pubDate"`
	Categories  []string `xml:"category"`
}

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

// generateRSSFeed creates an RSS 2.0 feed from classified items
func (s *Server) generateRSSFeed(topic string, minScore float64, items []types.ItemWithClassification) string {
	// determine title
	var title string
	if topic != "" {
		title = fmt.Sprintf("Newscope - %s (Score ≥ %.1f)", topic, minScore)
	} else {
		title = fmt.Sprintf("Newscope - All Topics (Score ≥ %.1f)", minScore)
	}

	// build self link
	selfLink := "http://localhost:8080/rss"
	if topic != "" {
		selfLink = fmt.Sprintf("http://localhost:8080/rss/%s", topic)
	}

	// convert items to RSS items
	rssItems := make([]rssItem, 0, len(items))
	for _, item := range items {
		// build description
		desc := fmt.Sprintf("Score: %.1f/10 - %s", item.RelevanceScore, item.Explanation)
		if len(item.Topics) > 0 {
			desc += fmt.Sprintf("\nTopics: %s", strings.Join(item.Topics, ", "))
		}
		if item.Description != "" {
			desc += "\n\n" + item.Description
		}

		rssItems = append(rssItems, rssItem{
			Title:       fmt.Sprintf("[%.1f] %s", item.RelevanceScore, item.Title),
			Link:        item.Link,
			GUID:        item.GUID,
			Description: desc,
			Author:      item.Author,
			PubDate:     item.Published.Format(time.RFC1123Z),
			Categories:  item.Topics,
		})
	}

	// create RSS structure
	feed := rss{
		Version: "2.0",
		Atom:    "http://www.w3.org/2005/Atom",
		Channel: rssChannel{
			Title:         title,
			Link:          "http://localhost:8080/",
			Description:   fmt.Sprintf("AI-curated articles with relevance score ≥ %.1f", minScore),
			AtomLink:      atomLink{Href: selfLink, Rel: "self", Type: "application/rss+xml"},
			LastBuildDate: time.Now().Format(time.RFC1123Z),
			Items:         rssItems,
		},
	}

	// marshal to XML
	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		log.Printf("[ERROR] failed to marshal RSS: %v", err)
		return ""
	}

	// add XML declaration
	return xml.Header + string(output)
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

// createFeedHandler handles feed creation
func (s *Server) createFeedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// parse form data
	err := r.ParseForm()
	if err != nil {
		RenderError(w, r, fmt.Errorf("invalid form data"), http.StatusBadRequest)
		return
	}

	url := r.FormValue("url")
	if url == "" {
		RenderError(w, r, fmt.Errorf("feed URL is required"), http.StatusBadRequest)
		return
	}

	// parse fetch interval
	fetchInterval := 1800 // default 30 minutes
	if intervalStr := r.FormValue("fetch_interval"); intervalStr != "" {
		if minutes, err := strconv.Atoi(intervalStr); err == nil {
			fetchInterval = minutes * 60 // convert to seconds
		}
	}

	feed := &db.Feed{
		URL:           url,
		Title:         r.FormValue("title"),
		FetchInterval: fetchInterval,
		Enabled:       true,
	}

	// create feed in database
	if err := s.db.CreateFeed(ctx, feed); err != nil {
		log.Printf("[ERROR] failed to create feed: %v", err)
		RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// trigger immediate fetch
	go func() {
		if err := s.scheduler.UpdateFeedNow(context.Background(), feed.ID); err != nil {
			log.Printf("[ERROR] failed to fetch new feed: %v", err)
		}
	}()

	// return the feed card HTML for HTMX
	s.renderFeedCard(w, feed)
}

// renderFeedCard renders a single feed card
func (s *Server) renderFeedCard(w http.ResponseWriter, feed *db.Feed) {
	if err := s.templates.ExecuteTemplate(w, "feed-card.html", feed); err != nil {
		log.Printf("[ERROR] failed to render feed card: %v", err)
		http.Error(w, "Failed to render feed", http.StatusInternalServerError)
	}
}

// enableFeedHandler enables a feed
func (s *Server) enableFeedHandler(w http.ResponseWriter, r *http.Request) {
	s.updateFeedStatus(w, r, true)
}

// disableFeedHandler disables a feed
func (s *Server) disableFeedHandler(w http.ResponseWriter, r *http.Request) {
	s.updateFeedStatus(w, r, false)
}

// updateFeedStatus updates feed enabled status
func (s *Server) updateFeedStatus(w http.ResponseWriter, r *http.Request, enabled bool) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		RenderError(w, r, fmt.Errorf("invalid feed ID"), http.StatusBadRequest)
		return
	}

	// update status
	if err := s.db.UpdateFeedStatus(ctx, id, enabled); err != nil {
		log.Printf("[ERROR] failed to update feed status: %v", err)
		RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// get updated feed
	feeds, err := s.db.GetAllFeeds(ctx)
	if err != nil {
		http.Error(w, "Failed to reload feed", http.StatusInternalServerError)
		return
	}

	// find the updated feed
	for _, feed := range feeds {
		if feed.ID == id {
			s.renderFeedCard(w, &feed)
			return
		}
	}

	http.Error(w, "Feed not found", http.StatusNotFound)
}

// fetchFeedHandler triggers immediate feed fetch
func (s *Server) fetchFeedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		RenderError(w, r, fmt.Errorf("invalid feed ID"), http.StatusBadRequest)
		return
	}

	// trigger fetch
	if err := s.scheduler.UpdateFeedNow(ctx, id); err != nil {
		log.Printf("[ERROR] failed to fetch feed: %v", err)
		RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// deleteFeedHandler deletes a feed
func (s *Server) deleteFeedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		RenderError(w, r, fmt.Errorf("invalid feed ID"), http.StatusBadRequest)
		return
	}

	// delete feed
	if err := s.db.DeleteFeed(ctx, id); err != nil {
		log.Printf("[ERROR] failed to delete feed: %v", err)
		RenderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// return empty response for HTMX to remove the element
	w.WriteHeader(http.StatusOK)
}

// renderPage renders a pre-parsed page template
func (s *Server) renderPage(w http.ResponseWriter, templateName string, data interface{}) error {
	// get the pre-parsed template
	tmpl, ok := s.pageTemplates[templateName]
	if !ok {
		return fmt.Errorf("template %s not found", templateName)
	}

	// execute the template
	return tmpl.ExecuteTemplate(w, templateName, data)
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

	// check if this is an HTMX request for partial update
	if r.Header.Get("HX-Request") == "true" {
		// for HTMX requests, only render the articles list
		for _, article := range articles {
			s.renderArticleCard(w, &article)
		}
		if len(articles) == 0 {
			if _, err := w.Write([]byte(`<p class="no-articles">No articles found. Try lowering the score filter or wait for classification to run.</p>`)); err != nil {
				log.Printf("[ERROR] failed to write no articles message: %v", err)
			}
		}
		return
	}

	// prepare template data for full page render
	data := struct {
		ActivePage    string
		Articles      []types.ItemWithClassification
		Topics        []string
		MinScore      float64
		SelectedTopic string
	}{
		ActivePage:    "home",
		Articles:      articles,
		Topics:        topics,
		MinScore:      minScore,
		SelectedTopic: topic,
	}

	// render full page with base template and article card component
	if err := s.renderPage(w, "articles.html", data); err != nil {
		log.Printf("[ERROR] failed to render articles page: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// feedsHandler displays the feeds management page
func (s *Server) feedsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get all feeds from database
	feeds, err := s.db.GetAllFeeds(ctx)
	if err != nil {
		log.Printf("[ERROR] failed to get feeds: %v", err)
		http.Error(w, "Failed to load feeds", http.StatusInternalServerError)
		return
	}

	// prepare template data
	data := struct {
		ActivePage string
		Feeds      []db.Feed
	}{
		ActivePage: "feeds",
		Feeds:      feeds,
	}

	// render page with base template
	if err := s.renderPage(w, "feeds.html", data); err != nil {
		log.Printf("[ERROR] failed to render feeds page: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// settingsHandler displays the settings page
func (s *Server) settingsHandler(w http.ResponseWriter, _ *http.Request) {
	// get full configuration
	cfg := s.config.GetFullConfig()

	// prepare data for display
	data := struct {
		ActivePage string
		Config     *config.Config
		Version    string
		Debug      bool
	}{
		ActivePage: "settings",
		Config:     cfg,
		Version:    s.version,
		Debug:      s.debug,
	}

	// render settings page
	if err := s.renderPage(w, "settings.html", data); err != nil {
		log.Printf("[ERROR] failed to render settings page: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
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

	// render the content template
	if err := s.templates.ExecuteTemplate(w, "article-content.html", article); err != nil {
		log.Printf("[ERROR] failed to render article content: %v", err)
		http.Error(w, "Failed to render content", http.StatusInternalServerError)
		return
	}

	// also send out-of-band update for the button
	fmt.Fprintf(w, `<span id="content-toggle-%d" hx-swap-oob="true">
		<button class="btn-content"
			hx-get="/api/v1/articles/%d/hide"
			hx-target="#content-%d"
			hx-swap="innerHTML">
			Hide Content
		</button>
	</span>`, id, id, id)
}

// hideContentHandler returns the hidden state for article content
func (s *Server) hideContentHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	// clear the content div
	if _, err := w.Write([]byte("")); err != nil {
		log.Printf("[ERROR] failed to write response: %v", err)
	}

	// also send out-of-band update for the button
	fmt.Fprintf(w, `<span id="content-toggle-%d" hx-swap-oob="true">
		<button class="btn-content"
			hx-get="/api/v1/articles/%d/content"
			hx-target="#content-%d"
			hx-swap="innerHTML">
			Show Content
		</button>
	</span>`, id, id, id)
}
