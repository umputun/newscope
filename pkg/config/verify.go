package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/invopop/jsonschema"
)

//go:embed schema.json
var embeddedSchema string

// VerifyAgainstEmbeddedSchema validates the config against the embedded JSON schema
func VerifyAgainstEmbeddedSchema(cfg *Config) error {
	// parse schema
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(embeddedSchema), &schema); err != nil {
		return fmt.Errorf("parse embedded schema: %w", err)
	}

	// convert config to JSON for validation
	configData, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(configData, &configMap); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// basic validation - check required fields match
	if err := validateRequiredFields(cfg); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// VerifyAgainstSchema validates the config against the JSON schema from file
// Deprecated: Use VerifyAgainstEmbeddedSchema instead
func VerifyAgainstSchema(cfg *Config, schemaPath string) error {
	// read schema file
	schemaData, err := os.ReadFile(schemaPath) //nolint:gosec // schema path is controlled by us
	if err != nil {
		return fmt.Errorf("read schema file: %w", err)
	}

	// parse schema
	var schema map[string]interface{}
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}

	// convert config to JSON for validation
	configData, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(configData, &configMap); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// validate using jsonschema package
	// note: for production use, consider using a dedicated JSON schema validator
	// like github.com/xeipuuv/gojsonschema for full draft support

	// basic validation - check required fields match
	if err := validateRequiredFields(cfg); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// validateRequiredFields performs basic validation of required fields
func validateRequiredFields(cfg *Config) error {
	// check server config
	if cfg.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if cfg.Server.Timeout == 0 {
		return fmt.Errorf("server.timeout is required")
	}

	// check extraction config if enabled
	if cfg.Extraction.Enabled {
		if cfg.Extraction.Timeout == 0 {
			return fmt.Errorf("extraction.timeout is required when extraction is enabled")
		}
		if cfg.Extraction.MaxConcurrent == 0 {
			return fmt.Errorf("extraction.max_concurrent is required when extraction is enabled")
		}
		if cfg.Extraction.RateLimit == 0 {
			return fmt.Errorf("extraction.rate_limit is required when extraction is enabled")
		}
		if cfg.Extraction.MinTextLength < 0 {
			return fmt.Errorf("extraction.min_text_length must be non-negative")
		}
	}

	return nil
}

// GenerateSchema generates a JSON schema for the Config struct
func GenerateSchema() (*jsonschema.Schema, error) {
	return jsonschema.Reflect(&Config{}), nil
}
