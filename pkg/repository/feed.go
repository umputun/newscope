package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/go-pkgz/repeater/v2"
	"github.com/jmoiron/sqlx"

	"github.com/umputun/newscope/pkg/domain"
)

// FeedRepository handles feed-related database operations
type FeedRepository struct {
	db *sqlx.DB
}

// feedSQL represents a feed for SQL operations
type feedSQL struct {
	ID            int64      `db:"id"`
	URL           string     `db:"url"`
	Title         string     `db:"title"`
	Description   string     `db:"description"`
	LastFetched   *time.Time `db:"last_fetched"`
	NextFetch     *time.Time `db:"next_fetch"`
	FetchInterval int        `db:"fetch_interval"`
	ErrorCount    int        `db:"error_count"`
	LastError     string     `db:"last_error"`
	Enabled       bool       `db:"enabled"`
	CreatedAt     time.Time  `db:"created_at"`
}

// NewFeedRepository creates a new feed repository
func NewFeedRepository(database *sqlx.DB) *FeedRepository {
	return &FeedRepository{db: database}
}

// CreateFeed inserts a new feed
func (r *FeedRepository) CreateFeed(ctx context.Context, feed *domain.Feed) error {
	sqlFeed := &feedSQL{
		URL:           feed.URL,
		Title:         feed.Title,
		Description:   feed.Description,
		FetchInterval: int(feed.FetchInterval.Seconds()),
		Enabled:       feed.Enabled,
	}

	query := `
		INSERT INTO feeds (url, title, description, fetch_interval, enabled)
		VALUES (:url, :title, :description, :fetch_interval, :enabled)
	`
	result, err := r.db.NamedExecContext(ctx, query, sqlFeed)
	if err != nil {
		return fmt.Errorf("create feed: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get insert id: %w", err)
	}

	feed.ID = id
	return nil
}

// GetFeed retrieves a feed by ID
func (r *FeedRepository) GetFeed(ctx context.Context, id int64) (*domain.Feed, error) {
	var sqlFeed feedSQL
	err := r.db.GetContext(ctx, &sqlFeed, "SELECT * FROM feeds WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("get feed: %w", err)
	}
	return r.toDomainFeed(&sqlFeed), nil
}

// GetFeeds retrieves feeds with optional filtering
func (r *FeedRepository) GetFeeds(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
	query := "SELECT * FROM feeds"
	if enabledOnly {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY title"

	var sqlFeeds []feedSQL
	err := r.db.SelectContext(ctx, &sqlFeeds, query)
	if err != nil {
		return nil, fmt.Errorf("get feeds: %w", err)
	}

	feeds := make([]domain.Feed, len(sqlFeeds))
	for i, f := range sqlFeeds {
		feeds[i] = *r.toDomainFeed(&f)
	}
	return feeds, nil
}

// GetFeedsToFetch retrieves feeds that need updating
func (r *FeedRepository) GetFeedsToFetch(ctx context.Context, limit int) ([]domain.Feed, error) {
	query := `
		SELECT * FROM feeds 
		WHERE enabled = 1 
		AND (next_fetch IS NULL OR next_fetch <= datetime('now'))
		ORDER BY next_fetch ASC
		LIMIT ?
	`
	var sqlFeeds []feedSQL
	err := r.db.SelectContext(ctx, &sqlFeeds, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get feeds to fetch: %w", err)
	}

	feeds := make([]domain.Feed, len(sqlFeeds))
	for i, f := range sqlFeeds {
		feeds[i] = *r.toDomainFeed(&f)
	}
	return feeds, nil
}

// UpdateFeedFetched updates feed after successful fetch
func (r *FeedRepository) UpdateFeedFetched(ctx context.Context, feedID int64, nextFetch time.Time) error {
	retrier := repeater.NewBackoff(5, 50*time.Millisecond, repeater.WithMaxDelay(2*time.Second))

	return retrier.Do(ctx, func() error {
		query := `
			UPDATE feeds 
			SET last_fetched = datetime('now'), 
			    next_fetch = ?,
			    error_count = 0,
			    last_error = ''
			WHERE id = ?
		`
		_, err := r.db.ExecContext(ctx, query, nextFetch, feedID)
		if err != nil {
			if isLockError(err) {
				return err // retry
			}
			return &criticalError{err: fmt.Errorf("update feed fetched: %w", err)}
		}
		return nil
	})
}

// UpdateFeedError updates feed after fetch error with exponential backoff
func (r *FeedRepository) UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error {
	retrier := repeater.NewBackoff(5, 50*time.Millisecond, repeater.WithMaxDelay(2*time.Second))

	return retrier.Do(ctx, func() error {
		// simple exponential backoff: 5, 10, 20, 40, 80... minutes, capped at 24 hours
		query := `
			UPDATE feeds 
			SET error_count = error_count + 1,
			    last_error = ?,
			    next_fetch = CASE 
			        WHEN error_count >= 7 THEN datetime('now', '+1440 minutes')  -- 24 hours max
			        WHEN error_count = 6 THEN datetime('now', '+640 minutes')   -- ~10.7 hours
			        WHEN error_count = 5 THEN datetime('now', '+320 minutes')   -- ~5.3 hours
			        WHEN error_count = 4 THEN datetime('now', '+160 minutes')   -- ~2.7 hours
			        WHEN error_count = 3 THEN datetime('now', '+80 minutes')    -- ~1.3 hours
			        WHEN error_count = 2 THEN datetime('now', '+40 minutes')    
			        WHEN error_count = 1 THEN datetime('now', '+20 minutes')
			        WHEN error_count = 0 THEN datetime('now', '+10 minutes')
			        ELSE datetime('now', '+5 minutes')
			    END
			WHERE id = ?
		`
		_, err := r.db.ExecContext(ctx, query, errMsg, feedID)
		if err != nil {
			if isLockError(err) {
				return err // retry
			}
			return &criticalError{err: fmt.Errorf("update feed error: %w", err)}
		}
		return nil
	})
}

// UpdateFeedStatus enables or disables a feed
func (r *FeedRepository) UpdateFeedStatus(ctx context.Context, feedID int64, enabled bool) error {
	query := "UPDATE feeds SET enabled = ? WHERE id = ?"
	_, err := r.db.ExecContext(ctx, query, enabled, feedID)
	if err != nil {
		return fmt.Errorf("update feed status: %w", err)
	}
	return nil
}

// UpdateFeed updates feed title and interval
func (r *FeedRepository) UpdateFeed(ctx context.Context, feedID int64, title string, fetchInterval time.Duration) error {
	query := "UPDATE feeds SET title = ?, fetch_interval = ? WHERE id = ?"
	_, err := r.db.ExecContext(ctx, query, title, int(fetchInterval.Seconds()), feedID)
	if err != nil {
		return fmt.Errorf("update feed: %w", err)
	}
	return nil
}

// DeleteFeed removes a feed and all its items
func (r *FeedRepository) DeleteFeed(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM feeds WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	return nil
}

// GetActiveFeedNames returns distinct feed names for feeds that have classified articles
func (r *FeedRepository) GetActiveFeedNames(ctx context.Context, minScore float64) ([]string, error) {
	query := `
		SELECT DISTINCT 
			CASE 
				WHEN f.title != '' THEN f.title
				ELSE REPLACE(REPLACE(SUBSTR(f.url, INSTR(f.url, '://') + 3), 'www.', ''), '/', '')
			END as feed_name
		FROM items i
		JOIN feeds f ON i.feed_id = f.id
		WHERE i.relevance_score >= ?
		AND i.classified_at IS NOT NULL
		ORDER BY feed_name
	`

	var feedNames []string
	if err := r.db.SelectContext(ctx, &feedNames, query, minScore); err != nil {
		return nil, fmt.Errorf("get active feed names: %w", err)
	}
	return feedNames, nil
}

// toDomainFeed converts feedSQL to domain.Feed
func (r *FeedRepository) toDomainFeed(sqlFeed *feedSQL) *domain.Feed {
	return &domain.Feed{
		ID:            sqlFeed.ID,
		URL:           sqlFeed.URL,
		Title:         sqlFeed.Title,
		Description:   sqlFeed.Description,
		LastFetched:   sqlFeed.LastFetched,
		NextFetch:     sqlFeed.NextFetch,
		FetchInterval: time.Duration(sqlFeed.FetchInterval) * time.Second,
		ErrorCount:    sqlFeed.ErrorCount,
		LastError:     sqlFeed.LastError,
		Enabled:       sqlFeed.Enabled,
		CreatedAt:     sqlFeed.CreatedAt,
	}
}
