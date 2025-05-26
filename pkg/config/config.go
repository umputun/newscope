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

	Extraction ExtractionConfig `yaml:"extraction" json:"extraction" jsonschema:"description=Content extraction configuration"`

	Feeds []Feed `yaml:"feeds" json:"feeds" jsonschema:"required,minItems=1,description=RSS/Atom feed sources"`
}

// ExtractionConfig holds content extraction settings
type ExtractionConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled" jsonschema:"default=false,description=Enable content extraction"`
	Timeout       time.Duration `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=Extraction timeout per article"`
	MaxConcurrent int           `yaml:"max_concurrent" json:"max_concurrent" jsonschema:"default=5,minimum=1,maximum=20,description=Maximum concurrent extractions"`
	RateLimit     time.Duration `yaml:"rate_limit" json:"rate_limit" jsonschema:"default=100ms,description=Minimum time between extractions"`
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

	// set defaults for extraction
	if cfg.Extraction.Timeout == 0 {
		cfg.Extraction.Timeout = 30 * time.Second
	}
	if cfg.Extraction.MaxConcurrent == 0 {
		cfg.Extraction.MaxConcurrent = 5
	}
	if cfg.Extraction.RateLimit == 0 {
		cfg.Extraction.RateLimit = 100 * time.Millisecond
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
		if cfg.Extraction.MaxConcurrent < 1 || cfg.Extraction.MaxConcurrent > 20 {
			return fmt.Errorf("extraction max_concurrent must be between 1 and 20")
		}
		if cfg.Extraction.Timeout < time.Second {
			return fmt.Errorf("extraction timeout must be at least 1 second")
		}
		if cfg.Extraction.RateLimit < 10*time.Millisecond {
			return fmt.Errorf("extraction rate_limit must be at least 10ms")
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
