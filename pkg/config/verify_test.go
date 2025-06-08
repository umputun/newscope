package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyAgainstEmbeddedSchema(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
				Database: struct {
					DSN             string `yaml:"dsn" json:"dsn" jsonschema:"default=file:newscope.db?cache=shared&mode=rwc,description=Database connection string"`
					MaxOpenConns    int    `yaml:"max_open_conns" json:"max_open_conns" jsonschema:"default=10,description=Maximum number of open connections"`
					MaxIdleConns    int    `yaml:"max_idle_conns" json:"max_idle_conns" jsonschema:"default=5,description=Maximum number of idle connections"`
					ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime" jsonschema:"default=3600,description=Connection maximum lifetime in seconds"`
				}{
					DSN: "file:test.db",
				},
				LLM: LLMConfig{
					Endpoint: "http://localhost:8080",
					APIKey:   "test-key",
					Model:    "test-model",
				},
				Schedule: struct {
					UpdateInterval  time.Duration `yaml:"update_interval" json:"update_interval" jsonschema:"default=1m,description=Scheduler run interval"`
					MaxWorkers      int           `yaml:"max_workers" json:"max_workers" jsonschema:"default=5,description=Maximum concurrent workers"`
					CleanupAge      time.Duration `yaml:"cleanup_age" json:"cleanup_age" jsonschema:"default=168h,description=Maximum age for articles with low scores (default 1 week)"`
					CleanupMinScore float64       `yaml:"cleanup_min_score" json:"cleanup_min_score" jsonschema:"default=5.0,description=Minimum score to keep articles regardless of age"`
					CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval" jsonschema:"default=24h,description=How often to run cleanup"`
				}{
					UpdateInterval: 1 * time.Minute,
					MaxWorkers:     5,
				},
				Extraction: ExtractionConfig{
					Enabled:       false,
					Timeout:       30 * time.Second,
					MaxConcurrent: 5,
					RateLimit:     100 * time.Millisecond,
				},
			},
			wantErr: false,
		},
		{
			name: "missing server listen",
			config: &Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					Listen:   "",
					Timeout:  30 * time.Second,
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
				Database: struct {
					DSN             string `yaml:"dsn" json:"dsn" jsonschema:"default=file:newscope.db?cache=shared&mode=rwc,description=Database connection string"`
					MaxOpenConns    int    `yaml:"max_open_conns" json:"max_open_conns" jsonschema:"default=10,description=Maximum number of open connections"`
					MaxIdleConns    int    `yaml:"max_idle_conns" json:"max_idle_conns" jsonschema:"default=5,description=Maximum number of idle connections"`
					ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime" jsonschema:"default=3600,description=Connection maximum lifetime in seconds"`
				}{
					DSN: "file:test.db",
				},
				LLM: LLMConfig{
					Endpoint: "http://localhost:8080",
					APIKey:   "test-key",
					Model:    "test-model",
				},
			},
			wantErr: true,
			errMsg:  "server.listen is required",
		},
		{
			name: "extraction enabled without timeout",
			config: &Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
				Database: struct {
					DSN             string `yaml:"dsn" json:"dsn" jsonschema:"default=file:newscope.db?cache=shared&mode=rwc,description=Database connection string"`
					MaxOpenConns    int    `yaml:"max_open_conns" json:"max_open_conns" jsonschema:"default=10,description=Maximum number of open connections"`
					MaxIdleConns    int    `yaml:"max_idle_conns" json:"max_idle_conns" jsonschema:"default=5,description=Maximum number of idle connections"`
					ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime" jsonschema:"default=3600,description=Connection maximum lifetime in seconds"`
				}{
					DSN: "file:test.db",
				},
				LLM: LLMConfig{
					Endpoint: "http://localhost:8080",
					APIKey:   "test-key",
					Model:    "test-model",
				},
				Schedule: struct {
					UpdateInterval  time.Duration `yaml:"update_interval" json:"update_interval" jsonschema:"default=1m,description=Scheduler run interval"`
					MaxWorkers      int           `yaml:"max_workers" json:"max_workers" jsonschema:"default=5,description=Maximum concurrent workers"`
					CleanupAge      time.Duration `yaml:"cleanup_age" json:"cleanup_age" jsonschema:"default=168h,description=Maximum age for articles with low scores (default 1 week)"`
					CleanupMinScore float64       `yaml:"cleanup_min_score" json:"cleanup_min_score" jsonschema:"default=5.0,description=Minimum score to keep articles regardless of age"`
					CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval" jsonschema:"default=24h,description=How often to run cleanup"`
				}{
					UpdateInterval: 1 * time.Minute,
					MaxWorkers:     5,
				},
				Extraction: ExtractionConfig{
					Enabled:       true,
					Timeout:       0, // missing
					MaxConcurrent: 5,
					RateLimit:     100 * time.Millisecond,
				},
			},
			wantErr: true,
			errMsg:  "extraction.timeout is required when extraction is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyAgainstEmbeddedSchema(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal config",
			config: &Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
				Database: struct {
					DSN             string `yaml:"dsn" json:"dsn" jsonschema:"default=file:newscope.db?cache=shared&mode=rwc,description=Database connection string"`
					MaxOpenConns    int    `yaml:"max_open_conns" json:"max_open_conns" jsonschema:"default=10,description=Maximum number of open connections"`
					MaxIdleConns    int    `yaml:"max_idle_conns" json:"max_idle_conns" jsonschema:"default=5,description=Maximum number of idle connections"`
					ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime" jsonschema:"default=3600,description=Connection maximum lifetime in seconds"`
				}{
					DSN: "file:test.db",
				},
				LLM: LLMConfig{
					Endpoint: "http://localhost:8080",
					APIKey:   "test-key",
					Model:    "test-model",
				},
				Schedule: struct {
					UpdateInterval  time.Duration `yaml:"update_interval" json:"update_interval" jsonschema:"default=1m,description=Scheduler run interval"`
					MaxWorkers      int           `yaml:"max_workers" json:"max_workers" jsonschema:"default=5,description=Maximum concurrent workers"`
					CleanupAge      time.Duration `yaml:"cleanup_age" json:"cleanup_age" jsonschema:"default=168h,description=Maximum age for articles with low scores (default 1 week)"`
					CleanupMinScore float64       `yaml:"cleanup_min_score" json:"cleanup_min_score" jsonschema:"default=5.0,description=Minimum score to keep articles regardless of age"`
					CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval" jsonschema:"default=24h,description=How often to run cleanup"`
				}{
					UpdateInterval: 1 * time.Minute,
					MaxWorkers:     5,
				},
			},
			wantErr: false,
		},
		{
			name: "extraction enabled with missing max_concurrent",
			config: &Config{
				Server: struct {
					Listen   string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout  time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
					PageSize int           `yaml:"page_size" json:"page_size" jsonschema:"default=50,minimum=1,description=Articles per page for pagination"`
					BaseURL  string        `yaml:"base_url" json:"base_url" jsonschema:"default=http://localhost:8080,description=Base URL for RSS feeds and external links"`
				}{
					Listen:   ":8080",
					Timeout:  30 * time.Second,
					PageSize: 50,
					BaseURL:  "http://localhost:8080",
				},
				Database: struct {
					DSN             string `yaml:"dsn" json:"dsn" jsonschema:"default=file:newscope.db?cache=shared&mode=rwc,description=Database connection string"`
					MaxOpenConns    int    `yaml:"max_open_conns" json:"max_open_conns" jsonschema:"default=10,description=Maximum number of open connections"`
					MaxIdleConns    int    `yaml:"max_idle_conns" json:"max_idle_conns" jsonschema:"default=5,description=Maximum number of idle connections"`
					ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime" jsonschema:"default=3600,description=Connection maximum lifetime in seconds"`
				}{
					DSN: "file:test.db",
				},
				LLM: LLMConfig{
					Endpoint: "http://localhost:8080",
					APIKey:   "test-key",
					Model:    "test-model",
				},
				Schedule: struct {
					UpdateInterval  time.Duration `yaml:"update_interval" json:"update_interval" jsonschema:"default=1m,description=Scheduler run interval"`
					MaxWorkers      int           `yaml:"max_workers" json:"max_workers" jsonschema:"default=5,description=Maximum concurrent workers"`
					CleanupAge      time.Duration `yaml:"cleanup_age" json:"cleanup_age" jsonschema:"default=168h,description=Maximum age for articles with low scores (default 1 week)"`
					CleanupMinScore float64       `yaml:"cleanup_min_score" json:"cleanup_min_score" jsonschema:"default=5.0,description=Minimum score to keep articles regardless of age"`
					CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval" jsonschema:"default=24h,description=How often to run cleanup"`
				}{
					UpdateInterval: 1 * time.Minute,
					MaxWorkers:     5,
				},
				Extraction: ExtractionConfig{
					Enabled:       true,
					Timeout:       30 * time.Second,
					MaxConcurrent: 0, // missing
					RateLimit:     100 * time.Millisecond,
				},
			},
			wantErr: true,
			errMsg:  "extraction.max_concurrent is required when extraction is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequiredFields(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGenerateSchema(t *testing.T) {
	schema, err := GenerateSchema()
	require.NoError(t, err)
	require.NotNil(t, schema)

	// verify schema can be marshaled to JSON
	data, err := schema.MarshalJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// verify it contains expected fields
	schemaStr := string(data)
	assert.Contains(t, schemaStr, "Config")
	assert.Contains(t, schemaStr, "server")
	assert.Contains(t, schemaStr, "extraction")
}
