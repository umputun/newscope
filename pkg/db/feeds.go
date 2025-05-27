package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CreateFeed creates a new feed
func (db *DB) CreateFeed(ctx context.Context, feed *Feed) error {
	query := `
		INSERT INTO feeds (url, title, description, enabled, priority, fetch_interval)
		VALUES (:url, :title, :description, :enabled, :priority, :fetch_interval)
	`
	result, err := db.conn.NamedExecContext(ctx, query, feed)
	if err != nil {
		return fmt.Errorf("insert feed: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	feed.ID = id
	return nil
}

// GetFeed retrieves a feed by ID
func (db *DB) GetFeed(ctx context.Context, id int64) (*Feed, error) {
	var feed Feed
	query := `SELECT * FROM feeds WHERE id = ?`
	err := db.conn.GetContext(ctx, &feed, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("feed not found")
		}
		return nil, fmt.Errorf("get feed: %w", err)
	}
	return &feed, nil
}

// GetFeedByURL retrieves a feed by URL
func (db *DB) GetFeedByURL(ctx context.Context, url string) (*Feed, error) {
	var feed Feed
	query := `SELECT * FROM feeds WHERE url = ?`
	err := db.conn.GetContext(ctx, &feed, query, url)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("feed not found")
		}
		return nil, fmt.Errorf("get feed by url: %w", err)
	}
	return &feed, nil
}

// GetFeeds retrieves all feeds
func (db *DB) GetFeeds(ctx context.Context) ([]Feed, error) {
	var feeds []Feed
	query := `SELECT * FROM feeds ORDER BY title`
	err := db.conn.SelectContext(ctx, &feeds, query)
	if err != nil {
		return nil, fmt.Errorf("get feeds: %w", err)
	}
	return feeds, nil
}

// GetEnabledFeeds retrieves all enabled feeds
func (db *DB) GetEnabledFeeds(ctx context.Context) ([]Feed, error) {
	var feeds []Feed
	query := `SELECT * FROM feeds WHERE enabled = 1 ORDER BY title`
	err := db.conn.SelectContext(ctx, &feeds, query)
	if err != nil {
		return nil, fmt.Errorf("get enabled feeds: %w", err)
	}
	return feeds, nil
}

// GetFeedsWithStats retrieves feeds with statistics
func (db *DB) GetFeedsWithStats(ctx context.Context) ([]FeedWithStats, error) {
	var feeds []FeedWithStats
	query := `
		SELECT 
			f.*,
			COUNT(DISTINCT i.id) as item_count,
			COUNT(DISTINCT CASE WHEN i.content_extracted = 0 THEN i.id END) as unread_count,
			COUNT(DISTINCT CASE WHEN i.content_extracted = 1 THEN i.id END) as extracted_count,
			MAX(i.published) as last_item_published
		FROM feeds f
		LEFT JOIN items i ON f.id = i.feed_id
		GROUP BY f.id
		ORDER BY f.title
	`
	err := db.conn.SelectContext(ctx, &feeds, query)
	if err != nil {
		return nil, fmt.Errorf("get feeds with stats: %w", err)
	}
	return feeds, nil
}

// UpdateFeed updates a feed
func (db *DB) UpdateFeed(ctx context.Context, feed *Feed) error {
	feed.UpdatedAt = time.Now()
	query := `
		UPDATE feeds 
		SET title = :title,
		    description = :description,
		    last_fetched = :last_fetched,
		    last_error = :last_error,
		    error_count = :error_count,
		    enabled = :enabled,
		    updated_at = :updated_at
		WHERE id = :id
	`
	_, err := db.conn.NamedExecContext(ctx, query, feed)
	if err != nil {
		return fmt.Errorf("update feed: %w", err)
	}
	return nil
}

// UpdateFeedLastFetched updates the last fetched timestamp
func (db *DB) UpdateFeedLastFetched(ctx context.Context, feedID int64, lastFetched time.Time) error {
	// get feed to know its interval
	feed, err := db.GetFeed(ctx, feedID)
	if err != nil {
		return fmt.Errorf("get feed for interval: %w", err)
	}

	// calculate next fetch time based on interval
	nextFetch := lastFetched.Add(time.Duration(feed.FetchInterval) * time.Second)
	query := `
		UPDATE feeds 
		SET last_fetched = ?, 
		    next_fetch = ?,
		    last_error = NULL, 
		    error_count = 0, 
		    updated_at = ?
		WHERE id = ?
	`
	_, err = db.conn.ExecContext(ctx, query, lastFetched.UTC(), nextFetch.UTC(), time.Now().UTC(), feedID)
	if err != nil {
		return fmt.Errorf("update feed last fetched: %w", err)
	}
	return nil
}

// UpdateFeedError updates the feed error information
func (db *DB) UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error {
	query := `
		UPDATE feeds 
		SET last_error = ?, error_count = error_count + 1, updated_at = ?
		WHERE id = ?
	`
	_, err := db.conn.ExecContext(ctx, query, errMsg, time.Now(), feedID)
	if err != nil {
		return fmt.Errorf("update feed error: %w", err)
	}
	return nil
}

// DeleteFeed deletes a feed and all associated items
func (db *DB) DeleteFeed(ctx context.Context, id int64) error {
	query := `DELETE FROM feeds WHERE id = ?`
	result, err := db.conn.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("feed not found")
	}

	return nil
}

// GetFeedsDueForUpdate retrieves feeds that need to be updated
func (db *DB) GetFeedsDueForUpdate(ctx context.Context, limit int) ([]Feed, error) {
	query := `
		SELECT * FROM feeds 
		WHERE enabled = 1 
		  AND (next_fetch IS NULL OR next_fetch <= ?)
		ORDER BY priority DESC, next_fetch ASC
		LIMIT ?`

	var feeds []Feed
	err := db.conn.SelectContext(ctx, &feeds, query, time.Now().UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("get feeds due for update: %w", err)
	}
	return feeds, nil
}

// UpdateFeedPriority updates the priority of a feed
func (db *DB) UpdateFeedPriority(ctx context.Context, feedID int64, priority int) error {
	query := `UPDATE feeds SET priority = ?, updated_at = ? WHERE id = ?`
	_, err := db.conn.ExecContext(ctx, query, priority, time.Now(), feedID)
	if err != nil {
		return fmt.Errorf("update feed priority: %w", err)
	}
	return nil
}

// UpdateFeedInterval updates the fetch interval for a feed
func (db *DB) UpdateFeedInterval(ctx context.Context, feedID int64, intervalSeconds int) error {
	query := `
		UPDATE feeds 
		SET fetch_interval = ?, updated_at = ? 
		WHERE id = ?`
	_, err := db.conn.ExecContext(ctx, query, intervalSeconds, time.Now(), feedID)
	if err != nil {
		return fmt.Errorf("update feed interval: %w", err)
	}
	return nil
}

// SetFeedMetadata sets custom metadata for a feed
func (db *DB) SetFeedMetadata(ctx context.Context, feedID int64, metadata string) error {
	query := `UPDATE feeds SET metadata = ?, updated_at = ? WHERE id = ?`
	_, err := db.conn.ExecContext(ctx, query, metadata, time.Now(), feedID)
	if err != nil {
		return fmt.Errorf("set feed metadata: %w", err)
	}
	return nil
}

// GetFeedsByPriority retrieves feeds ordered by priority
func (db *DB) GetFeedsByPriority(ctx context.Context, minPriority int) ([]Feed, error) {
	query := `
		SELECT * FROM feeds 
		WHERE enabled = 1 AND priority >= ?
		ORDER BY priority DESC, title`

	var feeds []Feed
	err := db.conn.SelectContext(ctx, &feeds, query, minPriority)
	if err != nil {
		return nil, fmt.Errorf("get feeds by priority: %w", err)
	}
	return feeds, nil
}
