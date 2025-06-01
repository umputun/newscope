package server

import (
	"context"

	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/feed/types"
	"github.com/umputun/newscope/pkg/repository"
)

// RepositoryAdapter adapts repositories to server.Database interface
type RepositoryAdapter struct {
	repos *repository.Repositories
}

// NewRepositoryAdapter creates a new repository adapter
func NewRepositoryAdapter(repos *repository.Repositories) *RepositoryAdapter {
	return &RepositoryAdapter{repos: repos}
}

// GetFeeds adapts repository feeds to return types.Feed
func (r *RepositoryAdapter) GetFeeds(ctx context.Context) ([]types.Feed, error) {
	domainFeeds, err := r.repos.Feed.GetFeeds(ctx, false) // get all feeds
	if err != nil {
		return nil, err
	}

	feeds := make([]types.Feed, len(domainFeeds))
	for i, f := range domainFeeds {
		feeds[i] = types.Feed{
			Title:       f.Title,
			Description: f.Description,
			Link:        f.URL,
		}
	}
	return feeds, nil
}

// GetItems adapts repository items to return types.Item
func (r *RepositoryAdapter) GetItems(ctx context.Context, limit, _ int) ([]types.Item, error) {
	// Repository uses minScore instead of offset
	// for now, return all items with score >= 0
	domainItems, err := r.repos.Item.GetItems(ctx, limit, 0)
	if err != nil {
		return nil, err
	}

	items := make([]types.Item, len(domainItems))
	for i, item := range domainItems {
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
func (r *RepositoryAdapter) GetClassifiedItems(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error) {
	return r.GetClassifiedItemsWithFilters(ctx, minScore, topic, "", limit)
}

// GetClassifiedItemsWithFilters returns items with classification data filtered by topic and feed
func (r *RepositoryAdapter) GetClassifiedItemsWithFilters(ctx context.Context, minScore float64, topic, feedName string, limit int) ([]types.ItemWithClassification, error) {
	filter := &domain.ItemFilter{
		MinScore: minScore,
		Topic:    topic,
		FeedName: feedName,
		Limit:    limit,
	}

	// get items from repository
	items, err := r.repos.Classification.GetClassifiedItems(ctx, filter)
	if err != nil {
		return nil, err
	}

	// convert to types
	result := make([]types.ItemWithClassification, 0, len(items))
	for _, item := range items {
		feedDisplayName := getFeedDisplayName(item.FeedName, item.FeedURL)

		itemWithClass := types.ItemWithClassification{
			Item: types.Item{
				GUID:        item.GUID,
				Title:       item.Title,
				Link:        item.Link,
				Description: item.Description,
				Author:      item.Author,
				Published:   item.Published,
			},
			ID:       item.ID,
			FeedName: feedDisplayName,
		}

		// handle extraction data if available
		if item.Extraction != nil {
			itemWithClass.ExtractedContent = item.Extraction.PlainText
			itemWithClass.ExtractedRichContent = item.Extraction.RichHTML
			itemWithClass.ExtractionError = item.Extraction.Error
		}

		// handle classification data if available
		if item.Classification != nil {
			itemWithClass.RelevanceScore = item.Classification.Score
			itemWithClass.Explanation = item.Classification.Explanation
			itemWithClass.Topics = item.Classification.Topics
			itemWithClass.ClassifiedAt = &item.Classification.ClassifiedAt
		}

		// handle feedback if available
		if item.UserFeedback != nil {
			itemWithClass.UserFeedback = string(item.UserFeedback.Type)
		}

		result = append(result, itemWithClass)
	}

	return result, nil
}

// UpdateItemFeedback updates user feedback for an item
func (r *RepositoryAdapter) UpdateItemFeedback(ctx context.Context, itemID int64, feedback string) error {
	feedbackType := domain.FeedbackType(feedback)
	domainFeedback := &domain.Feedback{
		Type: feedbackType,
	}
	return r.repos.Classification.UpdateItemFeedback(ctx, itemID, domainFeedback)
}

// GetClassifiedItem returns a single item with classification data
func (r *RepositoryAdapter) GetClassifiedItem(ctx context.Context, itemID int64) (*types.ItemWithClassification, error) {
	item, err := r.repos.Classification.GetClassifiedItem(ctx, itemID)
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
		ID:       item.ID,
		FeedName: getFeedDisplayName(item.FeedName, item.FeedURL),
	}

	// handle extraction data if available
	if item.Extraction != nil {
		result.ExtractedContent = item.Extraction.PlainText
		result.ExtractedRichContent = item.Extraction.RichHTML
		result.ExtractionError = item.Extraction.Error
	}

	// handle classification data if available
	if item.Classification != nil {
		result.RelevanceScore = item.Classification.Score
		result.Explanation = item.Classification.Explanation
		result.Topics = item.Classification.Topics
		result.ClassifiedAt = &item.Classification.ClassifiedAt
	}

	// handle feedback if available
	if item.UserFeedback != nil {
		result.UserFeedback = string(item.UserFeedback.Type)
	}

	return result, nil
}

// GetTopics returns all unique topics from classified items
func (r *RepositoryAdapter) GetTopics(ctx context.Context) ([]string, error) {
	return r.repos.Classification.GetTopics(ctx)
}

// GetTopicsFiltered returns unique topics from items with score >= minScore
func (r *RepositoryAdapter) GetTopicsFiltered(ctx context.Context, minScore float64) ([]string, error) {
	return r.repos.Classification.GetTopicsFiltered(ctx, minScore)
}

// GetAllFeeds returns all feeds with full details
func (r *RepositoryAdapter) GetAllFeeds(ctx context.Context) ([]db.Feed, error) {
	domainFeeds, err := r.repos.Feed.GetFeeds(ctx, false) // get all feeds, not just enabled
	if err != nil {
		return nil, err
	}

	// convert domain feeds to db feeds
	feeds := make([]db.Feed, len(domainFeeds))
	for i, f := range domainFeeds {
		feeds[i] = db.Feed{
			ID:            f.ID,
			URL:           f.URL,
			Title:         f.Title,
			Description:   f.Description,
			LastFetched:   f.LastFetched,
			NextFetch:     f.NextFetch,
			FetchInterval: f.FetchInterval,
			ErrorCount:    f.ErrorCount,
			LastError:     f.LastError,
			Enabled:       f.Enabled,
			CreatedAt:     f.CreatedAt,
		}
	}
	return feeds, nil
}

// CreateFeed adds a new feed
func (r *RepositoryAdapter) CreateFeed(ctx context.Context, feed *db.Feed) error {
	domainFeed := &domain.Feed{
		URL:           feed.URL,
		Title:         feed.Title,
		Description:   feed.Description,
		FetchInterval: feed.FetchInterval,
		Enabled:       feed.Enabled,
	}
	
	err := r.repos.Feed.CreateFeed(ctx, domainFeed)
	if err != nil {
		return err
	}
	
	// update the original feed with the new ID
	feed.ID = domainFeed.ID
	return nil
}

// UpdateFeedStatus enables or disables a feed
func (r *RepositoryAdapter) UpdateFeedStatus(ctx context.Context, feedID int64, enabled bool) error {
	return r.repos.Feed.UpdateFeedStatus(ctx, feedID, enabled)
}

// DeleteFeed removes a feed
func (r *RepositoryAdapter) DeleteFeed(ctx context.Context, feedID int64) error {
	return r.repos.Feed.DeleteFeed(ctx, feedID)
}

// GetActiveFeedNames returns names of feeds that have classified articles
func (r *RepositoryAdapter) GetActiveFeedNames(ctx context.Context, minScore float64) ([]string, error) {
	return r.repos.Feed.GetActiveFeedNames(ctx, minScore)
}

