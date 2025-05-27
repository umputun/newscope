package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMLModelOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("create and get ML model", func(t *testing.T) {
		model := &MLModel{
			ModelType: "naive_bayes",
			ModelData: []byte("mock model data"),
			FeatureConfig: sql.NullString{
				String: `{"features": ["tfidf", "metadata"], "max_features": 10000}`,
				Valid:  true,
			},
			TrainingStats: sql.NullString{
				String: `{"accuracy": 0.85, "precision": 0.82, "recall": 0.88}`,
				Valid:  true,
			},
			SampleCount: sql.NullInt64{Int64: 1000, Valid: true},
			CreatedAt:   time.Now(),
			IsActive:    true,
		}

		err := db.CreateMLModel(ctx, model)
		require.NoError(t, err)
		assert.NotZero(t, model.ID)

		// get model by ID
		retrieved, err := db.GetModel(ctx, model.ID)
		require.NoError(t, err)
		assert.Equal(t, model.ModelType, retrieved.ModelType)
		assert.Equal(t, model.ModelData, retrieved.ModelData)
		assert.Equal(t, model.IsActive, retrieved.IsActive)
	})

	t.Run("get active model", func(t *testing.T) {
		// create inactive model
		inactiveModel := &MLModel{
			ModelType: "naive_bayes",
			ModelData: []byte("old model"),
			CreatedAt: time.Now().Add(-24 * time.Hour),
			IsActive:  false,
		}
		err := db.CreateMLModel(ctx, inactiveModel)
		require.NoError(t, err)

		// create active model
		activeModel := &MLModel{
			ModelType: "naive_bayes",
			ModelData: []byte("new model"),
			CreatedAt: time.Now(),
			IsActive:  true,
		}
		err = db.CreateMLModel(ctx, activeModel)
		require.NoError(t, err)

		// get active model
		retrieved, err := db.GetActiveModel(ctx, "naive_bayes")
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, activeModel.ID, retrieved.ID)
		assert.True(t, retrieved.IsActive)
	})

	t.Run("set active model", func(t *testing.T) {
		// create multiple models
		models := []MLModel{
			{ModelType: "logistic", ModelData: []byte("model1"), CreatedAt: time.Now(), IsActive: true},
			{ModelType: "logistic", ModelData: []byte("model2"), CreatedAt: time.Now(), IsActive: false},
			{ModelType: "logistic", ModelData: []byte("model3"), CreatedAt: time.Now(), IsActive: false},
		}

		for i := range models {
			err := db.CreateMLModel(ctx, &models[i])
			require.NoError(t, err)
		}

		// set model2 as active
		err := db.SetActiveModel(ctx, models[1].ID, "logistic")
		require.NoError(t, err)

		// verify only model2 is active
		active, err := db.GetActiveModel(ctx, "logistic")
		require.NoError(t, err)
		assert.Equal(t, models[1].ID, active.ID)

		// verify others are inactive
		all, err := db.GetModelHistory(ctx, "logistic", 10)
		require.NoError(t, err)
		activeCount := 0
		for _, m := range all {
			if m.IsActive {
				activeCount++
				assert.Equal(t, models[1].ID, m.ID)
			}
		}
		assert.Equal(t, 1, activeCount)
	})

	t.Run("get model history", func(t *testing.T) {
		// create models with different timestamps
		baseTime := time.Now()
		models := []MLModel{
			{ModelType: "svm", ModelData: []byte("v1"), CreatedAt: baseTime.Add(-3 * time.Hour)},
			{ModelType: "svm", ModelData: []byte("v2"), CreatedAt: baseTime.Add(-2 * time.Hour)},
			{ModelType: "svm", ModelData: []byte("v3"), CreatedAt: baseTime.Add(-1 * time.Hour)},
			{ModelType: "svm", ModelData: []byte("v4"), CreatedAt: baseTime},
		}

		for i := range models {
			err := db.CreateMLModel(ctx, &models[i])
			require.NoError(t, err)
		}

		// get history
		history, err := db.GetModelHistory(ctx, "svm", 3)
		require.NoError(t, err)
		assert.Len(t, history, 3)

		// verify order (newest first)
		assert.Equal(t, []byte("v4"), history[0].ModelData)
		assert.Equal(t, []byte("v3"), history[1].ModelData)
		assert.Equal(t, []byte("v2"), history[2].ModelData)
	})

	t.Run("delete old models", func(t *testing.T) {
		// get current svm models
		svmModels, err := db.GetModelHistory(ctx, "svm", 10)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(svmModels), 2)

		// set one model as active
		err = db.SetActiveModel(ctx, svmModels[0].ID, "svm")
		require.NoError(t, err)

		// delete old models, keeping only 2
		err = db.DeleteOldModels(ctx, 2)
		require.NoError(t, err)

		// verify only 2 models remain (including active)
		remaining, err := db.GetModelHistory(ctx, "svm", 10)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(remaining), 2)
	})

	t.Run("get model stats", func(t *testing.T) {
		stats, err := db.GetModelStats(ctx)
		require.NoError(t, err)

		modelsByType := stats["models_by_type"].(map[string]interface{})

		// check we have entries for each type
		assert.Contains(t, modelsByType, "naive_bayes")
		assert.Contains(t, modelsByType, "logistic")
		assert.Contains(t, modelsByType, "svm")

		// check total size is reasonable
		if totalSize, ok := stats["total_model_size_bytes"]; ok {
			assert.Positive(t, totalSize.(int64))
		}
	})
}

func TestSettingsOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("set and get setting", func(t *testing.T) {
		// set a setting
		settingValue := map[string]interface{}{
			"threshold": 0.5,
			"enabled":   true,
			"features":  []string{"tfidf", "metadata"},
		}

		jsonValue, err := json.Marshal(settingValue)
		require.NoError(t, err)

		err = db.SetSetting(ctx, "classifier_config", string(jsonValue))
		require.NoError(t, err)

		// get setting
		retrieved, err := db.GetSetting(ctx, "classifier_config")
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, "classifier_config", retrieved.Key)

		// parse value
		var parsedValue map[string]interface{}
		err = json.Unmarshal([]byte(retrieved.Value), &parsedValue)
		require.NoError(t, err)
		assert.InDelta(t, 0.5, parsedValue["threshold"], 0.01)
		assert.Equal(t, true, parsedValue["enabled"])
	})

	t.Run("update existing setting", func(t *testing.T) {
		// update the setting
		newValue := `{"threshold": 0.7, "enabled": false}`
		err := db.SetSetting(ctx, "classifier_config", newValue)
		require.NoError(t, err)

		// verify update
		retrieved, err := db.GetSetting(ctx, "classifier_config")
		require.NoError(t, err)
		assert.Equal(t, newValue, retrieved.Value)
		assert.True(t, retrieved.UpdatedAt.After(time.Now().Add(-1*time.Minute)))
	})

	t.Run("get all settings", func(t *testing.T) {
		// add more settings
		settings := map[string]string{
			"feed_update_interval": "300",
			"max_workers":          "10",
			"debug_mode":           "false",
		}

		for k, v := range settings {
			err := db.SetSetting(ctx, k, v)
			require.NoError(t, err)
		}

		// get all settings
		all, err := db.GetAllSettings(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(all), 4) // at least our 4 settings

		// verify they're ordered by key
		for i := 1; i < len(all); i++ {
			assert.Less(t, all[i-1].Key, all[i].Key)
		}
	})

	t.Run("get non-existent setting", func(t *testing.T) {
		setting, err := db.GetSetting(ctx, "non_existent_key")
		require.NoError(t, err)
		assert.Nil(t, setting)
	})

	t.Run("delete setting", func(t *testing.T) {
		// create a setting to delete
		err := db.SetSetting(ctx, "temp_setting", "temp_value")
		require.NoError(t, err)

		// verify it exists
		setting, err := db.GetSetting(ctx, "temp_setting")
		require.NoError(t, err)
		require.NotNil(t, setting)

		// delete it
		err = db.DeleteSetting(ctx, "temp_setting")
		require.NoError(t, err)

		// verify it's gone
		setting, err = db.GetSetting(ctx, "temp_setting")
		require.NoError(t, err)
		assert.Nil(t, setting)
	})
}
