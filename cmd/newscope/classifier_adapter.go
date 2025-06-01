package main

import (
	"context"

	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/domain"
	"github.com/umputun/newscope/pkg/llm"
)

// ClassifierAdapter adapts the old LLM classifier to the new scheduler interface
type ClassifierAdapter struct {
	classifier *llm.Classifier
}

// NewClassifierAdapter creates a new classifier adapter
func NewClassifierAdapter(classifier *llm.Classifier) *ClassifierAdapter {
	return &ClassifierAdapter{classifier: classifier}
}

// ClassifyItems implements the scheduler.Classifier interface
func (a *ClassifierAdapter) ClassifyItems(ctx context.Context, items []*domain.Item, feedbacks []*domain.FeedbackExample, topics []string, preferenceSummary string) ([]*domain.Classification, error) {
	if a.classifier == nil {
		// return empty classifications if no classifier configured
		result := make([]*domain.Classification, len(items))
		for i := range items {
			result[i] = &domain.Classification{
				Score:       0.0,
				Explanation: "No classifier configured",
				Topics:      []string{},
				Summary:     "",
			}
		}
		return result, nil
	}

	// Convert domain types to db types for the old classifier
	dbItems := make([]db.Item, len(items))
	for i, item := range items {
		dbItems[i] = db.Item{
			ID:               item.ID,
			FeedID:           item.FeedID,
			GUID:             item.GUID,
			Title:            item.Title,
			Link:             item.Link,
			Description:      item.Description,
			Content:          item.Content,
			Author:           item.Author,
			Published:        item.Published,
			ExtractedContent: item.Content, // use the content for classification
		}
	}

	dbFeedbacks := make([]db.FeedbackExample, len(feedbacks))
	for i, feedback := range feedbacks {
		dbFeedbacks[i] = db.FeedbackExample{
			Title:       feedback.Title,
			Description: feedback.Description,
			Content:     feedback.Content,
			Feedback:    string(feedback.Feedback),
			Topics:      feedback.Topics,
		}
	}

	// Call the old classifier
	classifications, err := a.classifier.Classify(ctx, llm.ClassifyRequest{
		Articles:          dbItems,
		Feedbacks:         dbFeedbacks,
		CanonicalTopics:   topics,
		PreferenceSummary: preferenceSummary,
	})
	if err != nil {
		return nil, err
	}

	// Convert back to domain types
	result := make([]*domain.Classification, len(classifications))
	for i, classification := range classifications {
		result[i] = &domain.Classification{
			Score:       classification.Score,
			Explanation: classification.Explanation,
			Topics:      classification.Topics,
			Summary:     classification.Summary,
		}
	}

	return result, nil
}

// GeneratePreferenceSummary implements the scheduler.Classifier interface
func (a *ClassifierAdapter) GeneratePreferenceSummary(ctx context.Context, feedback []*domain.FeedbackExample) (string, error) {
	if a.classifier == nil {
		return "", nil
	}

	// Convert domain types to db types
	dbFeedbacks := make([]db.FeedbackExample, len(feedback))
	for i, f := range feedback {
		dbFeedbacks[i] = db.FeedbackExample{
			Title:       f.Title,
			Description: f.Description,
			Content:     f.Content,
			Feedback:    string(f.Feedback),
			Topics:      f.Topics,
		}
	}

	return a.classifier.GeneratePreferenceSummary(ctx, dbFeedbacks)
}

// UpdatePreferenceSummary implements the scheduler.Classifier interface
func (a *ClassifierAdapter) UpdatePreferenceSummary(ctx context.Context, currentSummary string, newFeedback []*domain.FeedbackExample) (string, error) {
	if a.classifier == nil {
		return currentSummary, nil
	}

	// Convert domain types to db types
	dbFeedbacks := make([]db.FeedbackExample, len(newFeedback))
	for i, f := range newFeedback {
		dbFeedbacks[i] = db.FeedbackExample{
			Title:       f.Title,
			Description: f.Description,
			Content:     f.Content,
			Feedback:    string(f.Feedback),
			Topics:      f.Topics,
		}
	}

	return a.classifier.UpdatePreferenceSummary(ctx, currentSummary, dbFeedbacks)
}