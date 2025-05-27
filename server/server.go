package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
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

// Server represents HTTP server instance
type Server struct {
	config    ConfigProvider
	db        Database
	scheduler Scheduler
	version   string
	debug     bool

	lock       sync.Mutex
	httpServer *http.Server
	router     *routegroup.Bundle
}

// Database interface for server operations
type Database interface {
	GetFeeds(ctx context.Context) ([]types.Feed, error)
	GetItems(ctx context.Context, limit, offset int) ([]types.Item, error)
	GetItemsByFeed(ctx context.Context, feedID int64, limit, offset int) ([]types.Item, error)
	GetItemsWithContent(ctx context.Context, limit, offset int) ([]types.ItemWithContent, error)
	SearchItems(ctx context.Context, query string, limit, offset int) ([]types.Item, error)
}

// Scheduler interface for on-demand operations
type Scheduler interface {
	UpdateFeedNow(ctx context.Context, feedID int64) error
	ExtractContentNow(ctx context.Context, itemID int64) error
}

// ConfigProvider provides server configuration
type ConfigProvider interface {
	GetServerConfig() (listen string, timeout time.Duration)
}

// New initializes a new server instance
func New(cfg ConfigProvider, db Database, scheduler Scheduler, version string, debug bool) *Server {
	s := &Server{
		config:    cfg,
		db:        db,
		scheduler: scheduler,
		version:   version,
		debug:     debug,
		router:    routegroup.New(http.NewServeMux()),
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
	// API routes
	s.router.Mount("/api/v1").Route(func(r *routegroup.Bundle) {
		r.HandleFunc("GET /status", s.statusHandler)
		// add more API endpoints here
	})

	// RSS routes
	s.router.HandleFunc("GET /rss/{topic}", s.rssFeedHandler)

	// static files or UI routes can be added here
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

// RenderError sends error response as JSON
func RenderError(w http.ResponseWriter, r *http.Request, err error, code int) {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	RenderJSON(w, r, code, map[string]string{"error": errMsg})
}
