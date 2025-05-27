package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// category management operations

// CreateCategory creates a new category
func (db *DB) CreateCategory(ctx context.Context, category *Category) error {
	// ensure keywords is valid JSON
	if category.Keywords == "" {
		category.Keywords = "[]"
	}

	query := `
		INSERT INTO categories (
			name, keywords, is_positive, weight, parent_id, active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := db.conn.ExecContext(ctx, query,
		category.Name,
		category.Keywords,
		category.IsPositive,
		category.Weight,
		category.ParentID,
		category.Active,
		category.CreatedAt,
		category.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create category: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	category.ID = id
	return nil
}

// GetCategory retrieves a category by ID
func (db *DB) GetCategory(ctx context.Context, id int64) (*Category, error) {
	var category Category
	query := `SELECT * FROM categories WHERE id = ?`

	err := db.conn.GetContext(ctx, &category, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get category: %w", err)
	}
	return &category, nil
}

// GetCategoryByName retrieves a category by name
func (db *DB) GetCategoryByName(ctx context.Context, name string) (*Category, error) {
	var category Category
	query := `SELECT * FROM categories WHERE name = ?`

	err := db.conn.GetContext(ctx, &category, query, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get category by name: %w", err)
	}
	return &category, nil
}

// GetActiveCategories retrieves all active categories
func (db *DB) GetActiveCategories(ctx context.Context) ([]Category, error) {
	query := `SELECT * FROM categories WHERE active = 1 ORDER BY name`

	var categories []Category
	err := db.conn.SelectContext(ctx, &categories, query)
	if err != nil {
		return nil, fmt.Errorf("get active categories: %w", err)
	}
	return categories, nil
}

// UpdateCategory updates a category
func (db *DB) UpdateCategory(ctx context.Context, category *Category) error {
	query := `
		UPDATE categories SET
			name = ?, keywords = ?, is_positive = ?, weight = ?,
			parent_id = ?, active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	_, err := db.conn.ExecContext(ctx, query,
		category.Name,
		category.Keywords,
		category.IsPositive,
		category.Weight,
		category.ParentID,
		category.Active,
		category.ID,
	)
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	return nil
}

// DeleteCategory deletes a category
func (db *DB) DeleteCategory(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM categories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	return nil
}

// AssignItemCategory assigns a category to an item
func (db *DB) AssignItemCategory(ctx context.Context, itemID, categoryID int64, confidence float64) error {
	query := `
		INSERT OR REPLACE INTO item_categories (item_id, category_id, confidence)
		VALUES (?, ?, ?)`

	_, err := db.conn.ExecContext(ctx, query, itemID, categoryID, confidence)
	if err != nil {
		return fmt.Errorf("assign item category: %w", err)
	}
	return nil
}

// GetItemCategories retrieves categories assigned to an item
func (db *DB) GetItemCategories(ctx context.Context, itemID int64) ([]Category, error) {
	query := `
		SELECT c.* FROM categories c
		JOIN item_categories ic ON ic.category_id = c.id
		WHERE ic.item_id = ?
		ORDER BY ic.confidence DESC, c.name`

	var categories []Category
	err := db.conn.SelectContext(ctx, &categories, query, itemID)
	if err != nil {
		return nil, fmt.Errorf("get item categories: %w", err)
	}
	return categories, nil
}

// GetItemCategoriesWithConfidence retrieves categories with confidence scores
func (db *DB) GetItemCategoriesWithConfidence(ctx context.Context, itemID int64) ([]struct {
	Category
	Confidence float64 `db:"confidence"`
}, error) {
	query := `
		SELECT c.*, ic.confidence FROM categories c
		JOIN item_categories ic ON ic.category_id = c.id
		WHERE ic.item_id = ?
		ORDER BY ic.confidence DESC, c.name`

	var results []struct {
		Category
		Confidence float64 `db:"confidence"`
	}
	err := db.conn.SelectContext(ctx, &results, query, itemID)
	if err != nil {
		return nil, fmt.Errorf("get item categories with confidence: %w", err)
	}
	return results, nil
}

// GetItemsByCategory retrieves items in a specific category
func (db *DB) GetItemsByCategory(ctx context.Context, categoryID int64, limit, offset int) ([]Item, error) {
	query := `
		SELECT i.* FROM items i
		JOIN item_categories ic ON ic.item_id = i.id
		WHERE ic.category_id = ?
		ORDER BY i.published DESC
		LIMIT ? OFFSET ?`

	var items []Item
	err := db.conn.SelectContext(ctx, &items, query, categoryID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get items by category: %w", err)
	}
	return items, nil
}

// RemoveItemCategory removes a category assignment from an item
func (db *DB) RemoveItemCategory(ctx context.Context, itemID, categoryID int64) error {
	query := `DELETE FROM item_categories WHERE item_id = ? AND category_id = ?`
	_, err := db.conn.ExecContext(ctx, query, itemID, categoryID)
	if err != nil {
		return fmt.Errorf("remove item category: %w", err)
	}
	return nil
}

// RemoveAllItemCategories removes all category assignments from an item
func (db *DB) RemoveAllItemCategories(ctx context.Context, itemID int64) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM item_categories WHERE item_id = ?", itemID)
	if err != nil {
		return fmt.Errorf("remove all item categories: %w", err)
	}
	return nil
}

// GetCategoryStats returns statistics about categories
func (db *DB) GetCategoryStats(ctx context.Context) ([]struct {
	CategoryID    int64           `db:"category_id"`
	CategoryName  string          `db:"category_name"`
	ItemCount     int64           `db:"item_count"`
	AvgConfidence sql.NullFloat64 `db:"avg_confidence"`
}, error) {
	query := `
		SELECT 
			c.id as category_id,
			c.name as category_name,
			COUNT(ic.item_id) as item_count,
			AVG(ic.confidence) as avg_confidence
		FROM categories c
		LEFT JOIN item_categories ic ON ic.category_id = c.id
		WHERE c.active = 1
		GROUP BY c.id, c.name
		ORDER BY item_count DESC`

	var stats []struct {
		CategoryID    int64           `db:"category_id"`
		CategoryName  string          `db:"category_name"`
		ItemCount     int64           `db:"item_count"`
		AvgConfidence sql.NullFloat64 `db:"avg_confidence"`
	}

	err := db.conn.SelectContext(ctx, &stats, query)
	if err != nil {
		return nil, fmt.Errorf("get category stats: %w", err)
	}
	return stats, nil
}

// helper methods for Category

// SetKeywords sets the keywords for a category from a slice
func (c *Category) SetKeywords(keywords []string) error {
	data, err := json.Marshal(keywords)
	if err != nil {
		return fmt.Errorf("marshal keywords: %w", err)
	}
	c.Keywords = string(data)
	return nil
}

// GetKeywords retrieves the keywords as a slice
func (c *Category) GetKeywords() ([]string, error) {
	if c.Keywords == "" {
		return []string{}, nil
	}

	var keywords []string
	err := json.Unmarshal([]byte(c.Keywords), &keywords)
	if err != nil {
		return nil, fmt.Errorf("unmarshal keywords: %w", err)
	}
	return keywords, nil
}
