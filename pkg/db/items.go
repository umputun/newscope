package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

// UpdateItemContent updates the content fields after extraction
func (db *DB) UpdateItemContent(ctx context.Context, itemID int64, content, contentHTML, language, extractionMethod, extractionMode string, readTime, mediaCount int) error {
	// calculate content hash
	hash := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(hash[:])

	query := `
		UPDATE items SET
			content = ?, content_html = ?, content_hash = ?,
			language = ?, read_time = ?, media_count = ?,
			extraction_method = ?, extraction_mode = ?,
			content_extracted = 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	_, err := db.conn.ExecContext(ctx, query,
		sql.NullString{String: content, Valid: content != ""},
		sql.NullString{String: contentHTML, Valid: contentHTML != ""},
		sql.NullString{String: contentHash, Valid: true},
		sql.NullString{String: language, Valid: language != ""},
		sql.NullInt64{Int64: int64(readTime), Valid: readTime > 0},
		sql.NullInt64{Int64: int64(mediaCount), Valid: true},
		sql.NullString{String: extractionMethod, Valid: extractionMethod != ""},
		sql.NullString{String: extractionMode, Valid: extractionMode != ""},
		itemID,
	)

	if err != nil {
		return fmt.Errorf("update item content: %w", err)
	}
	return nil
}

// GetItemsByContentHash finds items with the same content hash
func (db *DB) GetItemsByContentHash(ctx context.Context, contentHash string) ([]Item, error) {
	query := `
		SELECT * FROM items 
		WHERE content_hash = ? 
		ORDER BY published DESC`

	var items []Item
	err := db.conn.SelectContext(ctx, &items, query, contentHash)
	if err != nil {
		return nil, fmt.Errorf("get items by content hash: %w", err)
	}
	return items, nil
}

// GetItemsWithoutContent retrieves items that need content extraction
func (db *DB) GetItemsWithoutContent(ctx context.Context, limit int) ([]Item, error) {
	query := `
		SELECT * FROM items 
		WHERE content_extracted = 0 
		  AND published > datetime('now', '-7 days')
		ORDER BY published DESC
		LIMIT ?`

	var items []Item
	err := db.conn.SelectContext(ctx, &items, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get items without content: %w", err)
	}
	return items, nil
}

// GetItemsByLanguage retrieves items by language
func (db *DB) GetItemsByLanguage(ctx context.Context, language string, limit, offset int) ([]Item, error) {
	query := `
		SELECT * FROM items 
		WHERE language = ? 
		ORDER BY published DESC
		LIMIT ? OFFSET ?`

	var items []Item
	err := db.conn.SelectContext(ctx, &items, query, language, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get items by language: %w", err)
	}
	return items, nil
}

// SearchItemsFullText performs full-text search on items
func (db *DB) SearchItemsFullText(ctx context.Context, query string, limit, offset int) ([]Item, error) {
	searchQuery := `
		SELECT i.* FROM items i
		JOIN items_fts ON items_fts.rowid = i.id
		WHERE items_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?`

	var items []Item
	err := db.conn.SelectContext(ctx, &items, searchQuery, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search items full text: %w", err)
	}
	return items, nil
}

// GetItemStats returns statistics about items
func (db *DB) GetItemStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// total items
	var totalItems int64
	err := db.conn.GetContext(ctx, &totalItems, "SELECT COUNT(*) FROM items")
	if err != nil {
		return nil, fmt.Errorf("get total items: %w", err)
	}
	stats["total_items"] = totalItems

	// items with content
	var itemsWithContent int64
	err = db.conn.GetContext(ctx, &itemsWithContent,
		"SELECT COUNT(*) FROM items WHERE content_extracted = 1")
	if err != nil {
		return nil, fmt.Errorf("get items with content: %w", err)
	}
	stats["items_with_content"] = itemsWithContent

	// average read time
	var avgReadTime sql.NullFloat64
	err = db.conn.GetContext(ctx, &avgReadTime,
		"SELECT AVG(read_time) FROM items WHERE read_time IS NOT NULL")
	if err != nil {
		return nil, fmt.Errorf("get average read time: %w", err)
	}
	if avgReadTime.Valid {
		stats["avg_read_time_minutes"] = avgReadTime.Float64
	}

	// items by language
	query := `
		SELECT language, COUNT(*) as count
		FROM items
		WHERE language IS NOT NULL
		GROUP BY language
		ORDER BY count DESC
		LIMIT 10`

	var langCounts []struct {
		Language string `db:"language"`
		Count    int64  `db:"count"`
	}

	err = db.conn.SelectContext(ctx, &langCounts, query)
	if err != nil {
		return nil, fmt.Errorf("get language counts: %w", err)
	}

	languages := make(map[string]int64)
	for _, lc := range langCounts {
		languages[lc.Language] = lc.Count
	}
	stats["items_by_language"] = languages

	// extraction methods
	query = `
		SELECT extraction_method, COUNT(*) as count
		FROM items
		WHERE extraction_method IS NOT NULL
		GROUP BY extraction_method`

	var methodCounts []struct {
		Method string `db:"extraction_method"`
		Count  int64  `db:"count"`
	}

	err = db.conn.SelectContext(ctx, &methodCounts, query)
	if err != nil {
		return nil, fmt.Errorf("get extraction method counts: %w", err)
	}

	methods := make(map[string]int64)
	for _, mc := range methodCounts {
		methods[mc.Method] = mc.Count
	}
	stats["extraction_methods"] = methods

	return stats, nil
}

// GetDuplicateItems finds potential duplicate items based on content hash
func (db *DB) GetDuplicateItems(ctx context.Context, limit int) ([]struct {
	ContentHash string `db:"content_hash"`
	Count       int    `db:"count"`
	Items       []Item
}, error) {
	// first get content hashes with duplicates
	query := `
		SELECT content_hash, COUNT(*) as count
		FROM items
		WHERE content_hash IS NOT NULL
		GROUP BY content_hash
		HAVING count > 1
		ORDER BY count DESC
		LIMIT ?`

	var duplicates []struct {
		ContentHash string `db:"content_hash"`
		Count       int    `db:"count"`
	}

	err := db.conn.SelectContext(ctx, &duplicates, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get duplicate hashes: %w", err)
	}

	// fetch items for each duplicate hash
	results := make([]struct {
		ContentHash string `db:"content_hash"`
		Count       int    `db:"count"`
		Items       []Item
	}, len(duplicates))

	for i, dup := range duplicates {
		items, err := db.GetItemsByContentHash(ctx, dup.ContentHash)
		if err != nil {
			return nil, fmt.Errorf("get items for hash %s: %w", dup.ContentHash, err)
		}

		results[i] = struct {
			ContentHash string `db:"content_hash"`
			Count       int    `db:"count"`
			Items       []Item
		}{
			ContentHash: dup.ContentHash,
			Count:       dup.Count,
			Items:       items,
		}
	}

	return results, nil
}

// CleanupOldItems removes items older than the specified duration (enhanced version)
func (db *DB) CleanupOldItems(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	result, err := db.conn.ExecContext(ctx,
		"DELETE FROM items WHERE published < ? OR (published IS NULL AND created_at < ?)",
		cutoff, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleanup old items: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}
	return affected, nil
}
