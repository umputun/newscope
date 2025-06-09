package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/repository"
	"github.com/umputun/newscope/server/mocks"
)

func TestNewRepositoryAdapterWithInterfaces(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}
	settingRepo := &mocks.SettingRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, settingRepo)

	assert.NotNil(t, adapter)
	assert.Equal(t, feedRepo, adapter.feedRepo)
	assert.Equal(t, itemRepo, adapter.itemRepo)
	assert.Equal(t, classificationRepo, adapter.classificationRepo)
	assert.Equal(t, settingRepo, adapter.settingRepo)
}

func TestRepositoryAdapter_GetClassifiedItemsWithFilters_Pagination(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, &mocks.SettingRepoMock{})

	now := time.Now()
	classifiedAt := now.Add(-1 * time.Hour)

	mockItem := &domain.ClassifiedItem{
		Item: &domain.Item{
			ID:          1,
			FeedID:      10,
			GUID:        "item-1",
			Title:       "Test Article",
			Link:        "https://example.com/article1",
			Description: "Test description",
			Published:   now,
		},
		FeedName: "Tech News",
		FeedURL:  "https://technews.com/feed",
		Classification: &domain.Classification{
			Score:        8.5,
			Explanation:  "test explanation",
			Topics:       []string{"tech", "news"},
			ClassifiedAt: classifiedAt,
		},
	}

	tests := []struct {
		name           string
		req            domain.ArticlesRequest
		expectedOffset int
		expectedLimit  int
	}{
		{
			name: "first page",
			req: domain.ArticlesRequest{
				Page:     1,
				Limit:    10,
				MinScore: 5.0,
				SortBy:   "published",
			},
			expectedOffset: 0,
			expectedLimit:  10,
		},
		{
			name: "second page",
			req: domain.ArticlesRequest{
				Page:     2,
				Limit:    10,
				MinScore: 5.0,
				SortBy:   "published",
			},
			expectedOffset: 10,
			expectedLimit:  10,
		},
		{
			name: "third page with different limit",
			req: domain.ArticlesRequest{
				Page:     3,
				Limit:    25,
				MinScore: 7.0,
				SortBy:   "score",
			},
			expectedOffset: 50,
			expectedLimit:  25,
		},
		{
			name: "page zero defaults to first page",
			req: domain.ArticlesRequest{
				Page:     0,
				Limit:    15,
				MinScore: 6.0,
			},
			expectedOffset: 0,
			expectedLimit:  15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedFilter *domain.ItemFilter

			classificationRepo.GetClassifiedItemsFunc = func(ctx context.Context, filter *domain.ItemFilter) ([]*domain.ClassifiedItem, error) {
				capturedFilter = filter
				return []*domain.ClassifiedItem{mockItem}, nil
			}

			items, err := adapter.GetClassifiedItemsWithFilters(context.Background(), tt.req)

			require.NoError(t, err)
			require.NotNil(t, capturedFilter)
			assert.Equal(t, tt.expectedOffset, capturedFilter.Offset)
			assert.Equal(t, tt.expectedLimit, capturedFilter.Limit)
			assert.InDelta(t, tt.req.MinScore, capturedFilter.MinScore, 0.01)
			assert.Equal(t, tt.req.Topic, capturedFilter.Topic)
			assert.Equal(t, tt.req.FeedName, capturedFilter.FeedName)
			assert.Equal(t, tt.req.SortBy, capturedFilter.SortBy)

			// verify transformation
			require.Len(t, items, 1)
			assert.Equal(t, int64(1), items[0].ID)
			assert.Equal(t, "Tech News", items[0].FeedName)
			assert.InDelta(t, 8.5, items[0].RelevanceScore, 0.01)
		})
	}
}

func TestRepositoryAdapter_GetClassifiedItemsWithFilters_DomainTransformation(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, &mocks.SettingRepoMock{})

	now := time.Now()
	classifiedAt := now.Add(-1 * time.Hour)

	tests := []struct {
		name           string
		classifiedItem *domain.ClassifiedItem
		expectedResult domain.ItemWithClassification
	}{
		{
			name: "complete item with all fields",
			classifiedItem: &domain.ClassifiedItem{
				Item: &domain.Item{
					ID:          1,
					FeedID:      10,
					GUID:        "item-1",
					Title:       "Test Article",
					Link:        "https://example.com/article",
					Description: "Test description",
					Content:     "Test content",
					Author:      "Test Author",
					Published:   now,
				},
				FeedName: "Test Feed",
				FeedURL:  "https://example.com/feed",
				Classification: &domain.Classification{
					Score:        7.5,
					Explanation:  "Relevant article",
					Topics:       []string{"tech", "ai"},
					ClassifiedAt: classifiedAt,
				},
				Extraction: &domain.ExtractedContent{
					PlainText: "Extracted plain text",
					RichHTML:  "<p>Extracted rich content</p>",
					Error:     "",
				},
				UserFeedback: &domain.Feedback{
					Type: domain.FeedbackLike,
				},
			},
			expectedResult: domain.ItemWithClassification{
				ID:                   1,
				FeedID:               10,
				FeedName:             "Test Feed",
				GUID:                 "item-1",
				Title:                "Test Article",
				Link:                 "https://example.com/article",
				Description:          "Test description",
				Content:              "Test content",
				Author:               "Test Author",
				Published:            now,
				RelevanceScore:       7.5,
				Explanation:          "Relevant article",
				Topics:               []string{"tech", "ai"},
				ClassifiedAt:         &classifiedAt,
				ExtractedContent:     "Extracted plain text",
				ExtractedRichContent: "<p>Extracted rich content</p>",
				ExtractionError:      "",
				UserFeedback:         "like",
			},
		},
		{
			name: "item with nil classification",
			classifiedItem: &domain.ClassifiedItem{
				Item: &domain.Item{
					ID:        2,
					FeedID:    20,
					GUID:      "item-2",
					Title:     "Another Article",
					Published: now,
				},
				FeedName:       "Another Feed",
				Classification: nil,
				Extraction:     nil,
				UserFeedback:   nil,
			},
			expectedResult: domain.ItemWithClassification{
				ID:             2,
				FeedID:         20,
				FeedName:       "Another Feed",
				GUID:           "item-2",
				Title:          "Another Article",
				Published:      now,
				RelevanceScore: 0,
				Explanation:    "",
				Topics:         nil,
				ClassifiedAt:   nil,
			},
		},
		{
			name: "item with extraction error",
			classifiedItem: &domain.ClassifiedItem{
				Item: &domain.Item{
					ID:        3,
					FeedID:    30,
					GUID:      "item-3",
					Title:     "Error Article",
					Published: now,
				},
				FeedName: "Error Feed",
				Extraction: &domain.ExtractedContent{
					PlainText: "",
					RichHTML:  "",
					Error:     "extraction failed",
				},
			},
			expectedResult: domain.ItemWithClassification{
				ID:                   3,
				FeedID:               30,
				FeedName:             "Error Feed",
				GUID:                 "item-3",
				Title:                "Error Article",
				Published:            now,
				ExtractedContent:     "",
				ExtractedRichContent: "",
				ExtractionError:      "extraction failed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classificationRepo.GetClassifiedItemsFunc = func(ctx context.Context, filter *domain.ItemFilter) ([]*domain.ClassifiedItem, error) {
				return []*domain.ClassifiedItem{tt.classifiedItem}, nil
			}

			req := domain.ArticlesRequest{
				Page:     1,
				Limit:    10,
				MinScore: 0,
			}

			items, err := adapter.GetClassifiedItemsWithFilters(context.Background(), req)

			require.NoError(t, err)
			require.Len(t, items, 1)
			assert.Equal(t, tt.expectedResult, items[0])
		})
	}
}

func TestGetFeedDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		feedURL  string
		expected string
	}{
		{
			name:     "title provided",
			title:    "Tech News Daily",
			feedURL:  "https://technews.com/feed.xml",
			expected: "Tech News Daily",
		},
		{
			name:     "no title, extract from URL",
			title:    "",
			feedURL:  "https://www.example.com/rss",
			expected: "example.com",
		},
		{
			name:     "no title, URL without www",
			title:    "",
			feedURL:  "https://hackernews.com/feed",
			expected: "hackernews.com",
		},
		{
			name:     "no title, URL with subdomain",
			title:    "",
			feedURL:  "https://blog.github.com/feed.atom",
			expected: "blog.github.com",
		},
		{
			name:     "no title, invalid URL",
			title:    "",
			feedURL:  "not-a-url",
			expected: "not-a-url",
		},
		{
			name:     "empty title and URL",
			title:    "",
			feedURL:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFeedDisplayName(tt.title, tt.feedURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRepositoryAdapter_GetClassifiedItemsCount(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, &mocks.SettingRepoMock{})

	t.Run("successful count", func(t *testing.T) {
		classificationRepo.GetClassifiedItemsCountFunc = func(ctx context.Context, filter *domain.ItemFilter) (int, error) {
			return 42, nil
		}

		req := domain.ArticlesRequest{
			MinScore: 5.0,
			Topic:    "tech",
			FeedName: "TechFeed",
			Limit:    10,
		}

		count, err := adapter.GetClassifiedItemsCount(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, 42, count)
	})

	t.Run("repository error", func(t *testing.T) {
		classificationRepo.GetClassifiedItemsCountFunc = func(ctx context.Context, filter *domain.ItemFilter) (int, error) {
			return 0, errors.New("database error")
		}

		req := domain.ArticlesRequest{MinScore: 5.0}

		count, err := adapter.GetClassifiedItemsCount(context.Background(), req)

		require.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "database error")
	})
}

func TestRepositoryAdapter_UpdateItemFeedback(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, &mocks.SettingRepoMock{})

	t.Run("successful feedback update", func(t *testing.T) {
		classificationRepo.UpdateItemFeedbackFunc = func(ctx context.Context, itemID int64, feedback *domain.Feedback) error {
			return nil
		}

		err := adapter.UpdateItemFeedback(context.Background(), 123, "positive")

		assert.NoError(t, err)
	})

	t.Run("repository error", func(t *testing.T) {
		classificationRepo.UpdateItemFeedbackFunc = func(ctx context.Context, itemID int64, feedback *domain.Feedback) error {
			return errors.New("update failed")
		}

		err := adapter.UpdateItemFeedback(context.Background(), 123, "negative")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "update failed")
	})

	t.Run("feedback type conversion", func(t *testing.T) {
		var capturedFeedback *domain.Feedback

		classificationRepo.UpdateItemFeedbackFunc = func(ctx context.Context, itemID int64, feedback *domain.Feedback) error {
			capturedFeedback = feedback
			return nil
		}

		err := adapter.UpdateItemFeedback(context.Background(), 456, "negative")

		require.NoError(t, err)
		require.NotNil(t, capturedFeedback)
		assert.Equal(t, domain.FeedbackType("negative"), capturedFeedback.Type)
	})
}

func TestRepositoryAdapter_GetClassifiedItem(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, &mocks.SettingRepoMock{})

	now := time.Now()
	classifiedAt := now.Add(-2 * time.Hour)

	t.Run("successful get single item", func(t *testing.T) {
		mockItem := &domain.ClassifiedItem{
			Item: &domain.Item{
				ID:        789,
				FeedID:    100,
				GUID:      "single-item",
				Title:     "Single Article",
				Link:      "https://example.com/single",
				Published: now,
			},
			FeedName: "",
			FeedURL:  "https://example.com/feed",
			Classification: &domain.Classification{
				Score:        9.0,
				Explanation:  "Highly relevant",
				Topics:       []string{"important"},
				ClassifiedAt: classifiedAt,
			},
		}

		classificationRepo.GetClassifiedItemFunc = func(ctx context.Context, itemID int64) (*domain.ClassifiedItem, error) {
			return mockItem, nil
		}

		item, err := adapter.GetClassifiedItem(context.Background(), 789)

		require.NoError(t, err)
		require.NotNil(t, item)
		assert.Equal(t, int64(789), item.ID)
		assert.Equal(t, "example.com", item.FeedName) // URL hostname extraction
		assert.InDelta(t, 9.0, item.RelevanceScore, 0.01)
		assert.Equal(t, &classifiedAt, item.ClassifiedAt)
	})

	t.Run("repository error", func(t *testing.T) {
		classificationRepo.GetClassifiedItemFunc = func(ctx context.Context, itemID int64) (*domain.ClassifiedItem, error) {
			return nil, errors.New("item not found")
		}

		item, err := adapter.GetClassifiedItem(context.Background(), 999)

		require.Error(t, err)
		assert.Nil(t, item)
		assert.Contains(t, err.Error(), "item not found")
	})
}

func TestRepositoryAdapter_ErrorPropagation(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, &mocks.SettingRepoMock{})

	testError := errors.New("repository error")

	t.Run("GetFeeds error", func(t *testing.T) {
		feedRepo.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
			return nil, testError
		}

		feeds, err := adapter.GetFeeds(context.Background())
		require.Error(t, err)
		assert.Nil(t, feeds)
	})

	t.Run("GetItems error", func(t *testing.T) {
		itemRepo.GetItemsFunc = func(ctx context.Context, limit int, minScore float64) ([]domain.Item, error) {
			return nil, testError
		}

		items, err := adapter.GetItems(context.Background(), 10, 0)
		require.Error(t, err)
		assert.Nil(t, items)
	})

	t.Run("GetClassifiedItemsWithFilters error", func(t *testing.T) {
		classificationRepo.GetClassifiedItemsFunc = func(ctx context.Context, filter *domain.ItemFilter) ([]*domain.ClassifiedItem, error) {
			return nil, testError
		}

		req := domain.ArticlesRequest{Limit: 10}
		items, err := adapter.GetClassifiedItemsWithFilters(context.Background(), req)
		require.Error(t, err)
		assert.Nil(t, items)
	})

	t.Run("GetTopics error", func(t *testing.T) {
		classificationRepo.GetTopicsFunc = func(ctx context.Context) ([]string, error) {
			return nil, testError
		}

		topics, err := adapter.GetTopics(context.Background())
		require.Error(t, err)
		assert.Nil(t, topics)
	})

	t.Run("GetTopicsFiltered error", func(t *testing.T) {
		classificationRepo.GetTopicsFilteredFunc = func(ctx context.Context, minScore float64) ([]string, error) {
			return nil, testError
		}

		topics, err := adapter.GetTopicsFiltered(context.Background(), 5.0)
		require.Error(t, err)
		assert.Nil(t, topics)
	})
}

func TestRepositoryAdapter_FeedOperations(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, &mocks.SettingRepoMock{})

	t.Run("GetAllFeeds", func(t *testing.T) {
		expectedFeeds := []domain.Feed{
			{ID: 1, Title: "Feed 1", URL: "https://feed1.com"},
			{ID: 2, Title: "Feed 2", URL: "https://feed2.com"},
		}

		feedRepo.GetFeedsFunc = func(ctx context.Context, enabledOnly bool) ([]domain.Feed, error) {
			return expectedFeeds, nil
		}

		feeds, err := adapter.GetAllFeeds(context.Background())

		require.NoError(t, err)
		assert.Equal(t, expectedFeeds, feeds)
	})

	t.Run("CreateFeed", func(t *testing.T) {
		feedRepo.CreateFeedFunc = func(ctx context.Context, feed *domain.Feed) error {
			return nil
		}

		feed := &domain.Feed{Title: "New Feed", URL: "https://newfeed.com"}

		err := adapter.CreateFeed(context.Background(), feed)

		assert.NoError(t, err)
	})

	t.Run("UpdateFeed", func(t *testing.T) {
		feedRepo.UpdateFeedFunc = func(ctx context.Context, feedID int64, title string, fetchInterval time.Duration) error {
			assert.Equal(t, int64(123), feedID)
			assert.Equal(t, "New Title", title)
			assert.Equal(t, 3600*time.Second, fetchInterval)
			return nil
		}

		err := adapter.UpdateFeed(context.Background(), 123, "New Title", 3600*time.Second)
		require.NoError(t, err)
		assert.Len(t, feedRepo.UpdateFeedCalls(), 1)
	})

	t.Run("UpdateFeedStatus", func(t *testing.T) {
		feedRepo.UpdateFeedStatusFunc = func(ctx context.Context, feedID int64, enabled bool) error {
			return nil
		}

		err := adapter.UpdateFeedStatus(context.Background(), 123, true)
		assert.NoError(t, err)
	})

	t.Run("DeleteFeed", func(t *testing.T) {
		feedRepo.DeleteFeedFunc = func(ctx context.Context, feedID int64) error {
			return nil
		}

		err := adapter.DeleteFeed(context.Background(), 456)
		assert.NoError(t, err)
	})

	t.Run("GetActiveFeedNames", func(t *testing.T) {
		expectedNames := []string{"Active Feed 1", "Active Feed 2"}

		feedRepo.GetActiveFeedNamesFunc = func(ctx context.Context, minScore float64) ([]string, error) {
			return expectedNames, nil
		}

		names, err := adapter.GetActiveFeedNames(context.Background(), 5.0)

		require.NoError(t, err)
		assert.Equal(t, expectedNames, names)
	})
}

func TestRepositoryAdapter_Settings(t *testing.T) {
	feedRepo := &mocks.FeedRepoMock{}
	itemRepo := &mocks.ItemRepoMock{}
	classificationRepo := &mocks.ClassificationRepoMock{}
	settingRepo := &mocks.SettingRepoMock{}

	adapter := NewRepositoryAdapterWithInterfaces(feedRepo, itemRepo, classificationRepo, settingRepo)

	t.Run("GetSetting", func(t *testing.T) {
		settingRepo.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
			assert.Equal(t, "test_key", key)
			return "test_value", nil
		}

		value, err := adapter.GetSetting(context.Background(), "test_key")
		require.NoError(t, err)
		assert.Equal(t, "test_value", value)
		assert.Len(t, settingRepo.GetSettingCalls(), 1)
	})

	t.Run("SetSetting", func(t *testing.T) {
		settingRepo.SetSettingFunc = func(ctx context.Context, key, value string) error {
			assert.Equal(t, "test_key", key)
			assert.Equal(t, "test_value", value)
			return nil
		}

		err := adapter.SetSetting(context.Background(), "test_key", "test_value")
		require.NoError(t, err)
		assert.Len(t, settingRepo.SetSettingCalls(), 1)
	})

	t.Run("GetSetting error", func(t *testing.T) {
		testError := errors.New("setting error")
		settingRepo.GetSettingFunc = func(ctx context.Context, key string) (string, error) {
			return "", testError
		}

		value, err := adapter.GetSetting(context.Background(), "test_key")
		require.Error(t, err)
		assert.Equal(t, testError, err)
		assert.Empty(t, value)
	})

	t.Run("SetSetting error", func(t *testing.T) {
		testError := errors.New("setting error")
		settingRepo.SetSettingFunc = func(ctx context.Context, key, value string) error {
			return testError
		}

		err := adapter.SetSetting(context.Background(), "test_key", "test_value")
		require.Error(t, err)
		assert.Equal(t, testError, err)
	})
}

func TestRepositoryAdapter_GetClassifiedItems(t *testing.T) {
	// GetClassifiedItems simply calls GetClassifiedItemsWithFilters, which is already tested
	// We just need to verify that it creates the correct request structure

	classificationRepo := &mocks.ClassificationRepoMock{
		GetClassifiedItemsFunc: func(ctx context.Context, filter *domain.ItemFilter) ([]*domain.ClassifiedItem, error) {
			// verify filter structure created by GetClassifiedItems
			assert.InDelta(t, 7.5, filter.MinScore, 0.01)
			assert.Equal(t, "technology", filter.Topic)
			assert.Empty(t, filter.FeedName)
			assert.Equal(t, "published", filter.SortBy)
			assert.Equal(t, 50, filter.Limit)
			assert.Equal(t, 0, filter.Offset)
			return []*domain.ClassifiedItem{
				{
					Item: &domain.Item{
						Title: "Test",
					},
				},
			}, nil
		},
	}

	adapter := NewRepositoryAdapterWithInterfaces(nil, nil, classificationRepo, nil)

	items, err := adapter.GetClassifiedItems(context.Background(), 7.5, "technology", 50)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "Test", items[0].Title)
}

func TestRepositoryAdapter_GetTopTopicsByScore(t *testing.T) {
	classificationRepo := &mocks.ClassificationRepoMock{}
	adapter := NewRepositoryAdapterWithInterfaces(nil, nil, classificationRepo, nil)

	t.Run("successful get top topics", func(t *testing.T) {
		repoTopics := []repository.TopicWithScore{
			{Topic: "technology", AvgScore: 8.5, ItemCount: 150},
			{Topic: "science", AvgScore: 8.2, ItemCount: 120},
			{Topic: "business", AvgScore: 7.8, ItemCount: 90},
		}

		classificationRepo.GetTopTopicsByScoreFunc = func(ctx context.Context, minScore float64, limit int) ([]repository.TopicWithScore, error) {
			assert.InDelta(t, 5.0, minScore, 0.01)
			assert.Equal(t, 10, limit)
			return repoTopics, nil
		}

		topics, err := adapter.GetTopTopicsByScore(context.Background(), 5.0, 10)
		require.NoError(t, err)
		require.Len(t, topics, 3)

		// verify conversion from repository type to domain type
		assert.Equal(t, "technology", topics[0].Topic)
		assert.InDelta(t, 8.5, topics[0].AvgScore, 0.01)
		assert.Equal(t, 150, topics[0].ItemCount)

		assert.Equal(t, "science", topics[1].Topic)
		assert.InDelta(t, 8.2, topics[1].AvgScore, 0.01)
		assert.Equal(t, 120, topics[1].ItemCount)

		assert.Equal(t, "business", topics[2].Topic)
		assert.InDelta(t, 7.8, topics[2].AvgScore, 0.01)
		assert.Equal(t, 90, topics[2].ItemCount)

		assert.Len(t, classificationRepo.GetTopTopicsByScoreCalls(), 1)
	})

	t.Run("empty result", func(t *testing.T) {
		classificationRepo.GetTopTopicsByScoreFunc = func(ctx context.Context, minScore float64, limit int) ([]repository.TopicWithScore, error) {
			return []repository.TopicWithScore{}, nil
		}

		topics, err := adapter.GetTopTopicsByScore(context.Background(), 8.0, 5)
		require.NoError(t, err)
		assert.Empty(t, topics)
	})

	t.Run("error handling", func(t *testing.T) {
		testError := errors.New("database error")
		classificationRepo.GetTopTopicsByScoreFunc = func(ctx context.Context, minScore float64, limit int) ([]repository.TopicWithScore, error) {
			return nil, testError
		}

		topics, err := adapter.GetTopTopicsByScore(context.Background(), 5.0, 10)
		require.Error(t, err)
		assert.Equal(t, testError, err)
		assert.Nil(t, topics)
	})
}

func TestRepositoryAdapter_GetFeedbackCount(t *testing.T) {
	classificationRepo := &mocks.ClassificationRepoMock{}
	adapter := NewRepositoryAdapterWithInterfaces(nil, nil, classificationRepo, nil)

	t.Run("successful get feedback count", func(t *testing.T) {
		classificationRepo.GetFeedbackCountFunc = func(ctx context.Context) (int64, error) {
			return 42, nil
		}

		count, err := adapter.GetFeedbackCount(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(42), count)
		assert.Len(t, classificationRepo.GetFeedbackCountCalls(), 1)
	})

	t.Run("zero count", func(t *testing.T) {
		classificationRepo.GetFeedbackCountFunc = func(ctx context.Context) (int64, error) {
			return 0, nil
		}

		count, err := adapter.GetFeedbackCount(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	t.Run("error handling", func(t *testing.T) {
		testError := errors.New("database error")
		classificationRepo.GetFeedbackCountFunc = func(ctx context.Context) (int64, error) {
			return 0, testError
		}

		count, err := adapter.GetFeedbackCount(context.Background())
		require.Error(t, err)
		assert.Equal(t, testError, err)
		assert.Equal(t, int64(0), count)
	})
}
