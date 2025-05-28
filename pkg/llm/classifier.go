package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/db"
)

// Classifier uses LLM to classify articles
type Classifier struct {
	client    *openai.Client
	config    config.LLMConfig
	systemMsg string
}

// NewClassifier creates a new LLM classifier
func NewClassifier(cfg config.LLMConfig) *Classifier {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if cfg.Endpoint != "" {
		clientConfig.BaseURL = cfg.Endpoint
	}

	// use custom system prompt if provided, otherwise use default
	systemMsg := cfg.SystemPrompt
	if systemMsg == "" {
		systemMsg = defaultSystemPrompt
	}

	return &Classifier{
		client:    openai.NewClientWithConfig(clientConfig),
		config:    cfg,
		systemMsg: systemMsg,
	}
}

// default system prompt for article classification
const defaultSystemPrompt = `You are an AI assistant that evaluates articles for relevance to the user's interests.
Rate each article from 0-10 where:
- 0-3: Not relevant
- 4-6: Somewhat relevant
- 7-8: Relevant
- 9-10: Highly relevant

Each classification should contain:
- guid: the article's GUID
- score: relevance score (0-10)
- explanation: brief explanation (max 100 chars)
- topics: array of 1-3 relevant topic keywords
- summary: concise 2-3 sentence summary of the article (max 200 chars)

Consider the user's previous feedback when provided.`

// ClassifyArticles classifies a batch of articles
func (c *Classifier) ClassifyArticles(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample) ([]db.Classification, error) {
	if len(articles) == 0 {
		return []db.Classification{}, nil
	}

	// prepare the prompt
	prompt := c.buildPrompt(articles, feedbacks)

	// create the chat completion request
	req := openai.ChatCompletionRequest{
		Model:       c.config.Model,
		Temperature: float32(c.config.Temperature),
		MaxTokens:   c.config.MaxTokens,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: c.systemMsg,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	}

	// add JSON response format if enabled
	if c.config.Classification.UseJSONMode {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}
	}

	// call the LLM
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from llm")
	}

	// parse the response
	content := resp.Choices[0].Message.Content
	return c.parseResponse(content, articles)
}

// buildPrompt creates the prompt for the LLM
func (c *Classifier) buildPrompt(articles []db.Item, feedbackExamples []db.FeedbackExample) string {
	var sb strings.Builder

	// add feedback examples if available
	if len(feedbackExamples) > 0 {
		sb.WriteString("Based on user feedback:\n")
		for _, ex := range feedbackExamples {
			sb.WriteString(fmt.Sprintf("- %s article: %s\n", ex.Feedback, ex.Title))
			if len(ex.Topics) > 0 {
				sb.WriteString(fmt.Sprintf("  Topics: %s\n", strings.Join(ex.Topics, ", ")))
			}
		}
		sb.WriteString("\n")
	}

	// add articles to classify
	sb.WriteString("Classify these articles:\n\n")
	for i, article := range articles {
		sb.WriteString(fmt.Sprintf("%d. GUID: %s\n", i+1, article.GUID))
		sb.WriteString(fmt.Sprintf("   Title: %s\n", article.Title))
		if article.Description != "" {
			sb.WriteString(fmt.Sprintf("   Description: %s\n", article.Description))
		}
		if article.ExtractedContent != "" {
			// limit content to first 500 chars
			content := article.ExtractedContent
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			sb.WriteString(fmt.Sprintf("   Content: %s\n", content))
		}
		sb.WriteString("\n")
	}

	if c.config.Classification.UseJSONMode {
		sb.WriteString("Respond with a JSON object containing a 'classifications' array of classification objects.")
	} else {
		sb.WriteString("Respond with a JSON array of classification objects.")
	}
	return sb.String()
}

// parseResponse parses the LLM response into classifications
func (c *Classifier) parseResponse(content string, articles []db.Item) ([]db.Classification, error) {
	var classifications []db.Classification

	if c.config.Classification.UseJSONMode {
		// parse as JSON object with classifications array
		var resp struct {
			Classifications []db.Classification `json:"classifications"`
		}
		if err := json.Unmarshal([]byte(content), &resp); err != nil {
			return nil, fmt.Errorf("failed to parse json object response: %w", err)
		}
		classifications = resp.Classifications
	} else {
		// parse as JSON array (backward compatible)
		start := strings.Index(content, "[")
		end := strings.LastIndex(content, "]")
		if start == -1 || end == -1 || start >= end {
			return nil, fmt.Errorf("no json array found in response")
		}

		jsonStr := content[start : end+1]
		if err := json.Unmarshal([]byte(jsonStr), &classifications); err != nil {
			return nil, fmt.Errorf("failed to parse json array response: %w", err)
		}
	}

	// validate we got classifications for all articles
	guidMap := make(map[string]bool)
	for _, article := range articles {
		guidMap[article.GUID] = true
	}

	var valid []db.Classification
	for _, class := range classifications {
		if guidMap[class.GUID] {
			// ensure score is in valid range
			if class.Score < 0 {
				class.Score = 0
			} else if class.Score > 10 {
				class.Score = 10
			}
			valid = append(valid, class)
		}
	}

	return valid, nil
}
