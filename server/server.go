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
	"github.com/go-pkgz/routegroup"
	"github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
)

// Config defines server configuration parameters
type Config struct {
	Listen           string        // HTTP listen address
	ReadTimeout      time.Duration // HTTP read timeout
	WriteTimeout     time.Duration // HTTP write timeout
	ShutdownTimeout  time.Duration // Graceful shutdown timeout
	Version          string        // App version
	Debug            bool          // Debug mode
	BasicAuthEnabled bool          // Enable basic auth
	BasicAuthUser    string        // Basic auth username
	BasicAuthPass    string        // Basic auth password
}

// Server represents HTTP server instance
type Server struct {
	Config

	lock       sync.Mutex
	httpServer *http.Server
	router     *routegroup.Bundle
}

// New initializes a new server instance
func New(config Config) *Server {
	// set defaults
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 10 * time.Second
	}

	s := &Server{
		Config: config,
		router: routegroup.New(http.NewServeMux()),
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// Run starts the HTTP server and handles graceful shutdown
func (s *Server) Run(ctx context.Context) error {
	log.Printf("[INFO] starting server on %s", s.Listen)

	s.lock.Lock()
	s.httpServer = &http.Server{
		Addr:         s.Listen,
		Handler:      s.router,
		ReadTimeout:  s.ReadTimeout,
		WriteTimeout: s.WriteTimeout,
	}
	s.lock.Unlock()

	go func() {
		<-ctx.Done()
		log.Printf("[INFO] shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.ShutdownTimeout)
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
	s.router.Use(rest.AppInfo("newscope", "umputun", s.Version))
	s.router.Use(rest.Ping)
	
	if s.Debug {
		s.router.Use(logger.New(logger.Log(lgr.Default()), logger.Prefix("[DEBUG]")).Handler)
	}
	
	s.router.Use(rest.Recoverer(lgr.Default()))
	s.router.Use(rest.Throttle(100))
	s.router.Use(rest.SizeLimit(1024 * 1024)) // 1MB

	if s.BasicAuthEnabled {
		s.router.Use(rest.BasicAuthWithPrompt(s.BasicAuthUser, s.BasicAuthPass))
	}
}

// setupRoutes configures application routes
func (s *Server) setupRoutes() {
	// API routes
	s.router.Mount("/api/v1").Route(func(r *routegroup.Bundle) {
		r.HandleFunc("GET /status", s.statusHandler)
		// add more API endpoints here
	})

	// static files or UI routes can be added here
}

// statusHandler returns server status
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":  "ok",
		"version": s.Version,
		"time":    time.Now().UTC(),
	}
	RenderJSON(w, r, http.StatusOK, status)
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
	RenderJSON(w, r, code, map[string]string{"error": err.Error()})
}