package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

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
	scheduler := &mocks.SchedulerMock{}

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
