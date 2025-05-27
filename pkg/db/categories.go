package db

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// AddCategory adds a category to an item
func (db *DB) AddCategory(ctx context.Context, itemID int64, category string) error {
	query := `INSERT INTO categories (item_id, category) VALUES (?, ?)`
	_, err := db.conn.ExecContext(ctx, query, itemID, category)
	if err != nil {
		return fmt.Errorf("add category: %w", err)
	}
	return nil
}

// AddCategories adds multiple categories to an item
func (db *DB) AddCategories(ctx context.Context, itemID int64, categories []string) error {
	return db.InTransaction(ctx, func(tx *sqlx.Tx) error {
		query := `INSERT INTO categories (item_id, category) VALUES (?, ?)`
		for _, category := range categories {
			if _, err := tx.ExecContext(ctx, query, itemID, category); err != nil {
				return fmt.Errorf("add category %s: %w", category, err)
			}
		}
		return nil
	})
}

// GetItemCategories retrieves all categories for an item
func (db *DB) GetItemCategories(ctx context.Context, itemID int64) ([]string, error) {
	var categories []string
	query := `SELECT category FROM categories WHERE item_id = ? ORDER BY category`
	err := db.conn.SelectContext(ctx, &categories, query, itemID)
	if err != nil {
		return nil, fmt.Errorf("get item categories: %w", err)
	}
	return categories, nil
}

// GetItemsByCategory retrieves items with a specific category
func (db *DB) GetItemsByCategory(ctx context.Context, category string, limit, offset int) ([]Item, error) {
	var items []Item
	query := `
		SELECT i.* FROM items i
		JOIN categories c ON i.id = c.item_id
		WHERE c.category = ?
		ORDER BY i.published DESC
		LIMIT ? OFFSET ?
	`
	err := db.conn.SelectContext(ctx, &items, query, category, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get items by category: %w", err)
	}
	return items, nil
}

// GetAllCategories retrieves all unique categories
func (db *DB) GetAllCategories(ctx context.Context) ([]string, error) {
	var categories []string
	query := `SELECT DISTINCT category FROM categories ORDER BY category`
	err := db.conn.SelectContext(ctx, &categories, query)
	if err != nil {
		return nil, fmt.Errorf("get all categories: %w", err)
	}
	return categories, nil
}

// GetCategoriesWithCounts retrieves categories with item counts
func (db *DB) GetCategoriesWithCounts(ctx context.Context) (map[string]int64, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT category, COUNT(*) as count 
		FROM categories 
		GROUP BY category 
		ORDER BY count DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query categories with counts: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var category string
		var count int64
		if err := rows.Scan(&category, &count); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		result[category] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return result, nil
}

// RemoveCategory removes a category from an item
func (db *DB) RemoveCategory(ctx context.Context, itemID int64, category string) error {
	query := `DELETE FROM categories WHERE item_id = ? AND category = ?`
	_, err := db.conn.ExecContext(ctx, query, itemID, category)
	if err != nil {
		return fmt.Errorf("remove category: %w", err)
	}
	return nil
}

// RemoveAllCategories removes all categories from an item
func (db *DB) RemoveAllCategories(ctx context.Context, itemID int64) error {
	query := `DELETE FROM categories WHERE item_id = ?`
	_, err := db.conn.ExecContext(ctx, query, itemID)
	if err != nil {
		return fmt.Errorf("remove all categories: %w", err)
	}
	return nil
}
