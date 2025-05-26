package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyAgainstSchema(t *testing.T) {
	// create a temporary schema file
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test.schema.json")

	// generate and write schema
	schema, err := GenerateSchema()
	require.NoError(t, err)

	schemaJSON, err := schema.MarshalJSON()
	require.NoError(t, err)

	err = os.WriteFile(schemaPath, schemaJSON, 0o644)
	require.NoError(t, err)

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
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  ":8080",
					Timeout: 30 * time.Second,
				},
				Extraction: ExtractionConfig{
					Enabled:       false,
					Timeout:       30 * time.Second,
					MaxConcurrent: 5,
					RateLimit:     100 * time.Millisecond,
				},
				Feeds: []Feed{
					{
						URL:      "https://example.com/feed.xml",
						Name:     "Example Feed",
						Interval: 30 * time.Minute,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing server listen",
			config: &Config{
				Server: struct {
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  "",
					Timeout: 30 * time.Second,
				},
				Feeds: []Feed{
					{URL: "https://example.com/feed.xml"},
				},
			},
			wantErr: true,
			errMsg:  "server.listen is required",
		},
		{
			name: "no feeds",
			config: &Config{
				Server: struct {
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  ":8080",
					Timeout: 30 * time.Second,
				},
				Feeds: []Feed{},
			},
			wantErr: true,
			errMsg:  "at least one feed is required",
		},
		{
			name: "extraction enabled without timeout",
			config: &Config{
				Server: struct {
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  ":8080",
					Timeout: 30 * time.Second,
				},
				Extraction: ExtractionConfig{
					Enabled:       true,
					Timeout:       0, // missing
					MaxConcurrent: 5,
					RateLimit:     100 * time.Millisecond,
				},
				Feeds: []Feed{
					{URL: "https://example.com/feed.xml"},
				},
			},
			wantErr: true,
			errMsg:  "extraction.timeout is required when extraction is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyAgainstSchema(tt.config, schemaPath)
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
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  ":8080",
					Timeout: 30 * time.Second,
				},
				Feeds: []Feed{
					{URL: "https://example.com/feed.xml"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing feed URL",
			config: &Config{
				Server: struct {
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  ":8080",
					Timeout: 30 * time.Second,
				},
				Feeds: []Feed{
					{URL: ""},
				},
			},
			wantErr: true,
			errMsg:  "feed[0].url is required",
		},
		{
			name: "extraction enabled with missing max_concurrent",
			config: &Config{
				Server: struct {
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  ":8080",
					Timeout: 30 * time.Second,
				},
				Extraction: ExtractionConfig{
					Enabled:       true,
					Timeout:       30 * time.Second,
					MaxConcurrent: 0, // missing
					RateLimit:     100 * time.Millisecond,
				},
				Feeds: []Feed{
					{URL: "https://example.com/feed.xml"},
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
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  ":8080",
					Timeout: 30 * time.Second,
				},
				Extraction: ExtractionConfig{
					Enabled:       false,
					Timeout:       30 * time.Second,
					MaxConcurrent: 5,
					RateLimit:     100 * time.Millisecond,
				},
				Feeds: []Feed{
					{
						URL:      "https://example.com/feed.xml",
						Name:     "Example Feed",
						Interval: 30 * time.Minute,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing required field",
			config: &Config{
				Server: struct {
					Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
					Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
				}{
					Listen:  "",
					Timeout: 30 * time.Second,
				},
				Feeds: []Feed{
					{URL: "https://example.com/feed.xml"},
				},
			},
			wantErr: true,
			errMsg:  "server.listen is required",
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
	assert.Contains(t, schemaStr, "feeds")
	assert.Contains(t, schemaStr, "extraction")
}
