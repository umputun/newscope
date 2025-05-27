package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// ML model and settings database operations

// CreateMLModel stores a new ML model
func (db *DB) CreateMLModel(ctx context.Context, model *MLModel) error {
	query := `
		INSERT INTO ml_models (
			model_type, model_data, feature_config, training_stats,
			sample_count, created_at, is_active
		) VALUES (?, ?, ?, ?, ?, ?, ?)`

	result, err := db.conn.ExecContext(ctx, query,
		model.ModelType,
		model.ModelData,
		model.FeatureConfig,
		model.TrainingStats,
		model.SampleCount,
		model.CreatedAt,
		model.IsActive,
	)
	if err != nil {
		return fmt.Errorf("create ml model: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	model.ID = id
	return nil
}

// GetActiveModel retrieves the currently active model of a specific type
func (db *DB) GetActiveModel(ctx context.Context, modelType string) (*MLModel, error) {
	var model MLModel
	query := `
		SELECT * FROM ml_models 
		WHERE model_type = ? AND is_active = 1 
		ORDER BY created_at DESC 
		LIMIT 1`

	err := db.conn.GetContext(ctx, &model, query, modelType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active model: %w", err)
	}
	return &model, nil
}

// GetModel retrieves a model by ID
func (db *DB) GetModel(ctx context.Context, id int64) (*MLModel, error) {
	var model MLModel
	query := `SELECT * FROM ml_models WHERE id = ?`

	err := db.conn.GetContext(ctx, &model, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get model: %w", err)
	}
	return &model, nil
}

// SetActiveModel sets a model as active and deactivates others of the same type
func (db *DB) SetActiveModel(ctx context.Context, modelID int64, modelType string) error {
	return db.InTransaction(ctx, func(tx *sqlx.Tx) error {
		// deactivate all models of this type
		_, err := tx.ExecContext(ctx,
			"UPDATE ml_models SET is_active = 0 WHERE model_type = ?", modelType)
		if err != nil {
			return fmt.Errorf("deactivate models: %w", err)
		}

		// activate the specified model
		_, err = tx.ExecContext(ctx,
			"UPDATE ml_models SET is_active = 1 WHERE id = ?", modelID)
		if err != nil {
			return fmt.Errorf("activate model: %w", err)
		}

		return nil
	})
}

// GetModelHistory retrieves model history for a specific type
func (db *DB) GetModelHistory(ctx context.Context, modelType string, limit int) ([]MLModel, error) {
	query := `
		SELECT * FROM ml_models 
		WHERE model_type = ? 
		ORDER BY created_at DESC 
		LIMIT ?`

	var models []MLModel
	err := db.conn.SelectContext(ctx, &models, query, modelType, limit)
	if err != nil {
		return nil, fmt.Errorf("get model history: %w", err)
	}
	return models, nil
}

// DeleteOldModels removes old inactive models
func (db *DB) DeleteOldModels(ctx context.Context, keepCount int) error {
	// for each model type, keep the most recent N models
	types := []string{}
	err := db.conn.SelectContext(ctx, &types, "SELECT DISTINCT model_type FROM ml_models")
	if err != nil {
		return fmt.Errorf("get model types: %w", err)
	}

	for _, modelType := range types {
		query := `
			DELETE FROM ml_models 
			WHERE model_type = ? AND is_active = 0 AND id NOT IN (
				SELECT id FROM ml_models 
				WHERE model_type = ? 
				ORDER BY created_at DESC 
				LIMIT ?
			)`

		_, err := db.conn.ExecContext(ctx, query, modelType, modelType, keepCount)
		if err != nil {
			return fmt.Errorf("delete old models of type %s: %w", modelType, err)
		}
	}
	return nil
}

// settings operations

// GetSetting retrieves a setting value
func (db *DB) GetSetting(ctx context.Context, key string) (*Setting, error) {
	var setting Setting
	query := `SELECT * FROM settings WHERE key = ?`

	err := db.conn.GetContext(ctx, &setting, query, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get setting: %w", err)
	}
	return &setting, nil
}

// SetSetting creates or updates a setting
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	query := `
		INSERT OR REPLACE INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)`

	_, err := db.conn.ExecContext(ctx, query, key, value)
	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}

// GetAllSettings retrieves all settings
func (db *DB) GetAllSettings(ctx context.Context) ([]Setting, error) {
	query := `SELECT * FROM settings ORDER BY key`

	var settings []Setting
	err := db.conn.SelectContext(ctx, &settings, query)
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
	}
	return settings, nil
}

// DeleteSetting removes a setting
func (db *DB) DeleteSetting(ctx context.Context, key string) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("delete setting: %w", err)
	}
	return nil
}

// GetModelStats returns statistics about stored models
func (db *DB) GetModelStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// count models by type
	query := `
		SELECT model_type, COUNT(*) as count, MAX(datetime(created_at)) as latest
		FROM ml_models
		GROUP BY model_type`

	var modelStats []struct {
		Type   string         `db:"model_type"`
		Count  int64          `db:"count"`
		Latest sql.NullString `db:"latest"`
	}

	err := db.conn.SelectContext(ctx, &modelStats, query)
	if err != nil {
		return nil, fmt.Errorf("get model stats: %w", err)
	}

	modelsByType := make(map[string]interface{})
	for _, ms := range modelStats {
		modelsByType[ms.Type] = map[string]interface{}{
			"count":  ms.Count,
			"latest": ms.Latest.String,
		}
	}
	stats["models_by_type"] = modelsByType

	// get total model size
	var totalSize sql.NullInt64
	err = db.conn.GetContext(ctx, &totalSize,
		"SELECT SUM(LENGTH(model_data)) FROM ml_models")
	if err != nil {
		return nil, fmt.Errorf("get total model size: %w", err)
	}
	if totalSize.Valid {
		stats["total_model_size_bytes"] = totalSize.Int64
	}

	return stats, nil
}
