package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/go-pkgz/repeater/v2"
	"github.com/sashabaranov/go-openai"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/domain"
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
- summary: comprehensive summary that captures the key points, findings, main story, and important details (300-500 chars). RULE: Start DIRECTLY with the facts. NO meta-language. BAD: "The article discusses X". GOOD: "X happens/exists/works". Write the summary in the same language as the article content.

Examples of good summaries:
- "Go 1.22 introduces range-over-function iterators enabling more expressive code patterns. Compilation speeds improve by 50% for large projects through better parallelization. New toolchain management simplifies version control. Runtime gains 10-15% performance boost via enhanced garbage collection algorithms."
- "Scientists discover extensive water ice deposits on Mars equator using orbital radar data from Mars Express spacecraft. Ice layers extend 3.7km deep beneath Medusae Fossae Formation. Discovery challenges understanding of Mars climate history and could support future human missions with accessible water resources."
- "Новый вариант программы-вымогателя BlackCat сначала шифрует облачные резервные копии через API интеграции, затем атакует локальные системы. Использует двойное вымогательство с угрозой публикации данных. Требует оплату в Monero вместо Bitcoin для усложнения отслеживания транзакций." (for Russian content)

Examples of BAD summaries (NEVER write like this):
- "The article discusses new features in Go 1.22..." ❌
- "This piece explores the discovery of water on Mars..." ❌
- "The author explains how ransomware works..." ❌
- "It examines the impact of AI on healthcare..." ❌
- "The post describes a new programming technique..." ❌

Remember: Write as if you ARE presenting the information, not describing someone else's writing.

IMPORTANT: Even low-relevance articles (score 0-3) MUST have topics assigned. Examples:
- Article about "3D sneaker visualizer" (score: 0) should have topics: ["design", "3d", "fashion"]
- Article about "Tunisia travel notes" (score: 2) should have topics: ["travel", "tunisia", "culture"]
- Article about "Music piano rolls" (score: 2) should have topics: ["music", "history", "technology"]

Consider the user's previous feedback when provided.`

// ClassifyRequest contains all parameters for article classification
type ClassifyRequest struct {
	Articles          []domain.Item
	Feedbacks         []domain.FeedbackExample
	CanonicalTopics   []string
	PreferenceSummary string
	PreferredTopics   []string
	AvoidedTopics     []string
}

// classify classifies articles using the provided request parameters (internal implementation)
func (c *Classifier) classify(ctx context.Context, req ClassifyRequest) ([]domain.Classification, error) {
	if len(req.Articles) == 0 {
		return []domain.Classification{}, nil
	}

	// prepare the prompt
	prompt := c.buildPromptWithSummary(req.Articles, req.Feedbacks, req.CanonicalTopics, req.PreferenceSummary, req.PreferredTopics, req.AvoidedTopics)

	var classifications []domain.Classification

	// get retry attempts from config, default to 3
	retryAttempts := c.config.Classification.SummaryRetryAttempts
	if retryAttempts == 0 {
		retryAttempts = 3
	}

	// outer loop for summary validation retries
	for attempt := 0; attempt <= retryAttempts; attempt++ {
		// use repeater for resilient API calls with exponential backoff
		err := repeater.NewBackoff(5, time.Second,
			repeater.WithMaxDelay(30*time.Second),
			repeater.WithJitter(0.1),
		).Do(ctx, func() error {
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
				// all errors will be retried by repeater
				return fmt.Errorf("llm request failed: %w", err)
			}

			if len(resp.Choices) == 0 {
				// this is an unexpected response, but we'll retry it
				return fmt.Errorf("no response from llm")
			}

			// parse the response
			content := resp.Choices[0].Message.Content
			var parseErr error
			classifications, parseErr = c.parseResponse(content, req.Articles)
			if parseErr != nil {
				// all parsing errors will be retried
				return fmt.Errorf("failed to parse response: %w", parseErr)
			}

			return nil
		})

		if err != nil {
			return nil, err
		}

		// check if any summaries need fixing
		needsRetry := false
		badSummaryCount := 0
		for i := range classifications {
			if c.hasForbiddenPrefix(classifications[i].Summary) {
				needsRetry = true
				badSummaryCount++
				// if this is the last attempt, clean the summary instead of retrying
				if attempt == retryAttempts {
					original := classifications[i].Summary
					classifications[i].Summary = c.cleanSummary(classifications[i].Summary)
					if classifications[i].Summary != original {
						// log that we cleaned a summary
						log.Printf("[INFO] cleaned summary for article %q: removed forbidden prefix", classifications[i].GUID)
					}
				}
			}
		}

		// if all summaries are good or we've exhausted retries, return
		if !needsRetry || attempt == retryAttempts {
			if attempt > 0 && !needsRetry {
				log.Printf("[INFO] summary validation succeeded after %d retries", attempt)
			} else if needsRetry && attempt == retryAttempts {
				log.Printf("[WARN] exhausted %d retries, %d summaries still have forbidden prefixes", retryAttempts, badSummaryCount)
			}
			return classifications, nil
		}

		// log retry attempt
		log.Printf("[INFO] retrying classification (attempt %d/%d): %d summaries have forbidden prefixes", attempt+1, retryAttempts, badSummaryCount)

		// add a note to the prompt about the issue
		if attempt == 0 {
			prompt += "\n\nIMPORTANT: Remember to write summaries DIRECTLY without meta-language. Do NOT start with 'The article discusses' or similar phrases."
		}
	}

	return classifications, nil
}

// buildPrompt creates the prompt for the LLM
func (c *Classifier) buildPrompt(articles []domain.Item, feedbackExamples []domain.FeedbackExample, canonicalTopics []string) string {
	return c.buildPromptWithSummary(articles, feedbackExamples, canonicalTopics, "", nil, nil)
}

// buildPromptWithSummary creates the prompt for the LLM with optional preference summary
func (c *Classifier) buildPromptWithSummary(articles []domain.Item, feedbackExamples []domain.FeedbackExample, canonicalTopics []string, preferenceSummary string, preferredTopics, avoidedTopics []string) string {
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
	if len(preferredTopics) > 0 || len(avoidedTopics) > 0 {
		sb.WriteString("Topic preferences:\n")
		if len(preferredTopics) > 0 {
			sb.WriteString(fmt.Sprintf("- Preferred topics (increase score by 1-2): %s\n", strings.Join(preferredTopics, ", ")))
		}
		if len(avoidedTopics) > 0 {
			sb.WriteString(fmt.Sprintf("- Avoided topics (decrease score by 1-2): %s\n", strings.Join(avoidedTopics, ", ")))
		}
		sb.WriteString("\n")
	}

	// add feedback examples if available
	if len(feedbackExamples) > 0 {
		sb.WriteString("Recent user feedback:\n")
		for _, ex := range feedbackExamples {
			sb.WriteString(fmt.Sprintf("- %s article: %s\n", string(ex.Feedback), ex.Title))
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
		if article.Content != "" {
			// limit content to first 500 chars (rune-safe)
			content := article.Content
			runes := []rune(content)
			if len(runes) > 500 {
				content = string(runes[:500]) + "..."
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
func (c *Classifier) parseResponse(content string, articles []domain.Item) ([]domain.Classification, error) {
	var classifications []domain.Classification

	if c.config.Classification.UseJSONMode {
		// parse as JSON object with classifications array
		var resp struct {
			Classifications []domain.Classification `json:"classifications"`
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

	var valid []domain.Classification
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

// hasForbiddenPrefix checks if summary starts with forbidden phrases
func (c *Classifier) hasForbiddenPrefix(summary string) bool {
	if summary == "" {
		return false
	}

	lowerSummary := strings.ToLower(strings.TrimSpace(summary))

	// check if summary starts with any forbidden prefix
	for _, prefix := range c.getForbiddenPrefixes() {
		if strings.HasPrefix(lowerSummary, strings.ToLower(prefix)) {
			return true
		}
	}

	return false
}

// cleanSummary removes forbidden prefixes from summary
func (c *Classifier) cleanSummary(summary string) string {
	if summary == "" {
		return summary
	}

	lowerSummary := strings.ToLower(strings.TrimSpace(summary))

	// check if summary starts with any forbidden prefix
	for _, prefix := range c.getForbiddenPrefixes() {
		lowerPrefix := strings.ToLower(prefix)
		if strings.HasPrefix(lowerSummary, lowerPrefix) {
			// try to extract the actual content after the meta-language
			// look for what comes after the forbidden phrase
			remaining := summary[len(prefix):]
			remaining = strings.TrimSpace(remaining)

			// capitalize first letter if needed
			if remaining != "" {
				runes := []rune(remaining)
				runes[0] = unicode.ToUpper(runes[0])
				return string(runes)
			}
		}
	}

	return summary
}

// getForbiddenPrefixes returns the list of forbidden summary prefixes
func (c *Classifier) getForbiddenPrefixes() []string {
	// if custom forbidden prefixes are configured, use them
	if len(c.config.Classification.ForbiddenSummaryPrefixes) > 0 {
		return c.config.Classification.ForbiddenSummaryPrefixes
	}

	// otherwise use defaults
	return []string{
		"The article discusses", "The article introduces", "The article analyzes", "The article explores",
		"The article examines", "The article explains", "The article details", "The article critiques",
		"The article narrates", "The article describes", "The article highlights", "The article presents",
		"The article covers", "Article discusses", "Article introduces", "Article analyzes",
		"Article explores", "Article examines", "Article explains", "Article details", "Article critiques",
		"Article narrates", "Article describes", "Article highlights", "Article presents", "Article covers",
		"This article", "This post", "The post", "The piece", "Provides an overview", "Discusses",
		"Introduces", "Analyzes", "Explores", "Examines", "Explains", "Details", "Critiques", "Narrates",
		"Describes", "Highlights", "Presents", "Covers", "It explores", "It discusses", "It examines",
		"It explains", "It describes", "It details", "The author discusses", "The author explores",
		"The author explains", "The author describes", "The author analyzes", "The author examines",
	}
}

// ClassifyItems implements the scheduler.Classifier interface
func (c *Classifier) ClassifyItems(ctx context.Context, req ClassifyRequest) ([]domain.Classification, error) {
	return c.classify(ctx, req)
}

// default prompts for preference summary operations
const (
	defaultGenerateSummaryPrompt = `Analyze the following user feedback on articles and create a comprehensive preference summary.
The summary should capture patterns in what the user likes and dislikes.
Be specific about content types, writing styles, technical depth, and topics.
Keep the summary concise (200-300 words) but insightful.`

	defaultUpdateSummaryPrompt = `Update the following preference summary based on new user feedback.
Incorporate the new patterns while preserving existing insights.
Keep the updated summary concise (200-300 words) but comprehensive.`
)

// GeneratePreferenceSummary creates initial summary from feedback history
func (c *Classifier) GeneratePreferenceSummary(ctx context.Context, feedback []domain.FeedbackExample) (string, error) {
	if len(feedback) == 0 {
		return "", fmt.Errorf("no feedback provided")
	}

	// use custom prompt if provided, otherwise use default
	prompt := c.config.Classification.Prompts.GenerateSummary
	if prompt == "" {
		prompt = defaultGenerateSummaryPrompt
	}

	// build prompt for summary generation
	var sb strings.Builder
	sb.WriteString(prompt)
	sb.WriteString("\n\n")

	sb.WriteString("User feedback history:\n\n")
	for _, ex := range feedback {
		sb.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(string(ex.Feedback)), ex.Title))
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

	var summary string

	// use repeater for resilient API calls with exponential backoff
	err := repeater.NewBackoff(5, time.Second,
		repeater.WithMaxDelay(30*time.Second),
		repeater.WithJitter(0.1),
	).Do(ctx, func() error {
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
			return fmt.Errorf("generate preference summary failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return fmt.Errorf("no response from llm")
		}

		summary = resp.Choices[0].Message.Content
		return nil
	})

	if err != nil {
		return "", err
	}

	return summary, nil
}

// UpdatePreferenceSummary updates existing summary with new feedback
func (c *Classifier) UpdatePreferenceSummary(ctx context.Context, currentSummary string, newFeedback []domain.FeedbackExample) (string, error) {
	if currentSummary == "" {
		return "", fmt.Errorf("no current summary provided")
	}
	if len(newFeedback) == 0 {
		return currentSummary, nil // nothing to update
	}

	// use custom prompt if provided, otherwise use default
	prompt := c.config.Classification.Prompts.UpdateSummary
	if prompt == "" {
		prompt = defaultUpdateSummaryPrompt
	}

	// build prompt for summary update
	var sb strings.Builder
	sb.WriteString(prompt)
	sb.WriteString("\n\n")

	sb.WriteString("Current preference summary:\n")
	sb.WriteString(currentSummary)
	sb.WriteString("\n\n")

	sb.WriteString("New user feedback:\n\n")
	for _, ex := range newFeedback {
		sb.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(string(ex.Feedback)), ex.Title))
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

	var updatedSummary string

	// use repeater for resilient API calls with exponential backoff
	err := repeater.NewBackoff(5, time.Second,
		repeater.WithMaxDelay(30*time.Second),
		repeater.WithJitter(0.1),
	).Do(ctx, func() error {
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
			return fmt.Errorf("update preference summary failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return fmt.Errorf("no response from llm")
		}

		updatedSummary = resp.Choices[0].Message.Content
		return nil
	})

	if err != nil {
		return "", err
	}

	return updatedSummary, nil
}
