package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		configContent := `
server:
  listen: ":9090"
  timeout: 45s

feeds:
  - url: https://example.com/feed1.xml
    name: Feed1
    interval: 5m
  - url: https://example.com/feed2.xml
    name: Feed2
    interval: 10m
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "test-config.yml")
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err)

		cfg, err := Load(configPath)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.Equal(t, ":9090", cfg.Server.Listen)
		assert.Equal(t, 45*time.Second, cfg.Server.Timeout)
		assert.Len(t, cfg.Feeds, 2)

		assert.Equal(t, "https://example.com/feed1.xml", cfg.Feeds[0].URL)
		assert.Equal(t, "Feed1", cfg.Feeds[0].Name)
		assert.Equal(t, 5*time.Minute, cfg.Feeds[0].Interval)

		assert.Equal(t, "https://example.com/feed2.xml", cfg.Feeds[1].URL)
		assert.Equal(t, "Feed2", cfg.Feeds[1].Name)
		assert.Equal(t, 10*time.Minute, cfg.Feeds[1].Interval)
	})

	t.Run("defaults", func(t *testing.T) {
		configContent := `
feeds:
  - url: https://example.com/feed.xml
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "test-config.yml")
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err)

		cfg, err := Load(configPath)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// check server defaults
		assert.Equal(t, ":8080", cfg.Server.Listen)
		assert.Equal(t, 30*time.Second, cfg.Server.Timeout)

		// check feed defaults
		assert.Len(t, cfg.Feeds, 1)
		assert.Equal(t, "https://example.com/feed.xml", cfg.Feeds[0].Name) // name defaults to URL
		assert.Equal(t, 30*time.Minute, cfg.Feeds[0].Interval)
	})

	t.Run("file not found", func(t *testing.T) {
		cfg, err := Load("/non/existent/file.yml")
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "read config file")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		configContent := `
invalid yaml content
  with bad indentation
    and no structure
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid.yml")
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err)

		cfg, err := Load(configPath)
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "parse config")
	})
}

func TestConfig_GetFeeds(t *testing.T) {
	cfg := &Config{
		Feeds: []Feed{
			{URL: "https://feed1.com", Name: "Feed1", Interval: 5 * time.Minute},
			{URL: "https://feed2.com", Name: "Feed2", Interval: 10 * time.Minute},
		},
	}

	feeds := cfg.GetFeeds()
	assert.Len(t, feeds, 2)
	assert.Equal(t, cfg.Feeds, feeds)
}

func TestConfig_GetServerConfig(t *testing.T) {
	cfg := &Config{
		Server: struct {
			Listen  string        `yaml:"listen"`
			Timeout time.Duration `yaml:"timeout"`
		}{
			Listen:  ":9090",
			Timeout: 45 * time.Second,
		},
	}

	listen, timeout := cfg.GetServerConfig()
	assert.Equal(t, ":9090", listen)
	assert.Equal(t, 45*time.Second, timeout)
}