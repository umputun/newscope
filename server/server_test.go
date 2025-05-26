package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/feed/types"
)

// mockConfigProvider implements ConfigProvider interface
type mockConfigProvider struct {
	listen  string
	timeout time.Duration
}

func (m *mockConfigProvider) GetServerConfig() (string, time.Duration) {
	return m.listen, m.timeout
}

// mockFeedManager implements FeedManager interface
type mockFeedManager struct {
	fetchAllErr error
	items       []types.ExtractedItem
}

func (m *mockFeedManager) FetchAll(ctx context.Context) error {
	return m.fetchAllErr
}

func (m *mockFeedManager) GetItems() []types.ExtractedItem {
	return m.items
}

func TestServer_New(t *testing.T) {
	cfg := &mockConfigProvider{
		listen:  ":8080",
		timeout: 30 * time.Second,
	}
	mgr := &mockFeedManager{}

	srv := New(cfg, mgr, "1.0.0", false)
	assert.NotNil(t, srv)
	assert.Equal(t, cfg, srv.config)
	assert.Equal(t, mgr, srv.feedManager)
	assert.Equal(t, "1.0.0", srv.version)
	assert.False(t, srv.debug)
	assert.NotNil(t, srv.router)
}

func TestServer_Run(t *testing.T) {
	t.Run("successful run and shutdown", func(t *testing.T) {
		cfg := &mockConfigProvider{
			listen:  ":0", // use random port
			timeout: 30 * time.Second,
		}
		mgr := &mockFeedManager{}

		srv := New(cfg, mgr, "1.0.0", false)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.Run(ctx)
		}()

		// wait for server to start
		time.Sleep(100 * time.Millisecond)

		// trigger shutdown
		cancel()

		// wait for shutdown
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("server did not shut down in time")
		}
	})
}

// getTestServer starts a test server on a random port and returns the base URL
func getTestServer(t *testing.T, cfg ConfigProvider, mgr FeedManager, version string, debug bool) (baseURL string, cleanup func()) {
	// find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// update config to use the free port
	testCfg := &mockConfigProvider{
		listen:  fmt.Sprintf("127.0.0.1:%d", port),
		timeout: 30 * time.Second,
	}

	srv := New(testCfg, mgr, version, debug)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if err := srv.Run(ctx); err != nil {
			t.Logf("server run error: %v", err)
		}
	}()

	// wait for server to start
	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	for i := 0; i < 50; i++ {
		if _, err := http.Get(baseURL + "/api/v1/status"); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cleanup = cancel
	return baseURL, cleanup
}

func TestServer_StatusHandler(t *testing.T) {
	mgr := &mockFeedManager{}
	baseURL, cleanup := getTestServer(t, nil, mgr, "1.2.3", false)
	defer cleanup()

	// test status endpoint
	resp, err := http.Get(baseURL + "/api/v1/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var status map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&status)
	require.NoError(t, err)

	assert.Equal(t, "ok", status["status"])
	assert.Equal(t, "1.2.3", status["version"])
	assert.NotEmpty(t, status["time"])
}

func TestServer_RSSFeedHandler(t *testing.T) {
	mgr := &mockFeedManager{
		items: []types.ExtractedItem{
			{FeedItem: types.FeedItem{FeedName: "Test", Title: "Article 1", URL: "https://example.com/1"}},
		},
	}

	baseURL, cleanup := getTestServer(t, nil, mgr, "1.0.0", false)
	defer cleanup()

	// test RSS endpoint
	resp, err := http.Get(baseURL + "/rss/technology")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "RSS feed for topic: technology")
}

func TestRenderJSON(t *testing.T) {
	t.Run("successful render", func(t *testing.T) {
		w := &mockResponseWriter{
			headers: make(http.Header),
			body:    []byte{},
		}

		data := map[string]string{"message": "hello"}
		RenderJSON(w, nil, http.StatusOK, data)

		assert.Equal(t, http.StatusOK, w.statusCode)
		assert.Equal(t, "application/json", w.headers.Get("Content-Type"))

		var result map[string]string
		err := json.Unmarshal(w.body, &result)
		require.NoError(t, err)
		assert.Equal(t, "hello", result["message"])
	})

	t.Run("nil data", func(t *testing.T) {
		w := &mockResponseWriter{
			headers: make(http.Header),
			body:    []byte{},
		}

		RenderJSON(w, nil, http.StatusNoContent, nil)

		assert.Equal(t, http.StatusNoContent, w.statusCode)
		assert.Equal(t, "application/json", w.headers.Get("Content-Type"))
		assert.Empty(t, w.body)
	})
}

func TestRenderError(t *testing.T) {
	w := &mockResponseWriter{
		headers: make(http.Header),
		body:    []byte{},
	}

	err := assert.AnError
	RenderError(w, nil, err, http.StatusInternalServerError)

	assert.Equal(t, http.StatusInternalServerError, w.statusCode)
	assert.Equal(t, "application/json", w.headers.Get("Content-Type"))

	var result map[string]string
	jsonErr := json.Unmarshal(w.body, &result)
	require.NoError(t, jsonErr)
	assert.Equal(t, err.Error(), result["error"])
}

// mockResponseWriter implements http.ResponseWriter for testing
type mockResponseWriter struct {
	headers    http.Header
	body       []byte
	statusCode int
}

func (m *mockResponseWriter) Header() http.Header {
	return m.headers
}

func (m *mockResponseWriter) Write(data []byte) (int, error) {
	m.body = append(m.body, data...)
	return len(data), nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}
