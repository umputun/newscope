package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/scheduler/mocks"
)

func TestPreferenceManager_UpdatePreferenceSummary(t *testing.T) {
	t.Run("generate initial summary", func(t *testing.T) {
		// setup mocks
		classificationManager := &mocks.ClassificationManagerMock{
			GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
				assert.Equal(t, 50, limit)
				return []domain.FeedbackExample{
					{Title: "AI article", Feedback: domain.FeedbackLike, Topics: []string{"ai"}},
					{Title: "Politics", Feedback: domain.FeedbackDislike, Topics: []string{"politics"}},
				}, nil
			},
			GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
				return 2, nil
			},
		}

		settingManager := &mocks.SettingManagerMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				// no existing preference summary
				return "", nil
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				return nil
			},
		}

		classifier := &mocks.ClassifierMock{
			GeneratePreferenceSummaryFunc: func(ctx context.Context, feedback []domain.FeedbackExample) (string, error) {
				assert.Len(t, feedback, 2)
				return "User likes AI, dislikes politics", nil
			},
		}

		retryFunc := func(ctx context.Context, op func() error) error {
			return op()
		}

		pm := NewPreferenceManager(PreferenceManagerConfig{
			ClassificationManager:      classificationManager,
			SettingManager:             settingManager,
			Classifier:                 classifier,
			PreferenceSummaryThreshold: 25,
			RetryFunc:                  retryFunc,
		})

		// execute
		err := pm.UpdatePreferenceSummary(context.Background())

		// verify
		require.NoError(t, err)
		assert.Len(t, classifier.GeneratePreferenceSummaryCalls(), 1)
		assert.Len(t, settingManager.SetSettingCalls(), 2) // summary and count

		// check both settings were updated
		var summarySet, countSet bool
		for _, call := range settingManager.SetSettingCalls() {
			if call.Key == "preference_summary" && call.Value == "User likes AI, dislikes politics" {
				summarySet = true
			}
			if call.Key == "last_summary_feedback_count" && call.Value == "2" {
				countSet = true
			}
		}
		assert.True(t, summarySet)
		assert.True(t, countSet)
	})

	t.Run("update existing summary with threshold", func(t *testing.T) {
		// setup mocks
		classificationManager := &mocks.ClassificationManagerMock{
			GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
				return []domain.FeedbackExample{
					{Title: "New AI article", Feedback: domain.FeedbackLike, Topics: []string{"ai"}},
				}, nil
			},
			GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
				return 50, nil
			},
		}

		settingManager := &mocks.SettingManagerMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				switch key {
				case "preference_summary":
					return "Old summary", nil
				case "last_summary_feedback_count":
					return "20", nil
				default:
					return "", nil
				}
			},
			SetSettingFunc: func(ctx context.Context, key, value string) error {
				return nil
			},
		}

		classifier := &mocks.ClassifierMock{
			UpdatePreferenceSummaryFunc: func(ctx context.Context, currentSummary string, newFeedback []domain.FeedbackExample) (string, error) {
				assert.Equal(t, "Old summary", currentSummary)
				assert.Len(t, newFeedback, 1)
				return "Updated summary", nil
			},
		}

		retryFunc := func(ctx context.Context, op func() error) error {
			return op()
		}

		pm := NewPreferenceManager(PreferenceManagerConfig{
			ClassificationManager:      classificationManager,
			SettingManager:             settingManager,
			Classifier:                 classifier,
			PreferenceSummaryThreshold: 25,
			RetryFunc:                  retryFunc,
		})

		// execute
		err := pm.UpdatePreferenceSummary(context.Background())

		// verify
		require.NoError(t, err)
		assert.Len(t, classifier.UpdatePreferenceSummaryCalls(), 1)
		assert.Len(t, settingManager.SetSettingCalls(), 2) // summary and count updated
	})

	t.Run("skip update when below threshold", func(t *testing.T) {
		// setup mocks
		classificationManager := &mocks.ClassificationManagerMock{
			GetRecentFeedbackFunc: func(ctx context.Context, feedbackType string, limit int) ([]domain.FeedbackExample, error) {
				return []domain.FeedbackExample{
					{Title: "Article", Feedback: domain.FeedbackLike},
				}, nil
			},
			GetFeedbackCountFunc: func(ctx context.Context) (int64, error) {
				return 30, nil
			},
		}

		settingManager := &mocks.SettingManagerMock{
			GetSettingFunc: func(ctx context.Context, key string) (string, error) {
				switch key {
				case "preference_summary":
					return "Existing summary", nil
				case "last_summary_feedback_count":
					return "20", nil // only 10 new feedbacks
				default:
					return "", nil
				}
			},
		}

		classifier := &mocks.ClassifierMock{}

		retryFunc := func(ctx context.Context, op func() error) error {
			return op()
		}

		pm := NewPreferenceManager(PreferenceManagerConfig{
			ClassificationManager:      classificationManager,
			SettingManager:             settingManager,
			Classifier:                 classifier,
			PreferenceSummaryThreshold: 25,
			RetryFunc:                  retryFunc,
		})

		// execute
		err := pm.UpdatePreferenceSummary(context.Background())

		// verify
		require.NoError(t, err)
		assert.Empty(t, classifier.GeneratePreferenceSummaryCalls())
		assert.Empty(t, classifier.UpdatePreferenceSummaryCalls())
		assert.Empty(t, settingManager.SetSettingCalls()) // no update
	})
}
