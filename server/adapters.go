package server

import (
	"context"
	"net/url"
	"strings"

	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed/types"
)

// ArticleFilter holds filtering parameters for articles
type ArticleFilter struct {
	MinScore float64
	Topic    string
	FeedName string
	Limit    int
}

// DBAdapter adapts db.DB to server.Database interface
type DBAdapter struct {
	*db.DB
}

// GetFeeds adapts db.GetFeeds to return types.Feed
func (d *DBAdapter) GetFeeds(ctx context.Context) ([]types.Feed, error) {
	dbFeeds, err := d.DB.GetFeeds(ctx, false) // get all feeds
	if err != nil {
		return nil, err
	}

	feeds := make([]types.Feed, len(dbFeeds))
	for i, f := range dbFeeds {
		feeds[i] = types.Feed{
			Title:       f.Title,
			Description: f.Description,
			Link:        f.URL,
		}
	}
	return feeds, nil
}

// GetItems adapts db.GetItems to return types.Item
func (d *DBAdapter) GetItems(ctx context.Context, limit, _ int) ([]types.Item, error) {
	// DB uses minScore instead of offset
	// for now, return all items with score >= 0
	dbItems, err := d.DB.GetItems(ctx, limit, 0)
	if err != nil {
		return nil, err
	}

	items := make([]types.Item, len(dbItems))
	for i, item := range dbItems {
		items[i] = types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Author:      item.Author,
			Published:   item.Published,
		}
	}
	return items, nil
}

// GetClassifiedItems returns items with classification data
func (d *DBAdapter) GetClassifiedItems(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error) {
	return d.GetClassifiedItemsFiltered(ctx, ArticleFilter{
		MinScore: minScore,
		Topic:    topic,
		Limit:    limit,
	})
}

// GetClassifiedItemsWithFilters returns items with classification data filtered by topic and feed
func (d *DBAdapter) GetClassifiedItemsWithFilters(ctx context.Context, minScore float64, topic, feedName string, limit int) ([]types.ItemWithClassification, error) {
	return d.GetClassifiedItemsFiltered(ctx, ArticleFilter{
		MinScore: minScore,
		Topic:    topic,
		FeedName: feedName,
		Limit:    limit,
	})
}

// GetClassifiedItemsFiltered returns items with classification data using filter struct
func (d *DBAdapter) GetClassifiedItemsFiltered(ctx context.Context, filter ArticleFilter) ([]types.ItemWithClassification, error) {
	// get items from DB
	items, err := d.DB.GetClassifiedItems(ctx, filter.MinScore, filter.Limit)
	if err != nil {
		return nil, err
	}

	// convert to types and filter by topic and feed if needed
	result := make([]types.ItemWithClassification, 0, len(items))
	for _, item := range items {
		// filter by topic if specified
		if filter.Topic != "" {
			found := false
			for _, t := range item.Topics {
				if t == filter.Topic {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		
		// filter by feed if specified
		feedDisplayName := getFeedDisplayName(item.FeedTitle, item.FeedURL)
		if filter.FeedName != "" && feedDisplayName != filter.FeedName {
			continue
		}

		result = append(result, types.ItemWithClassification{
			Item: types.Item{
				GUID:        item.GUID,
				Title:       item.Title,
				Link:        item.Link,
				Description: item.Description,
				Author:      item.Author,
				Published:   item.Published,
			},
			ID:                   item.ID,
			FeedName:             feedDisplayName,
			ExtractedContent:     item.ExtractedContent,
			ExtractedRichContent: item.ExtractedRichContent,
			ExtractionError:      item.ExtractionError,
			RelevanceScore:       item.RelevanceScore,
			Explanation:          item.Explanation,
			Topics:               []string(item.Topics),
			ClassifiedAt:         item.ClassifiedAt,
			UserFeedback:         item.UserFeedback,
		})
	}

	return result, nil
}

// UpdateItemFeedback updates user feedback for an item
func (d *DBAdapter) UpdateItemFeedback(ctx context.Context, itemID int64, feedback string) error {
	return d.DB.UpdateItemFeedback(ctx, itemID, feedback)
}

// GetClassifiedItem returns a single item with classification data
func (d *DBAdapter) GetClassifiedItem(ctx context.Context, itemID int64) (*types.ItemWithClassification, error) {
	item, err := d.DB.GetClassifiedItem(ctx, itemID)
	if err != nil {
		return nil, err
	}

	result := &types.ItemWithClassification{
		Item: types.Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Author:      item.Author,
			Published:   item.Published,
		},
		ID:                   item.ID,
		FeedName:             getFeedDisplayName(item.FeedTitle, item.FeedURL),
		ExtractedContent:     item.ExtractedContent,
		ExtractedRichContent: item.ExtractedRichContent,
		ExtractionError:      item.ExtractionError,
		RelevanceScore:       item.RelevanceScore,
		Explanation:          item.Explanation,
		Topics:               []string(item.Topics),
		ClassifiedAt:         item.ClassifiedAt,
		UserFeedback:         item.UserFeedback,
	}

	return result, nil
}

// GetTopics returns all unique topics from classified items
func (d *DBAdapter) GetTopics(ctx context.Context) ([]string, error) {
	return d.DB.GetTopics(ctx)
}

// GetTopicsFiltered returns unique topics from items with score >= minScore
func (d *DBAdapter) GetTopicsFiltered(ctx context.Context, minScore float64) ([]string, error) {
	return d.DB.GetTopicsFiltered(ctx, minScore)
}

// GetAllFeeds returns all feeds with full details
func (d *DBAdapter) GetAllFeeds(ctx context.Context) ([]db.Feed, error) {
	return d.DB.GetFeeds(ctx, false) // get all feeds, not just enabled
}

// CreateFeed adds a new feed
func (d *DBAdapter) CreateFeed(ctx context.Context, feed *db.Feed) error {
	return d.DB.CreateFeed(ctx, feed)
}

// UpdateFeedStatus enables or disables a feed
func (d *DBAdapter) UpdateFeedStatus(ctx context.Context, feedID int64, enabled bool) error {
	return d.DB.UpdateFeedStatus(ctx, feedID, enabled)
}

// DeleteFeed removes a feed
func (d *DBAdapter) DeleteFeed(ctx context.Context, feedID int64) error {
	return d.DB.DeleteFeed(ctx, feedID)
}

// GetActiveFeedNames returns names of feeds that have classified articles
func (d *DBAdapter) GetActiveFeedNames(ctx context.Context, minScore float64) ([]string, error) {
	items, err := d.DB.GetClassifiedItems(ctx, minScore, 100) // get enough items to find all feeds
	if err != nil {
		return nil, err
	}
	
	feedSet := make(map[string]bool)
	for _, item := range items {
		feedName := getFeedDisplayName(item.FeedTitle, item.FeedURL)
		feedSet[feedName] = true
	}
	
	feeds := make([]string, 0, len(feedSet))
	for feed := range feedSet {
		feeds = append(feeds, feed)
	}
	
	return feeds, nil
}

// getFeedDisplayName returns the feed title if available, otherwise extracts hostname from URL
func getFeedDisplayName(title, feedURL string) string {
	if title != "" {
		return title
	}

	// try to extract hostname from URL
	if u, err := url.Parse(feedURL); err == nil {
		host := u.Hostname()
		// remove www. prefix if present
		host = strings.TrimPrefix(host, "www.")
		return host
	}

	// fallback to the full URL
	return feedURL
}
