package service

import (
	"context"
	"time"

	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/repository"
)

// SchedulerService provides unified access to repositories for the scheduler
type SchedulerService struct {
	feedRepo           *repository.FeedRepository
	itemRepo           *repository.ItemRepository
	classificationRepo *repository.ClassificationRepository
	settingRepo        *repository.SettingRepository
}

// NewSchedulerService creates a new scheduler service
func NewSchedulerService(feedRepo *repository.FeedRepository, itemRepo *repository.ItemRepository, classificationRepo *repository.ClassificationRepository, settingRepo *repository.SettingRepository) *SchedulerService {
	return &SchedulerService{
		feedRepo:           feedRepo,
		itemRepo:           itemRepo,
		classificationRepo: classificationRepo,
		settingRepo:        settingRepo,
	}
}

// Feed management methods

func (s *SchedulerService) GetFeed(ctx context.Context, id int64) (*domain.Feed, error) {
	return s.feedRepo.GetFeed(ctx, id)
}

func (s *SchedulerService) GetFeeds(ctx context.Context, enabledOnly bool) ([]*domain.Feed, error) {
	return s.feedRepo.GetFeeds(ctx, enabledOnly)
}

func (s *SchedulerService) UpdateFeedFetched(ctx context.Context, feedID int64, nextFetch time.Time) error {
	return s.feedRepo.UpdateFeedFetched(ctx, feedID, nextFetch)
}

func (s *SchedulerService) UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error {
	return s.feedRepo.UpdateFeedError(ctx, feedID, errMsg)
}

// Item processing methods

func (s *SchedulerService) GetItem(ctx context.Context, id int64) (*domain.Item, error) {
	return s.itemRepo.GetItem(ctx, id)
}

func (s *SchedulerService) CreateItem(ctx context.Context, item *domain.Item) error {
	return s.itemRepo.CreateItem(ctx, item)
}

func (s *SchedulerService) ItemExists(ctx context.Context, feedID int64, guid string) (bool, error) {
	return s.itemRepo.ItemExists(ctx, feedID, guid)
}

func (s *SchedulerService) ItemExistsByTitleOrURL(ctx context.Context, title, url string) (bool, error) {
	return s.itemRepo.ItemExistsByTitleOrURL(ctx, title, url)
}

func (s *SchedulerService) UpdateItemProcessed(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error {
	return s.itemRepo.UpdateItemProcessed(ctx, itemID, extraction, classification)
}

func (s *SchedulerService) UpdateItemExtraction(ctx context.Context, itemID int64, extraction *domain.ExtractedContent) error {
	return s.itemRepo.UpdateItemExtraction(ctx, itemID, extraction)
}

// Feedback methods

func (s *SchedulerService) GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]*domain.FeedbackExample, error) {
	return s.classificationRepo.GetRecentFeedback(ctx, feedbackType, limit)
}

func (s *SchedulerService) GetTopics(ctx context.Context) ([]string, error) {
	return s.classificationRepo.GetTopics(ctx)
}

func (s *SchedulerService) GetFeedbackCount(ctx context.Context) (int64, error) {
	return s.classificationRepo.GetFeedbackCount(ctx)
}

func (s *SchedulerService) GetFeedbackSince(ctx context.Context, offset int64, limit int) ([]*domain.FeedbackExample, error) {
	return s.classificationRepo.GetFeedbackSince(ctx, offset, limit)
}

// Setting methods

func (s *SchedulerService) GetSetting(ctx context.Context, key string) (string, error) {
	return s.settingRepo.GetSetting(ctx, key)
}

func (s *SchedulerService) SetSetting(ctx context.Context, key, value string) error {
	return s.settingRepo.SetSetting(ctx, key, value)
}