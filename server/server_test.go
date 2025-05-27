package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/feed/types"
	"github.com/umputun/newscope/server/mocks"
)

func TestServer_New(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	db := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, db, scheduler, "1.0.0", false)
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

	db := &mocks.DatabaseMock{
		GetFeedsFunc: func(ctx context.Context) ([]types.Feed, error) {
			return []types.Feed{}, nil
		},
		GetItemsFunc: func(ctx context.Context, limit, offset int) ([]types.Item, error) {
			return []types.Item{}, nil
		},
	}

	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, db, scheduler, "1.0.0", false)

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

func TestServer_statusHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	db := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, db, scheduler, "1.2.3", false)

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

func TestServer_rssFeedHandler(t *testing.T) {
	cfg := &mocks.ConfigProviderMock{
		GetServerConfigFunc: func() (string, time.Duration) {
			return ":8080", 30 * time.Second
		},
	}
	db := &mocks.DatabaseMock{}
	scheduler := &mocks.SchedulerMock{}

	srv := New(cfg, db, scheduler, "1.0.0", false)

	// create test request with path parameter
	req := httptest.NewRequest("GET", "/feed/technology", http.NoBody)
	req.SetPathValue("topic", "technology")
	w := httptest.NewRecorder()

	// call handler directly
	srv.rssFeedHandler(w, req)

	// check response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "RSS feed for topic: technology")
}

func TestRenderJSON(t *testing.T) {
	data := map[string]string{
		"message": "test",
		"status":  "ok",
	}

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	RenderJSON(w, req, http.StatusOK, data)

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

			RenderError(w, req, tt.err, tt.expectedCode)

			assert.Equal(t, tt.expectedCode, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var result map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &result)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedMsg, result["error"])
		})
	}
}
