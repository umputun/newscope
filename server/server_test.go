package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
	scheduler := &mocks.SchedulerMock{
		TriggerPreferenceUpdateFunc: func() {
			// do nothing in tests
		},
	}

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
		GetFeedsFunc: func(ctx context.Context) ([]domain.Feed, error) {
			return []domain.Feed{}, nil
		},
		GetItemsFunc: func(ctx context.Context, limit, offset int) ([]domain.Item, error) {
			return []domain.Item{}, nil
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

func TestGeneratePageNumbers(t *testing.T) {
	tests := []struct {
		name        string
		currentPage int
		totalPages  int
		expected    []int
	}{
		{
			name:        "single page",
			currentPage: 1,
			totalPages:  1,
			expected:    []int{1},
		},
		{
			name:        "three pages, on first",
			currentPage: 1,
			totalPages:  3,
			expected:    []int{1, 2, 3},
		},
		{
			name:        "five pages, on third",
			currentPage: 3,
			totalPages:  5,
			expected:    []int{1, 2, 3, 4, 5},
		},
		{
			name:        "ten pages, on fifth",
			currentPage: 5,
			totalPages:  10,
			expected:    []int{3, 4, 5, 6, 7},
		},
		{
			name:        "ten pages, on first",
			currentPage: 1,
			totalPages:  10,
			expected:    []int{1, 2, 3, 4, 5},
		},
		{
			name:        "ten pages, on last",
			currentPage: 10,
			totalPages:  10,
			expected:    []int{6, 7, 8, 9, 10},
		},
		{
			name:        "zero pages",
			currentPage: 1,
			totalPages:  0,
			expected:    []int{},
		},
		{
			name:        "negative pages",
			currentPage: 1,
			totalPages:  -1,
			expected:    []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generatePageNumbers(tt.currentPage, tt.totalPages)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestServer_GetPageSize(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetFullConfigFunc: func() *config.Config {
			return &config.Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					PageSize: 25,
				},
			}
		},
	}

	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := &Server{
		config:    cfg,
		db:        database,
		scheduler: scheduler,
	}

	pageSize := srv.GetPageSize()
	assert.Equal(t, 25, pageSize)
}

func TestServer_SafeHTML(t *testing.T) {
	// test bluemonday sanitization through template rendering
	tests := []struct {
		name        string
		input       string
		contains    []string // what should be in the output
		notContains []string // what should NOT be in the output
	}{
		{
			name:     "safe HTML preserved",
			input:    `<p>Hello <strong>world</strong></p>`,
			contains: []string{`<p>Hello <strong>world</strong></p>`},
		},
		{
			name:        "script tag removed",
			input:       `<p>Hello</p><script>alert('xss')</script>`,
			contains:    []string{`<p>Hello</p>`},
			notContains: []string{`<script>`, `alert`},
		},
		{
			name:        "onclick attribute removed",
			input:       `<p onclick="alert('xss')">Click me</p>`,
			contains:    []string{`<p>Click me</p>`},
			notContains: []string{`onclick`, `alert`},
		},
		{
			name:     "safe attributes preserved",
			input:    `<a href="https://example.com" title="Example">Link</a>`,
			contains: []string{`href="https://example.com"`, `title="Example"`, `Link</a>`},
		},
		{
			name:        "javascript URL sanitized",
			input:       `<a href="javascript:alert('xss')">Bad Link</a>`,
			contains:    []string{`Bad Link</a>`},
			notContains: []string{`alert('xss')`}, // the dangerous part should be escaped
		},
		{
			name:     "class attributes on allowed elements",
			input:    `<div class="content"><p class="highlight">Text</p></div>`,
			contains: []string{`<div class="content">`, `<p class="highlight">`, `Text</p></div>`},
		},
		{
			name:     "blockquote and cite preserved",
			input:    `<blockquote><p>Quote</p><cite>Author</cite></blockquote>`,
			contains: []string{`<blockquote>`, `<p>Quote</p>`, `<cite>Author</cite>`, `</blockquote>`},
		},
	}

	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	database := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, database, scheduler, "1.0.0", false)

	// create a simple template to test the safeHTML function
	tmpl, err := srv.templates.New("test").Parse(`{{.Content | safeHTML}}`)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			err := tmpl.Execute(&buf, map[string]string{"Content": tt.input})
			require.NoError(t, err)

			result := buf.String()

			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, result, notExpected)
			}
		})
	}
}

func TestServer_respondWithError(t *testing.T) {
	tests := []struct {
		name           string
		code           int
		message        string
		err            error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "with error",
			code:           http.StatusInternalServerError,
			message:        "Something went wrong",
			err:            errors.New("database error"),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Something went wrong\n",
		},
		{
			name:           "without error",
			code:           http.StatusBadRequest,
			message:        "Invalid request",
			err:            nil,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid request\n",
		},
		{
			name:           "not found with error",
			code:           http.StatusNotFound,
			message:        "Resource not found",
			err:            errors.New("item not found"),
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Resource not found\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &Server{
				config:    &mocks.ConfigProviderMock{},
				db:        &mocks.DatabaseMock{},
				scheduler: &mocks.SchedulerMock{},
				router:    routegroup.New(http.NewServeMux()),
			}

			w := httptest.NewRecorder()
			srv.respondWithError(w, tt.code, tt.message, tt.err)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, tt.expectedBody, w.Body.String())
		})
	}
}
