package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/newscope/pkg/domain"
)

// PreferenceManager handles user preference learning and summary management.
// It is responsible for:
//   - Learning user preferences from feedback on classified items
//   - Generating and updating preference summaries using the LLM
//   - Managing the preference update lifecycle with debouncing
//   - Tracking feedback counts to determine when updates are needed
//   - Persisting preference summaries for use in classification
//
// The PreferenceManager uses a threshold-based approach to avoid excessive
// LLM calls, only updating when sufficient new feedback is available.
type PreferenceManager struct {
	classificationManager ClassificationManager
	settingManager        SettingManager
	classifier            Classifier

	preferenceSummaryThreshold int
	retryFunc                  func(ctx context.Context, operation func() error) error
}

// PreferenceManagerConfig holds configuration for PreferenceManager
type PreferenceManagerConfig struct {
	ClassificationManager      ClassificationManager
	SettingManager             SettingManager
	Classifier                 Classifier
	PreferenceSummaryThreshold int
	RetryFunc                  func(ctx context.Context, operation func() error) error
}

// NewPreferenceManager creates a new preference manager with the provided configuration.
// The configuration must include the classification manager for feedback access,
// setting manager for persistence, classifier for LLM operations, and operational
// parameters (threshold, retry function).
func NewPreferenceManager(cfg PreferenceManagerConfig) *PreferenceManager {
	return &PreferenceManager{
		classificationManager:      cfg.ClassificationManager,
		settingManager:             cfg.SettingManager,
		classifier:                 cfg.Classifier,
		preferenceSummaryThreshold: cfg.PreferenceSummaryThreshold,
		retryFunc:                  cfg.RetryFunc,
	}
}

// UpdatePreferenceSummary updates the user preference summary based on recent feedback.
// It checks if enough new feedback has accumulated since the last update
// (based on preferenceSummaryThreshold), and if so, generates or updates
// the preference summary using the LLM. This method is designed to be
// called periodically but will only perform expensive LLM operations when needed.
func (pm *PreferenceManager) UpdatePreferenceSummary(ctx context.Context) error {
	// get more feedback examples for better learning (50 instead of 10)
	const feedbackExamples = 50

	feedbacks, err := pm.classificationManager.GetRecentFeedback(ctx, "", feedbackExamples)
	if err != nil {
		return fmt.Errorf("get recent feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		lgr.Printf("[INFO] no feedback to process for preference summary")
		return nil
	}

	// get current preference summary
	currentSummary, err := pm.settingManager.GetSetting(ctx, "preference_summary")
	if err != nil || currentSummary == "" {
		return pm.generateInitialPreferenceSummary(ctx, feedbacks)
	}

	// get last feedback count from settings
	lastCountStr, _ := pm.settingManager.GetSetting(ctx, "last_summary_feedback_count")
	lastCount := int64(0)
	if lastCountStr != "" {
		if parsed, err := strconv.ParseInt(lastCountStr, 10, 64); err == nil {
			lastCount = parsed
		}
	}

	// get current feedback count
	currentCount, err := pm.classificationManager.GetFeedbackCount(ctx)
	if err != nil {
		return fmt.Errorf("get feedback count: %w", err)
	}

	// calculate new feedback since last update
	newFeedbackCount := currentCount - lastCount
	if newFeedbackCount < int64(pm.preferenceSummaryThreshold) {
		lgr.Printf("[DEBUG] only %d new feedbacks since last update, skipping (threshold: %d)", newFeedbackCount, pm.preferenceSummaryThreshold)
		return nil
	}

	// check context before proceeding with expensive operation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	lgr.Printf("[INFO] updating preference summary with %d new feedbacks (total: %d)", newFeedbackCount, currentCount)

	// update the preference summary
	newSummary, err := pm.classifier.UpdatePreferenceSummary(ctx, currentSummary, feedbacks)
	if err != nil {
		return fmt.Errorf("update preference summary: %w", err)
	}

	// save updated summary and count
	err = pm.retryFunc(ctx, func() error {
		return pm.settingManager.SetSetting(ctx, "preference_summary", newSummary)
	})
	if err != nil {
		return fmt.Errorf("save preference summary: %w", err)
	}

	err = pm.retryFunc(ctx, func() error {
		return pm.settingManager.SetSetting(ctx, "last_summary_feedback_count", strconv.FormatInt(currentCount, 10))
	})
	if err != nil {
		return fmt.Errorf("save feedback count: %w", err)
	}

	lgr.Printf("[INFO] preference summary updated successfully")
	return nil
}

// PreferenceUpdateWorker processes preference update requests with debouncing.
// It listens on the updateCh channel for update signals but implements a
// debounce mechanism (5 minutes) to batch multiple rapid update requests.
// This prevents excessive LLM calls when users provide feedback in bursts.
// The worker runs until the context is canceled.
func (pm *PreferenceManager) PreferenceUpdateWorker(ctx context.Context, updateCh <-chan struct{}) {
	// debounce timer to avoid too frequent updates
	const debounceDelay = 5 * time.Minute
	debounceTimer := time.NewTimer(0)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			debounceTimer.Stop()
			return
		case <-updateCh:
			// reset debounce timer
			debounceTimer.Stop()
			debounceTimer.Reset(debounceDelay)
		case <-debounceTimer.C:
			// debounce period expired, perform update
			lgr.Printf("[DEBUG] processing preference update")
			if err := pm.UpdatePreferenceSummary(ctx); err != nil {
				lgr.Printf("[WARN] failed to update preference summary: %v", err)
			}
		}
	}
}

// generateInitialPreferenceSummary creates the first preference summary from feedback
func (pm *PreferenceManager) generateInitialPreferenceSummary(ctx context.Context, feedbacks []domain.FeedbackExample) error {
	lgr.Printf("[INFO] generating initial preference summary from %d feedback examples", len(feedbacks))
	newSummary, err := pm.classifier.GeneratePreferenceSummary(ctx, feedbacks)
	if err != nil {
		return fmt.Errorf("generate initial preference summary: %w", err)
	}

	err = pm.retryFunc(ctx, func() error {
		return pm.settingManager.SetSetting(ctx, "preference_summary", newSummary)
	})
	if err != nil {
		return fmt.Errorf("save initial preference summary: %w", err)
	}

	// get current feedback count
	currentCount, err := pm.classificationManager.GetFeedbackCount(ctx)
	if err != nil {
		return fmt.Errorf("get feedback count: %w", err)
	}

	// save initial feedback count
	err = pm.retryFunc(ctx, func() error {
		return pm.settingManager.SetSetting(ctx, "last_summary_feedback_count", strconv.FormatInt(currentCount, 10))
	})
	if err != nil {
		return fmt.Errorf("save initial feedback count: %w", err)
	}

	lgr.Printf("[INFO] initial preference summary generated successfully")
	return nil
}
