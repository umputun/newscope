package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// CreateContent creates extracted content for an item
func (db *DB) CreateContent(ctx context.Context, content *Content) error {
	return db.InTransaction(ctx, func(tx *sqlx.Tx) error {
		// insert content
		query := `
			INSERT INTO content (item_id, full_content, extraction_error)
			VALUES (:item_id, :full_content, :extraction_error)
		`
		result, err := tx.NamedExecContext(ctx, query, content)
		if err != nil {
			return fmt.Errorf("insert content: %w", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get last insert id: %w", err)
		}
		content.ID = id

		// update item to mark content as extracted
		updateQuery := `UPDATE items SET content_extracted = 1, updated_at = ? WHERE id = ?`
		if _, err := tx.ExecContext(ctx, updateQuery, time.Now(), content.ItemID); err != nil {
			return fmt.Errorf("update item: %w", err)
		}

		// update FTS index with content if extraction was successful
		if !content.ExtractionError.Valid || content.ExtractionError.String == "" {
			ftsQuery := `UPDATE items_fts SET content = ? WHERE rowid = ?`
			if _, err := tx.ExecContext(ctx, ftsQuery, content.FullContent, content.ItemID); err != nil {
				return fmt.Errorf("update fts index: %w", err)
			}
		}

		return nil
	})
}

// GetContent retrieves content by item ID
func (db *DB) GetContent(ctx context.Context, itemID int64) (*Content, error) {
	var content Content
	query := `SELECT * FROM content WHERE item_id = ?`
	err := db.conn.GetContext(ctx, &content, query, itemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("content not found")
		}
		return nil, fmt.Errorf("get content: %w", err)
	}
	return &content, nil
}

// UpdateContent updates existing content
func (db *DB) UpdateContent(ctx context.Context, content *Content) error {
	query := `
		UPDATE content 
		SET full_content = :full_content,
		    extraction_error = :extraction_error,
		    extracted_at = :extracted_at
		WHERE item_id = :item_id
	`
	_, err := db.conn.NamedExecContext(ctx, query, content)
	if err != nil {
		return fmt.Errorf("update content: %w", err)
	}
	return nil
}

// DeleteContent deletes content for an item
func (db *DB) DeleteContent(ctx context.Context, itemID int64) error {
	return db.InTransaction(ctx, func(tx *sqlx.Tx) error {
		// delete content
		query := `DELETE FROM content WHERE item_id = ?`
		if _, err := tx.ExecContext(ctx, query, itemID); err != nil {
			return fmt.Errorf("delete content: %w", err)
		}

		// update item to mark content as not extracted
		updateQuery := `UPDATE items SET content_extracted = 0, updated_at = ? WHERE id = ?`
		if _, err := tx.ExecContext(ctx, updateQuery, time.Now(), itemID); err != nil {
			return fmt.Errorf("update item: %w", err)
		}

		// clear content from FTS index
		ftsQuery := `UPDATE items_fts SET content = NULL WHERE rowid = ?`
		if _, err := tx.ExecContext(ctx, ftsQuery, itemID); err != nil {
			return fmt.Errorf("update fts index: %w", err)
		}

		return nil
	})
}

// CreateContentError records a content extraction error
func (db *DB) CreateContentError(ctx context.Context, itemID int64, err error) error {
	content := &Content{
		ItemID:          itemID,
		ExtractionError: sql.NullString{String: err.Error(), Valid: true},
	}
	return db.CreateContent(ctx, content)
}

// GetContentStats returns statistics about content extraction
func (db *DB) GetContentStats(ctx context.Context) (map[string]int64, error) {
	stats := make(map[string]int64)

	// total items
	var totalItems int64
	if err := db.conn.GetContext(ctx, &totalItems, `SELECT COUNT(*) FROM items`); err != nil {
		return nil, fmt.Errorf("count total items: %w", err)
	}
	stats["total_items"] = totalItems

	// items with content
	var withContent int64
	if err := db.conn.GetContext(ctx, &withContent, `SELECT COUNT(*) FROM items WHERE content_extracted = 1`); err != nil {
		return nil, fmt.Errorf("count items with content: %w", err)
	}
	stats["with_content"] = withContent

	// items without content
	stats["without_content"] = totalItems - withContent

	// extraction errors
	var withErrors int64
	if err := db.conn.GetContext(ctx, &withErrors, `SELECT COUNT(*) FROM content WHERE extraction_error IS NOT NULL`); err != nil {
		return nil, fmt.Errorf("count extraction errors: %w", err)
	}
	stats["extraction_errors"] = withErrors

	return stats, nil
}
