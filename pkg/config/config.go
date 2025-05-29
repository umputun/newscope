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
		UpdateInterval int `yaml:"update_interval" json:"update_interval" jsonschema:"default=30,description=Feed update interval in minutes"`
		MaxWorkers     int `yaml:"max_workers" json:"max_workers" jsonschema:"default=5,description=Maximum concurrent workers"`
	} `yaml:"schedule" json:"schedule" jsonschema:"description=Scheduler configuration"`

	LLM LLMConfig `yaml:"llm" json:"llm" jsonschema:"description=LLM configuration for article classification"`

	Extraction ExtractionConfig `yaml:"extraction" json:"extraction" jsonschema:"description=Content extraction configuration"`
}

// ClassificationConfig holds classification-specific settings
type ClassificationConfig struct {
	ScoreThreshold   float64 `yaml:"score_threshold" json:"score_threshold" jsonschema:"default=5.0,minimum=0,maximum=10,description=Minimum relevance score to include articles"`
	FeedbackExamples int     `yaml:"feedback_examples" json:"feedback_examples" jsonschema:"default=10,description=Number of recent feedback examples to include in prompt"`
	BatchSize        int     `yaml:"batch_size" json:"batch_size" jsonschema:"default=5,minimum=1,description=Number of articles to classify in one request"`
	UseJSONMode      bool    `yaml:"use_json_mode" json:"use_json_mode" jsonschema:"default=false,description=Use JSON response format (not all models support this)"`
}

// LLMConfig holds LLM configuration for article classification
type LLMConfig struct {
	Endpoint       string               `yaml:"endpoint" json:"endpoint" jsonschema:"required,description=OpenAI-compatible API endpoint"`
	APIKey         string               `yaml:"api_key" json:"api_key" jsonschema:"description=API key (can use environment variable)"`
	Model          string               `yaml:"model" json:"model" jsonschema:"required,description=Model name (e.g. gpt-4o-mini or llama3)"`
	Temperature    float64              `yaml:"temperature" json:"temperature" jsonschema:"default=0.3,description=Temperature for response generation"`
	MaxTokens      int                  `yaml:"max_tokens" json:"max_tokens" jsonschema:"default=500,description=Maximum tokens in response"`
	Timeout        time.Duration        `yaml:"timeout" json:"timeout" jsonschema:"default=30s,description=Request timeout"`
	SystemPrompt   string               `yaml:"system_prompt" json:"system_prompt" jsonschema:"description=System prompt for the LLM (optional)"`
	Classification ClassificationConfig `yaml:"classification" json:"classification" jsonschema:"description=Classification-specific settings"`
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

// Load reads configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // file path comes from CLI flag
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
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
		cfg.Database.DSN = "file:newscope.db?cache=shared&mode=rwc&_txlock=immediate"
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
	if cfg.Schedule.MaxWorkers == 0 {
		cfg.Schedule.MaxWorkers = 5
	}

	// set defaults for LLM
	if cfg.LLM.Temperature == 0 {
		cfg.LLM.Temperature = 0.3
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 500
	}
	if cfg.LLM.Timeout == 0 {
		cfg.LLM.Timeout = 30 * time.Second
	}
	if cfg.LLM.Classification.ScoreThreshold == 0 {
		cfg.LLM.Classification.ScoreThreshold = 5.0
	}
	if cfg.LLM.Classification.FeedbackExamples == 0 {
		cfg.LLM.Classification.FeedbackExamples = 10
	}
	if cfg.LLM.Classification.BatchSize == 0 {
		cfg.LLM.Classification.BatchSize = 5
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

	// validate LLM config
	if cfg.LLM.Endpoint == "" {
		return fmt.Errorf("llm.endpoint is required")
	}
	if cfg.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	if cfg.LLM.Temperature < 0 || cfg.LLM.Temperature > 2 {
		return fmt.Errorf("llm.temperature must be between 0 and 2")
	}
	if cfg.LLM.Classification.ScoreThreshold < 0 || cfg.LLM.Classification.ScoreThreshold > 10 {
		return fmt.Errorf("llm.classification.score_threshold must be between 0 and 10")
	}
	if cfg.LLM.Classification.BatchSize < 1 {
		return fmt.Errorf("llm.classification.batch_size must be at least 1")
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

// GetServerConfig returns server configuration
func (c *Config) GetServerConfig() (listen string, timeout time.Duration) {
	return c.Server.Listen, c.Server.Timeout
}

// GetExtractionConfig returns content extraction configuration
func (c *Config) GetExtractionConfig() ExtractionConfig {
	return c.Extraction
}

// GetLLMConfig returns LLM configuration
func (c *Config) GetLLMConfig() LLMConfig {
	return c.LLM
}

// GetFullConfig returns the full configuration
func (c *Config) GetFullConfig() *Config {
	return c
}
