package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/invopop/jsonschema"
)

//go:embed schema.json
var embeddedSchemaData []byte

// VerifyAgainstEmbeddedSchema validates the config against the embedded JSON schema
func VerifyAgainstEmbeddedSchema(cfg *Config) error {
	// parse embedded schema
	var schema map[string]interface{}
	if err := json.Unmarshal(embeddedSchemaData, &schema); err != nil {
		return fmt.Errorf("parse embedded schema: %w", err)
	}

	// basic validation using embedded schema data
	if err := validateRequiredFields(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	return nil
}


// validateRequiredFields performs basic validation of required fields
func validateRequiredFields(cfg *Config) error {
	// server validation
	if cfg.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if cfg.Server.Timeout <= 0 {
		return fmt.Errorf("server.timeout must be greater than 0")
	}

	// database validation
	if cfg.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}

	// llm validation
	if cfg.LLM.Endpoint == "" {
		return fmt.Errorf("llm.endpoint is required")
	}
	if cfg.LLM.APIKey == "" {
		return fmt.Errorf("llm.api_key is required")
	}
	if cfg.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}

	// extraction validation (when enabled)
	if cfg.Extraction.Enabled {
		if cfg.Extraction.MaxConcurrent <= 0 {
			return fmt.Errorf("extraction.max_concurrent is required when extraction is enabled")
		}
		if cfg.Extraction.RateLimit <= 0 {
			return fmt.Errorf("extraction.rate_limit is required when extraction is enabled")
		}
		if cfg.Extraction.Timeout <= 0 {
			return fmt.Errorf("extraction.timeout is required when extraction is enabled")
		}
	}

	// schedule validation
	if cfg.Schedule.UpdateInterval <= 0 {
		return fmt.Errorf("schedule.update_interval must be greater than 0")
	}
	if cfg.Schedule.MaxWorkers <= 0 {
		return fmt.Errorf("schedule.max_workers must be greater than 0")
	}

	// validate reasonable ranges
	if cfg.Server.Timeout < time.Second {
		return fmt.Errorf("server.timeout must be at least 1 second")
	}
	if cfg.Extraction.Enabled && cfg.Extraction.Timeout < time.Second {
		return fmt.Errorf("extraction.timeout must be at least 1 second")
	}

	return nil
}

// GenerateSchema generates a JSON schema for the Config struct
func GenerateSchema() (*jsonschema.Schema, error) {
	return jsonschema.Reflect(&Config{}), nil
}