package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// SettingRepository handles setting-related database operations
type SettingRepository struct {
	db *sqlx.DB
}

// NewSettingRepository creates a new setting repository
func NewSettingRepository(db *sqlx.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

// GetSetting retrieves a setting value
func (r *SettingRepository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.GetContext(ctx, &value, "SELECT value FROM settings WHERE key = ?", key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting: %w", err)
	}
	return value, nil
}

// SetSetting stores a setting value
func (r *SettingRepository) SetSetting(ctx context.Context, key, value string) error {
	query := `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`
	_, err := r.db.ExecContext(ctx, query, key, value)
	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}
