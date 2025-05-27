package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

//go:generate go run ../../cmd/schema/main.go schema.json

// Config holds the application configuration
type Config struct {
	Server struct {
		Listen  string        `yaml:"listen" json:"listen" jsonschema:"default=:8080,description=HTTP server listen address"`
		Timeout time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=HTTP server timeout"`
	} `yaml:"server" json:"server" jsonschema:"description=Server configuration"`

	Database struct {
		DSN             string `yaml:"dsn" json:"dsn" jsonschema:"default=file:newscope.db?cache=shared&mode=rwc,description=Database connection string"`
		MaxOpenConns    int    `yaml:"max_open_conns" json:"max_open_conns" jsonschema:"default=10,description=Maximum number of open connections"`
		MaxIdleConns    int    `yaml:"max_idle_conns" json:"max_idle_conns" jsonschema:"default=5,description=Maximum number of idle connections"`
		ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime" jsonschema:"default=3600,description=Connection maximum lifetime in seconds"`
	} `yaml:"database" json:"database" jsonschema:"description=Database configuration"`

	Schedule struct {
		UpdateInterval  int `yaml:"update_interval" json:"update_interval" jsonschema:"default=30,description=Feed update interval in minutes"`
		ExtractInterval int `yaml:"extract_interval" json:"extract_interval" jsonschema:"default=5,description=Content extraction interval in minutes"`
		MaxWorkers      int `yaml:"max_workers" json:"max_workers" jsonschema:"default=5,description=Maximum concurrent workers"`
	} `yaml:"schedule" json:"schedule" jsonschema:"description=Scheduler configuration"`

	Extraction ExtractionConfig `yaml:"extraction" json:"extraction" jsonschema:"description=Content extraction configuration"`

	Feeds []Feed `yaml:"feeds" json:"feeds" jsonschema:"required,minItems=1,description=RSS/Atom feed sources"`
}

// ExtractionConfig holds content extraction settings
type ExtractionConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled" jsonschema:"default=false,description=Enable content extraction"`
	Timeout       time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=Extraction timeout per article"`
	MaxConcurrent int           `yaml:"max_concurrent" json:"max_concurrent" jsonschema:"default=5,description=Maximum concurrent extractions"`
	RateLimit     time.Duration `yaml:"rate_limit" json:"rate_limit" jsonschema:"default=1s,description=Rate limit between extractions"`
	UserAgent     string        `yaml:"user_agent" json:"user_agent" jsonschema:"default=Newscope/1.0,description=User agent for HTTP requests"`
	FallbackURL   string        `yaml:"fallback_url" json:"fallback_url" jsonschema:"description=Fallback trafilatura API URL"`
	MinTextLength int           `yaml:"min_text_length" json:"min_text_length" jsonschema:"default=100,description=Minimum text length to consider valid"`
	IncludeImages bool          `yaml:"include_images" json:"include_images" jsonschema:"default=false,description=Include images in extraction"`
	IncludeLinks  bool          `yaml:"include_links" json:"include_links" jsonschema:"default=false,description=Include links in extraction"`
}

// Feed represents a single RSS/Atom feed
type Feed struct {
	URL      string        `yaml:"url" json:"url" jsonschema:"required,format=uri,description=Feed URL"`
	Name     string        `yaml:"name" json:"name" jsonschema:"description=Feed name (defaults to URL)"`
	Interval time.Duration `yaml:"interval" json:"interval" jsonschema:"default=30m,description=Feed refresh interval"`
}

// Load reads configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // file path comes from CLI flag
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// set defaults for server
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8080"
	}
	if cfg.Server.Timeout == 0 {
		cfg.Server.Timeout = 30 * time.Second
	}

	// set defaults for database
	if cfg.Database.DSN == "" {
		cfg.Database.DSN = "file:newscope.db?cache=shared&mode=rwc"
	}
	if cfg.Database.MaxOpenConns == 0 {
		cfg.Database.MaxOpenConns = 10
	}
	if cfg.Database.MaxIdleConns == 0 {
		cfg.Database.MaxIdleConns = 5
	}
	if cfg.Database.ConnMaxLifetime == 0 {
		cfg.Database.ConnMaxLifetime = 3600
	}

	// set defaults for schedule
	if cfg.Schedule.UpdateInterval == 0 {
		cfg.Schedule.UpdateInterval = 30
	}
	if cfg.Schedule.ExtractInterval == 0 {
		cfg.Schedule.ExtractInterval = 5
	}
	if cfg.Schedule.MaxWorkers == 0 {
		cfg.Schedule.MaxWorkers = 5
	}

	// set defaults for extraction
	if cfg.Extraction.Timeout == 0 {
		cfg.Extraction.Timeout = 30 * time.Second
	}
	if cfg.Extraction.MaxConcurrent == 0 {
		cfg.Extraction.MaxConcurrent = 5
	}
	if cfg.Extraction.RateLimit == 0 {
		cfg.Extraction.RateLimit = 1 * time.Second
	}
	if cfg.Extraction.UserAgent == "" {
		cfg.Extraction.UserAgent = "Newscope/1.0"
	}
	if cfg.Extraction.MinTextLength == 0 {
		cfg.Extraction.MinTextLength = 100
	}

	// set defaults for feeds
	for i := range cfg.Feeds {
		if cfg.Feeds[i].Interval == 0 {
			cfg.Feeds[i].Interval = 30 * time.Minute
		}
		if cfg.Feeds[i].Name == "" {
			cfg.Feeds[i].Name = cfg.Feeds[i].URL
		}
	}

	// validate configuration
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	// verify against embedded schema
	if err := VerifyAgainstEmbeddedSchema(&cfg); err != nil {
		// log warning but don't fail - schema validation is supplementary
		fmt.Printf("warning: schema validation failed: %v\n", err)
	}

	return &cfg, nil
}

// validate checks configuration for correctness
func validate(cfg *Config) error {
	// validate feeds
	if len(cfg.Feeds) == 0 {
		return fmt.Errorf("at least one feed is required")
	}

	for i, feed := range cfg.Feeds {
		if feed.URL == "" {
			return fmt.Errorf("feed[%d]: URL is required", i)
		}
		if feed.Interval < time.Minute {
			return fmt.Errorf("feed[%d]: interval must be at least 1 minute", i)
		}
	}

	// validate extraction config
	if cfg.Extraction.Enabled {
		if cfg.Extraction.Timeout < time.Second {
			return fmt.Errorf("extraction timeout must be at least 1 second")
		}
		if cfg.Extraction.MinTextLength < 0 {
			return fmt.Errorf("extraction min_text_length must be non-negative")
		}
	}

	// validate server config
	if cfg.Server.Timeout < time.Second {
		return fmt.Errorf("server timeout must be at least 1 second")
	}

	return nil
}

// GetFeeds returns all configured feeds
func (c *Config) GetFeeds() []Feed {
	return c.Feeds
}

// GetServerConfig returns server configuration
func (c *Config) GetServerConfig() (listen string, timeout time.Duration) {
	return c.Server.Listen, c.Server.Timeout
}

// GetExtractionConfig returns content extraction configuration
func (c *Config) GetExtractionConfig() ExtractionConfig {
	return c.Extraction
}
