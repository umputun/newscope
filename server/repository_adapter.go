package server

import (
	"context"
	"net/url"
	"strings"

	"github.com/umputun/newscope/pkg/domain"
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

// GetFeeds returns all feeds from repository
func (r *RepositoryAdapter) GetFeeds(ctx context.Context) ([]domain.Feed, error) {
	feeds, err := r.repos.Feed.GetFeeds(ctx, false) // get all feeds
	if err != nil {
		return nil, err
	}

	return feeds, nil
}

// GetItems returns items from repository
func (r *RepositoryAdapter) GetItems(ctx context.Context, limit, _ int) ([]domain.Item, error) {
	// repository uses minScore instead of offset
	// for now, return all items with score >= 0
	items, err := r.repos.Item.GetItems(ctx, limit, 0)
	if err != nil {
		return nil, err
	}

	return items, nil
}

// GetClassifiedItems returns items with classification data
func (r *RepositoryAdapter) GetClassifiedItems(ctx context.Context, minScore float64, topic string, limit int) ([]domain.ItemWithClassification, error) {
	req := domain.ArticlesRequest{
		MinScore: minScore,
		Topic:    topic,
		FeedName: "",
		SortBy:   "published",
		Limit:    limit,
	}
	return r.GetClassifiedItemsWithFilters(ctx, req)
}

// GetClassifiedItemsWithFilters returns items with classification data filtered by topic and feed
func (r *RepositoryAdapter) GetClassifiedItemsWithFilters(ctx context.Context, req domain.ArticlesRequest) ([]domain.ItemWithClassification, error) {
	// calculate offset from page number
	offset := 0
	if req.Page > 1 {
		offset = (req.Page - 1) * req.Limit
	}

	filter := &domain.ItemFilter{
		MinScore: req.MinScore,
		Topic:    req.Topic,
		FeedName: req.FeedName,
		SortBy:   req.SortBy,
		Limit:    req.Limit,
		Offset:   offset,
	}

	// get items from repository
	items, err := r.repos.Classification.GetClassifiedItems(ctx, filter)
	if err != nil {
		return nil, err
	}

	// convert to ItemWithClassification
	result := make([]domain.ItemWithClassification, 0, len(items))
	for _, item := range items {
		feedDisplayName := getFeedDisplayName(item.FeedName, item.FeedURL)

		itemWithClass := domain.ItemWithClassification{
			ID:          item.ID,
			FeedID:      item.FeedID,
			FeedName:    feedDisplayName,
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Content:     item.Content,
			Author:      item.Author,
			Published:   item.Published,
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

// GetClassifiedItemsCount returns total count of classified items matching filters
func (r *RepositoryAdapter) GetClassifiedItemsCount(ctx context.Context, req domain.ArticlesRequest) (int, error) {
	filter := &domain.ItemFilter{
		MinScore: req.MinScore,
		Topic:    req.Topic,
		FeedName: req.FeedName,
		SortBy:   req.SortBy,
		Limit:    req.Limit,
	}

	return r.repos.Classification.GetClassifiedItemsCount(ctx, filter)
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
func (r *RepositoryAdapter) GetClassifiedItem(ctx context.Context, itemID int64) (*domain.ItemWithClassification, error) {
	item, err := r.repos.Classification.GetClassifiedItem(ctx, itemID)
	if err != nil {
		return nil, err
	}

	result := &domain.ItemWithClassification{
		ID:          item.ID,
		FeedID:      item.FeedID,
		FeedName:    getFeedDisplayName(item.FeedName, item.FeedURL),
		GUID:        item.GUID,
		Title:       item.Title,
		Link:        item.Link,
		Description: item.Description,
		Content:     item.Content,
		Author:      item.Author,
		Published:   item.Published,
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
func (r *RepositoryAdapter) GetAllFeeds(ctx context.Context) ([]domain.Feed, error) {
	domainFeeds, err := r.repos.Feed.GetFeeds(ctx, false) // get all feeds, not just enabled
	if err != nil {
		return nil, err
	}

	return domainFeeds, nil
}

// CreateFeed adds a new feed
func (r *RepositoryAdapter) CreateFeed(ctx context.Context, feed *domain.Feed) error {
	return r.repos.Feed.CreateFeed(ctx, feed)
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
