package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Server struct {
		Listen  string        `yaml:"listen"`
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"server"`
	
	Feeds []Feed `yaml:"feeds"`
}

// Feed represents a single RSS/Atom feed
type Feed struct {
	URL      string        `yaml:"url"`
	Name     string        `yaml:"name"`
	Interval time.Duration `yaml:"interval"`
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

	// set defaults for feeds
	for i := range cfg.Feeds {
		if cfg.Feeds[i].Interval == 0 {
			cfg.Feeds[i].Interval = 30 * time.Minute
		}
		if cfg.Feeds[i].Name == "" {
			cfg.Feeds[i].Name = cfg.Feeds[i].URL
		}
	}

	return &cfg, nil
}

// GetFeeds returns all configured feeds
func (c *Config) GetFeeds() []Feed {
	return c.Feeds
}

// GetServerConfig returns server configuration
func (c *Config) GetServerConfig() (listen string, timeout time.Duration) {
	return c.Server.Listen, c.Server.Timeout
}