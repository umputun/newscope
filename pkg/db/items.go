package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// CreateItem creates a new item
func (db *DB) CreateItem(ctx context.Context, item *Item) error {
	query := `
		INSERT INTO items (feed_id, guid, title, link, description, published, author)
		VALUES (:feed_id, :guid, :title, :link, :description, :published, :author)
		ON CONFLICT(feed_id, guid) DO NOTHING
	`
	result, err := db.conn.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("insert item: %w", err)
	}

	// check if item was actually inserted (not a duplicate)
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get last insert id: %w", err)
		}
		item.ID = id
	}

	return nil
}

// CreateItems creates multiple items in a single transaction
func (db *DB) CreateItems(ctx context.Context, items []Item) error {
	return db.InTransaction(ctx, func(tx *sqlx.Tx) error {
		query := `
			INSERT INTO items (feed_id, guid, title, link, description, published, author)
			VALUES (:feed_id, :guid, :title, :link, :description, :published, :author)
			ON CONFLICT(feed_id, guid) DO NOTHING
		`
		for _, item := range items {
			if _, err := tx.NamedExecContext(ctx, query, item); err != nil {
				return fmt.Errorf("insert item: %w", err)
			}
		}
		return nil
	})
}

// GetItem retrieves an item by ID
func (db *DB) GetItem(ctx context.Context, id int64) (*Item, error) {
	var item Item
	query := `SELECT * FROM items WHERE id = ?`
	err := db.conn.GetContext(ctx, &item, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("item not found")
		}
		return nil, fmt.Errorf("get item: %w", err)
	}
	return &item, nil
}

// GetItemByGUID retrieves an item by feed ID and GUID
func (db *DB) GetItemByGUID(ctx context.Context, feedID int64, guid string) (*Item, error) {
	var item Item
	query := `SELECT * FROM items WHERE feed_id = ? AND guid = ?`
	err := db.conn.GetContext(ctx, &item, query, feedID, guid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("item not found")
		}
		return nil, fmt.Errorf("get item by guid: %w", err)
	}
	return &item, nil
}

// GetItemWithContent retrieves an item with its extracted content
func (db *DB) GetItemWithContent(ctx context.Context, id int64) (*ItemWithContent, error) {
	var item ItemWithContent
	query := `
		SELECT i.*, c.full_content, c.extracted_at, c.extraction_error
		FROM items i
		LEFT JOIN content c ON i.id = c.item_id
		WHERE i.id = ?
	`
	err := db.conn.GetContext(ctx, &item, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("item not found")
		}
		return nil, fmt.Errorf("get item with content: %w", err)
	}
	return &item, nil
}

// GetItems retrieves items with pagination
func (db *DB) GetItems(ctx context.Context, limit, offset int) ([]Item, error) {
	var items []Item
	query := `
		SELECT * FROM items 
		ORDER BY published DESC, created_at DESC
		LIMIT ? OFFSET ?
	`
	err := db.conn.SelectContext(ctx, &items, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get items: %w", err)
	}
	return items, nil
}

// GetItemsByFeed retrieves items for a specific feed
func (db *DB) GetItemsByFeed(ctx context.Context, feedID int64, limit, offset int) ([]Item, error) {
	var items []Item
	query := `
		SELECT * FROM items 
		WHERE feed_id = ?
		ORDER BY published DESC, created_at DESC
		LIMIT ? OFFSET ?
	`
	err := db.conn.SelectContext(ctx, &items, query, feedID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get items by feed: %w", err)
	}
	return items, nil
}

// GetItemsWithContent retrieves items with their content
func (db *DB) GetItemsWithContent(ctx context.Context, limit, offset int) ([]ItemWithContent, error) {
	var items []ItemWithContent
	query := `
		SELECT i.*, c.full_content, c.extracted_at, c.extraction_error
		FROM items i
		LEFT JOIN content c ON i.id = c.item_id
		ORDER BY i.published DESC, i.created_at DESC
		LIMIT ? OFFSET ?
	`
	err := db.conn.SelectContext(ctx, &items, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get items with content: %w", err)
	}
	return items, nil
}

// GetItemsForExtraction retrieves items that need content extraction
func (db *DB) GetItemsForExtraction(ctx context.Context, limit int) ([]Item, error) {
	var items []Item
	query := `
		SELECT i.* FROM items i
		LEFT JOIN content c ON i.id = c.item_id
		WHERE i.content_extracted = 0 AND c.id IS NULL
		ORDER BY i.published DESC
		LIMIT ?
	`
	err := db.conn.SelectContext(ctx, &items, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get items for extraction: %w", err)
	}
	return items, nil
}

// SearchItems performs full-text search on items
func (db *DB) SearchItems(ctx context.Context, query string, limit, offset int) ([]ItemWithContent, error) {
	var items []ItemWithContent
	searchQuery := `
		SELECT i.*, c.full_content, c.extracted_at, c.extraction_error
		FROM items i
		LEFT JOIN content c ON i.id = c.item_id
		WHERE i.id IN (
			SELECT rowid FROM items_fts WHERE items_fts MATCH ?
		)
		ORDER BY i.published DESC
		LIMIT ? OFFSET ?
	`
	err := db.conn.SelectContext(ctx, &items, searchQuery, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search items: %w", err)
	}
	return items, nil
}

// CountItems returns the total number of items
func (db *DB) CountItems(ctx context.Context) (int64, error) {
	var count int64
	query := `SELECT COUNT(*) FROM items`
	err := db.conn.GetContext(ctx, &count, query)
	if err != nil {
		return 0, fmt.Errorf("count items: %w", err)
	}
	return count, nil
}

// CountItemsByFeed returns the number of items for a feed
func (db *DB) CountItemsByFeed(ctx context.Context, feedID int64) (int64, error) {
	var count int64
	query := `SELECT COUNT(*) FROM items WHERE feed_id = ?`
	err := db.conn.GetContext(ctx, &count, query, feedID)
	if err != nil {
		return 0, fmt.Errorf("count items by feed: %w", err)
	}
	return count, nil
}

// UpdateItemContentExtracted marks an item as having content extracted
func (db *DB) UpdateItemContentExtracted(ctx context.Context, itemID int64) error {
	query := `UPDATE items SET content_extracted = 1, updated_at = ? WHERE id = ?`
	_, err := db.conn.ExecContext(ctx, query, time.Now(), itemID)
	if err != nil {
		return fmt.Errorf("update item content extracted: %w", err)
	}
	return nil
}

// DeleteItem deletes an item
func (db *DB) DeleteItem(ctx context.Context, id int64) error {
	query := `DELETE FROM items WHERE id = ?`
	result, err := db.conn.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("item not found")
	}

	return nil
}

// DeleteOldItems deletes items older than the specified duration
func (db *DB) DeleteOldItems(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	query := `DELETE FROM items WHERE created_at < ?`
	result, err := db.conn.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old items: %w", err)
	}

	return result.RowsAffected()
}
