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
}

func TestConfig_GetServerConfig(t *testing.T) {
	cfg := &Config{
		Server: struct {
			Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
			Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
		}{
			Listen:  ":9090",
			Timeout: 45 * time.Second,
		},
	}

	listen, timeout := cfg.GetServerConfig()
	assert.Equal(t, ":9090", listen)
	assert.Equal(t, 45*time.Second, timeout)
}
