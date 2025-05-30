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
- score: relevance score (0-10). Adjust based on topic preferences if provided.
- explanation: brief explanation (max 100 chars)
- topics: array of 1-3 relevant topic keywords. IMPORTANT: ALWAYS provide topics for EVERY article, regardless of relevance score. Use topics from the provided canonical list when applicable. Only create new topics if absolutely necessary. Even articles with score 0 must have topics that describe their content.
- summary: comprehensive summary that captures the key points, findings, main story, and important details (300-500 chars). Write directly about the content itself. NEVER use phrases like "The article discusses", "The article explores", "The piece covers", "The author explains", etc. Start with the actual subject matter. IMPORTANT: Write the summary in the same language as the article content.

Examples of good summaries:
- "Go 1.22 introduces range-over-function iterators enabling more expressive code patterns. Compilation speeds improve by 50% for large projects through better parallelization. New toolchain management simplifies version control. Runtime gains 10-15% performance boost via enhanced garbage collection algorithms."
- "Scientists discover extensive water ice deposits on Mars equator using orbital radar data from Mars Express spacecraft. Ice layers extend 3.7km deep beneath Medusae Fossae Formation. Discovery challenges understanding of Mars climate history and could support future human missions with accessible water resources."
- "Новый вариант программы-вымогателя BlackCat сначала шифрует облачные резервные копии через API интеграции, затем атакует локальные системы. Использует двойное вымогательство с угрозой публикации данных. Требует оплату в Monero вместо Bitcoin для усложнения отслеживания транзакций." (for Russian content)

Examples of bad summaries:
- "The article discusses new features in Go 1.22..."
- "This piece explores the discovery of water on Mars..."
- "The author explains how ransomware works..."

IMPORTANT: Even low-relevance articles (score 0-3) MUST have topics assigned. Examples:
- Article about "3D sneaker visualizer" (score: 0) should have topics: ["design", "3d", "fashion"]
- Article about "Tunisia travel notes" (score: 2) should have topics: ["travel", "tunisia", "culture"]
- Article about "Music piano rolls" (score: 2) should have topics: ["music", "history", "technology"]

Consider the user's previous feedback when provided.`

// ClassifyRequest contains all parameters for article classification
type ClassifyRequest struct {
	Articles          []db.Item
	Feedbacks         []db.FeedbackExample
	CanonicalTopics   []string
	PreferenceSummary string
}

// Classify classifies articles using the provided request parameters
func (c *Classifier) Classify(ctx context.Context, req ClassifyRequest) ([]db.Classification, error) {
	if len(req.Articles) == 0 {
		return []db.Classification{}, nil
	}

	// prepare the prompt
	prompt := c.buildPromptWithSummary(req.Articles, req.Feedbacks, req.CanonicalTopics, req.PreferenceSummary)

	// retry up to 3 times if we get invalid JSON
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		// create the chat completion request
		chatReq := openai.ChatCompletionRequest{
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
			chatReq.ResponseFormat = &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONObject,
			}
		}

		// call the LLM
		resp, err := c.client.CreateChatCompletion(ctx, chatReq)
		if err != nil {
			return nil, fmt.Errorf("llm request failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no response from llm")
		}

		// parse the response
		content := resp.Choices[0].Message.Content
		classifications, err := c.parseResponse(content, req.Articles)
		if err == nil {
			return classifications, nil
		}

		// save the error for potential return
		lastErr = err

		// if this was a JSON parsing error, retry
		if strings.Contains(err.Error(), "failed to parse json") || strings.Contains(err.Error(), "no json array found") {
			continue
		}

		// for other errors, don't retry
		return nil, err
	}

	return nil, fmt.Errorf("failed after 3 attempts: %w", lastErr)
}

// buildPrompt creates the prompt for the LLM
func (c *Classifier) buildPrompt(articles []db.Item, feedbackExamples []db.FeedbackExample, canonicalTopics []string) string {
	return c.buildPromptWithSummary(articles, feedbackExamples, canonicalTopics, "")
}

// buildPromptWithSummary creates the prompt for the LLM with optional preference summary
func (c *Classifier) buildPromptWithSummary(articles []db.Item, feedbackExamples []db.FeedbackExample, canonicalTopics []string, preferenceSummary string) string {
	var sb strings.Builder

	// add preference summary if available
	if preferenceSummary != "" {
		sb.WriteString("User preference summary (based on historical feedback):\n")
		sb.WriteString(preferenceSummary)
		sb.WriteString("\n\n")
	}

	// add canonical topics if available
	if len(canonicalTopics) > 0 {
		sb.WriteString("Available topics (use one of these when applicable):\n")
		sb.WriteString(strings.Join(canonicalTopics, ", "))
		sb.WriteString("\n\n")
	}

	// add topic preferences
	if len(c.config.Classification.PreferredTopics) > 0 || len(c.config.Classification.AvoidedTopics) > 0 {
		sb.WriteString("Topic preferences:\n")
		if len(c.config.Classification.PreferredTopics) > 0 {
			sb.WriteString(fmt.Sprintf("- Preferred topics (increase score by 1-2): %s\n", strings.Join(c.config.Classification.PreferredTopics, ", ")))
		}
		if len(c.config.Classification.AvoidedTopics) > 0 {
			sb.WriteString(fmt.Sprintf("- Avoided topics (decrease score by 1-2): %s\n", strings.Join(c.config.Classification.AvoidedTopics, ", ")))
		}
		sb.WriteString("\n")
	}

	// add feedback examples if available
	if len(feedbackExamples) > 0 {
		sb.WriteString("Recent user feedback:\n")
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

// GeneratePreferenceSummary creates initial summary from feedback history
func (c *Classifier) GeneratePreferenceSummary(ctx context.Context, feedback []db.FeedbackExample) (string, error) {
	if len(feedback) == 0 {
		return "", fmt.Errorf("no feedback provided")
	}

	// build prompt for summary generation
	var sb strings.Builder
	sb.WriteString("Analyze the following user feedback on articles and create a comprehensive preference summary.\n")
	sb.WriteString("The summary should capture patterns in what the user likes and dislikes.\n")
	sb.WriteString("Be specific about content types, writing styles, technical depth, and topics.\n")
	sb.WriteString("Keep the summary concise (200-300 words) but insightful.\n\n")

	sb.WriteString("User feedback history:\n\n")
	for _, ex := range feedback {
		sb.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(ex.Feedback), ex.Title))
		if ex.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", ex.Description))
		}
		if ex.Content != "" {
			sb.WriteString(fmt.Sprintf("  Content preview: %s\n", ex.Content))
		}
		if len(ex.Topics) > 0 {
			sb.WriteString(fmt.Sprintf("  Topics: %s\n", strings.Join(ex.Topics, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Generate a preference summary that will help classify future articles more accurately.")

	// create the chat completion request
	req := openai.ChatCompletionRequest{
		Model:       c.config.Model,
		Temperature: 0.7,
		MaxTokens:   500,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You are an AI assistant that analyzes user preferences based on their article feedback.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: sb.String(),
			},
		},
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("generate preference summary failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from llm")
	}

	return resp.Choices[0].Message.Content, nil
}

// UpdatePreferenceSummary updates existing summary with new feedback
func (c *Classifier) UpdatePreferenceSummary(ctx context.Context, currentSummary string, newFeedback []db.FeedbackExample) (string, error) {
	if currentSummary == "" {
		return "", fmt.Errorf("no current summary provided")
	}
	if len(newFeedback) == 0 {
		return currentSummary, nil // nothing to update
	}

	// build prompt for summary update
	var sb strings.Builder
	sb.WriteString("Update the following preference summary based on new user feedback.\n")
	sb.WriteString("Incorporate the new patterns while preserving existing insights.\n")
	sb.WriteString("Keep the updated summary concise (200-300 words) but comprehensive.\n\n")

	sb.WriteString("Current preference summary:\n")
	sb.WriteString(currentSummary)
	sb.WriteString("\n\n")

	sb.WriteString("New user feedback:\n\n")
	for _, ex := range newFeedback {
		sb.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(ex.Feedback), ex.Title))
		if ex.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", ex.Description))
		}
		if ex.Content != "" {
			sb.WriteString(fmt.Sprintf("  Content preview: %s\n", ex.Content))
		}
		if len(ex.Topics) > 0 {
			sb.WriteString(fmt.Sprintf("  Topics: %s\n", strings.Join(ex.Topics, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Generate an updated preference summary that incorporates these new insights.")

	// create the chat completion request
	req := openai.ChatCompletionRequest{
		Model:       c.config.Model,
		Temperature: 0.7,
		MaxTokens:   500,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You are an AI assistant that refines user preference summaries based on ongoing feedback.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: sb.String(),
			},
		},
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("update preference summary failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from llm")
	}

	return resp.Choices[0].Message.Content, nil
}
