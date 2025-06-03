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

llm:
  endpoint: http://localhost:11434/v1
  api_key: test-api-key
  model: llama3
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
	})

	t.Run("defaults", func(t *testing.T) {
		configContent := `
llm:
  endpoint: http://localhost:11434/v1
  api_key: test-api-key
  model: llama3
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

	t.Run("missing api key", func(t *testing.T) {
		configContent := `
llm:
  endpoint: http://localhost:11434/v1
  model: llama3
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "missing-key.yml")
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err)

		cfg, err := Load(configPath)
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "llm.api_key is required")
	})
}

func TestConfig_GetServerConfig(t *testing.T) {
	cfg := &Config{
		Server: struct {
			Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
			Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
			PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
			BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
		}{
			Listen:   ":9090",
			Timeout:  45 * time.Second,
			PageSize: 50,
			BaseURL:  "http://localhost:8080",
		},
	}

	listen, timeout := cfg.GetServerConfig()
	assert.Equal(t, ":9090", listen)
	assert.Equal(t, 45*time.Second, timeout)
}

func TestConfig_GetFullConfig(t *testing.T) {
	cfg := &Config{
		Server: struct {
			Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
			Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
			PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
			BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
		}{
			Listen:   ":9090",
			Timeout:  45 * time.Second,
			PageSize: 50,
			BaseURL:  "http://localhost:8080",
		},
	}

	fullConfig := cfg.GetFullConfig()
	assert.Equal(t, cfg, fullConfig)
	assert.Equal(t, ":9090", fullConfig.Server.Listen)
	assert.Equal(t, 45*time.Second, fullConfig.Server.Timeout)
	assert.Equal(t, 50, fullConfig.Server.PageSize)
}

func TestConfig_ValidateEdgeCases(t *testing.T) {
	t.Run("missing llm endpoint", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				APIKey: "test-key",
				Model:  "gpt-4",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.endpoint is required")
	})

	t.Run("missing llm api key", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint: "https://api.openai.com/v1",
				Model:    "gpt-4",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.api_key is required")
	})

	t.Run("missing llm model", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "test-key",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.model is required")
	})

	t.Run("llm temperature too low", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint:    "https://api.openai.com/v1",
				APIKey:      "test-key",
				Model:       "gpt-4",
				Temperature: -0.1,
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  time.Second,
				PageSize: 1,
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.temperature must be between 0 and 2")
	})

	t.Run("llm temperature too high", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint:    "https://api.openai.com/v1",
				APIKey:      "test-key",
				Model:       "gpt-4",
				Temperature: 2.1,
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  time.Second,
				PageSize: 1,
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.temperature must be between 0 and 2")
	})

	t.Run("llm temperature boundary values valid", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint:    "https://api.openai.com/v1",
				APIKey:      "test-key",
				Model:       "gpt-4",
				Temperature: 0.0, // boundary value
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  time.Second,
				PageSize: 1,
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		require.NoError(t, err)

		cfg.LLM.Temperature = 2.0 // boundary value
		err = validate(cfg)
		assert.NoError(t, err)
	})

	t.Run("extraction timeout too low", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "test-key",
				Model:    "gpt-4",
			},
			Extraction: ExtractionConfig{
				Enabled: true,
				Timeout: 999 * time.Millisecond, // less than 1 second
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  time.Second,
				PageSize: 1,
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extraction timeout must be at least 1 second")
	})

	t.Run("extraction min text length negative", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "test-key",
				Model:    "gpt-4",
			},
			Extraction: ExtractionConfig{
				Enabled:       true,
				Timeout:       time.Second,
				MinTextLength: -1,
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  time.Second,
				PageSize: 1,
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extraction min_text_length must be non-negative")
	})

	t.Run("extraction disabled skips validation", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "test-key",
				Model:    "gpt-4",
			},
			Extraction: ExtractionConfig{
				Enabled:       false,                  // disabled
				Timeout:       999 * time.Millisecond, // would be invalid if enabled
				MinTextLength: -1,                     // would be invalid if enabled
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  time.Second,
				PageSize: 1,
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		assert.NoError(t, err) // should not validate extraction when disabled
	})

	t.Run("server timeout too low", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "test-key",
				Model:    "gpt-4",
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  999 * time.Millisecond, // less than 1 second
				PageSize: 1,
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server timeout must be at least 1 second")
	})

	t.Run("server page size too low", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "test-key",
				Model:    "gpt-4",
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  time.Second,
				PageSize: 0, // less than 1
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server page_size must be at least 1")
	})

	t.Run("valid minimal config", func(t *testing.T) {
		cfg := &Config{
			LLM: LLMConfig{
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "test-key",
				Model:    "gpt-4",
			},
			Server: struct {
				Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
				Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
				BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
			}{
				Timeout:  time.Second, // minimum valid value
				PageSize: 1,           // minimum valid value
				BaseURL:  "http://localhost:8080",
			},
		}
		err := validate(cfg)
		assert.NoError(t, err)
	})
}
