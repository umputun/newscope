package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/domain"
)

func TestClassifier_Classify(t *testing.T) {
	// create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		// return mock response
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: `Here are the classifications:

[
  {
    "guid": "item1",
    "score": 8.5,
    "explanation": "Highly relevant Go programming content",
    "topics": ["golang", "programming", "backend"],
    "summary": "Go 1.22 brings range-over-function iterators enabling cleaner iteration patterns over custom types. Compilation speeds increase 50% for large projects through parallel compilation improvements. New toolchain versioning system simplifies managing Go versions. Runtime optimizations reduce memory usage by 15% in typical web applications. Profile-guided optimization now supports more optimization patterns."
  },
  {
    "guid": "item2", 
    "score": 3.0,
    "explanation": "Not relevant to tech interests",
    "topics": ["sports", "news"],
    "summary": "Manchester United defeated Chelsea 3-1 in crucial Premier League clash with Bruno Fernandes scoring twice. Liverpool maintains top position after dramatic 2-2 draw with Arsenal at Emirates Stadium. Multiple red cards issued in heated matches across European leagues including Serie A and La Liga. Champions League spots remain highly contested with six teams within five points. Injury concerns mount for several top clubs ahead of international break."
  }
]`,
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// create classifier with test server
	cfg := config.LLMConfig{
		Endpoint:    server.URL + "/v1",
		APIKey:      "test-key",
		Model:       "gpt-4",
		Temperature: 0.3,
		MaxTokens:   500,
	}
	classifier := NewClassifier(cfg)

	// test articles
	articles := []domain.Item{
		{
			GUID:        "item1",
			Title:       "Go 1.22 Released",
			Description: "New features in Go",
			Content:     "Go 1.22 brings exciting new features...",
		},
		{
			GUID:        "item2",
			Title:       "Sports News",
			Description: "Latest football results",
		},
	}

	// test feedback examples
	feedback := []domain.FeedbackExample{
		{
			Title:    "Previous Go Article",
			Feedback: domain.FeedbackLike,
			Topics:   []string{"golang"},
		},
	}

	// classify articles
	ctx := context.Background()
	canonicalTopics := []string{"golang", "programming", "backend", "sports", "news", "tech"}
	classifications, err := classifier.ClassifyItems(ctx, ClassifyRequest{
		Articles:        articles,
		Feedbacks:       feedback,
		CanonicalTopics: canonicalTopics,
	})
	require.NoError(t, err)
	require.Len(t, classifications, 2)

	// check first classification
	assert.Equal(t, "item1", classifications[0].GUID)
	assert.InEpsilon(t, 8.5, classifications[0].Score, 0.001)
	assert.Equal(t, "Highly relevant Go programming content", classifications[0].Explanation)
	assert.Equal(t, []string{"golang", "programming", "backend"}, classifications[0].Topics)
	assert.NotEmpty(t, classifications[0].Summary)
	assert.Contains(t, classifications[0].Summary, "Go 1.22")
	assert.NotContains(t, classifications[0].Summary, "The article")
	assert.NotContains(t, classifications[0].Summary, "discusses")

	// check second classification
	assert.Equal(t, "item2", classifications[1].GUID)
	assert.InEpsilon(t, 3.0, classifications[1].Score, 0.001)
	assert.Equal(t, "Not relevant to tech interests", classifications[1].Explanation)
	assert.Equal(t, []string{"sports", "news"}, classifications[1].Topics)
	assert.NotEmpty(t, classifications[1].Summary)
	assert.Contains(t, classifications[1].Summary, "Manchester United")
	assert.NotContains(t, classifications[1].Summary, "The article")
}

func TestClassifier_ClassifyArticles_EmptyInput(t *testing.T) {
	cfg := config.LLMConfig{
		APIKey: "test-key",
		Model:  "gpt-4",
	}
	classifier := NewClassifier(cfg)

	ctx := context.Background()
	classifications, err := classifier.ClassifyItems(ctx, ClassifyRequest{
		Articles: []domain.Item{},
	})
	require.NoError(t, err)
	assert.Empty(t, classifications)
}

func TestClassifier_CustomSystemPrompt(t *testing.T) {
	customPrompt := "You are a specialized tech curator. Rate articles 0-10."

	cfg := config.LLMConfig{
		APIKey:       "test-key",
		Model:        "gpt-4",
		SystemPrompt: customPrompt,
	}
	classifier := NewClassifier(cfg)

	// verify custom prompt is used
	assert.Equal(t, customPrompt, classifier.systemMsg)
}

func TestClassifier_TopicPreferences(t *testing.T) {
	cfg := config.LLMConfig{
		APIKey: "test-key",
		Model:  "gpt-4",
	}
	classifier := NewClassifier(cfg)

	articles := []domain.Item{{GUID: "item1", Title: "Test Article"}}
	preferredTopics := []string{"golang", "ai"}
	avoidedTopics := []string{"sports", "politics"}
	prompt := classifier.buildPromptWithSummary(articles, nil, nil, "", preferredTopics, avoidedTopics)

	// check topic preferences section
	assert.Contains(t, prompt, "Topic preferences:")
	assert.Contains(t, prompt, "Preferred topics (increase score by 1-2): golang, ai")
	assert.Contains(t, prompt, "Avoided topics (decrease score by 1-2): sports, politics")
}

func TestClassifier_DefaultSystemPrompt(t *testing.T) {
	cfg := config.LLMConfig{
		APIKey: "test-key",
		Model:  "gpt-4",
		// no system prompt provided
	}
	classifier := NewClassifier(cfg)

	// verify default prompt is used
	assert.Contains(t, classifier.systemMsg, "You are an AI assistant that evaluates articles")
	assert.Contains(t, classifier.systemMsg, "0-3: Not relevant")
}

func TestClassifier_buildPrompt(t *testing.T) {
	classifier := &Classifier{config: config.LLMConfig{}}

	articles := []domain.Item{
		{
			GUID:        "item1",
			Title:       "Test Article",
			Description: "Test description",
			Content:     "Long content that is now included in full " + strings.Repeat("x", 500),
		},
	}

	feedback := []domain.FeedbackExample{
		{
			Title:    "Liked Article",
			Feedback: domain.FeedbackLike,
			Topics:   []string{"tech", "ai"},
		},
		{
			Title:    "Disliked Article",
			Feedback: domain.FeedbackDislike,
		},
	}

	canonicalTopics := []string{"tech", "ai", "programming"}
	prompt := classifier.buildPrompt(articles, feedback, canonicalTopics)

	// check canonical topics section
	assert.Contains(t, prompt, "Commonly used topics (use as reference, but create new specific topics when needed):")
	assert.Contains(t, prompt, "tech, ai, programming")

	// check feedback section
	assert.Contains(t, prompt, "Recent user feedback:")
	assert.Contains(t, prompt, "like article: Liked Article")
	assert.Contains(t, prompt, "Topics: tech, ai")
	assert.Contains(t, prompt, "dislike article: Disliked Article")

	// check articles section
	assert.Contains(t, prompt, "Classify these articles:")
	assert.Contains(t, prompt, "GUID: item1")
	assert.Contains(t, prompt, "Title: Test Article")
	assert.Contains(t, prompt, "Description: Test description")
	assert.Contains(t, prompt, "Content: Long content that is now included in full")
	assert.Contains(t, prompt, strings.Repeat("x", 500)) // ensure full content is included

	// check instruction
	assert.Contains(t, prompt, "Respond with a JSON array")
}

func TestClassifier_parseResponse(t *testing.T) {
	classifier := &Classifier{config: config.LLMConfig{}}

	articles := []domain.Item{
		{GUID: "item1"},
		{GUID: "item2"},
		{GUID: "item3"},
	}

	tests := []struct {
		name        string
		response    string
		wantErr     bool
		wantCount   int
		checkResult func(t *testing.T, classifications []domain.Classification)
	}{
		{
			name: "valid json array",
			response: `[
				{"guid": "item1", "score": 7.5, "explanation": "Good", "topics": ["tech"]},
				{"guid": "item2", "score": 2.0, "explanation": "Bad", "topics": []}
			]`,
			wantCount: 2,
			checkResult: func(t *testing.T, classifications []domain.Classification) {
				assert.Equal(t, "item1", classifications[0].GUID)
				assert.InEpsilon(t, 7.5, classifications[0].Score, 0.001)
			},
		},
		{
			name: "json with extra text",
			response: `Here are the results:
			
			[{"guid": "item1", "score": 5}]
			
			That's all!`,
			wantCount: 1,
		},
		{
			name: "score out of range",
			response: `[
				{"guid": "item1", "score": 15, "explanation": "Too high"},
				{"guid": "item2", "score": -5, "explanation": "Too low"}
			]`,
			wantCount: 2,
			checkResult: func(t *testing.T, classifications []domain.Classification) {
				assert.Equal(t, float64(10), classifications[0].Score) //nolint:testifylint // exact value comparison
				assert.Equal(t, float64(0), classifications[1].Score)  //nolint:testifylint // exact value comparison
			},
		},
		{
			name:     "no json array",
			response: `This is just text without any JSON`,
			wantErr:  true,
		},
		{
			name:     "invalid json",
			response: `[{"guid": "item1", "score": }]`,
			wantErr:  true,
		},
		{
			name:      "unknown guid filtered out",
			response:  `[{"guid": "unknown", "score": 5}, {"guid": "item1", "score": 7}]`,
			wantCount: 1,
			checkResult: func(t *testing.T, classifications []domain.Classification) {
				assert.Equal(t, "item1", classifications[0].GUID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classifications, err := classifier.parseResponse(tt.response, articles)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, classifications, tt.wantCount)

			if tt.checkResult != nil {
				tt.checkResult(t, classifications)
			}
		})
	}
}

func TestClassifier_RetryOnInvalidJSON(t *testing.T) {
	attempts := 0
	// create test server that returns invalid JSON on first attempt
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		var content string
		if attempts == 1 {
			// first attempt: return response with no JSON array
			content = "I cannot process this request right now."
		} else {
			// second attempt: return valid JSON
			content = `[{"guid": "item1", "score": 8, "explanation": "Good", "topics": ["tech"]}]`
		}

		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: content,
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.LLMConfig{
		Endpoint: server.URL + "/v1",
		APIKey:   "test-key",
		Model:    "gpt-4",
	}
	classifier := NewClassifier(cfg)

	articles := []domain.Item{{GUID: "item1", Title: "Test"}}
	classifications, err := classifier.ClassifyItems(context.Background(), ClassifyRequest{
		Articles: articles,
	})

	require.NoError(t, err)
	require.Len(t, classifications, 1)
	assert.Equal(t, "item1", classifications[0].GUID)
	assert.InEpsilon(t, 8.0, classifications[0].Score, 0.001)
	assert.Equal(t, 2, attempts, "should retry once after invalid JSON")
}

func TestClassifier_JSONMode(t *testing.T) {
	t.Run("build prompt with JSON mode", func(t *testing.T) {
		classifier := &Classifier{
			config: config.LLMConfig{
				Classification: config.ClassificationConfig{
					UseJSONMode: true,
				},
			},
		}

		articles := []domain.Item{{GUID: "item1", Title: "Test"}}
		prompt := classifier.buildPrompt(articles, nil, nil)

		assert.Contains(t, prompt, "Respond with a JSON object containing a 'classifications' array")
	})

	t.Run("build prompt without JSON mode", func(t *testing.T) {
		classifier := &Classifier{
			config: config.LLMConfig{
				Classification: config.ClassificationConfig{
					UseJSONMode: false,
				},
			},
		}

		articles := []domain.Item{{GUID: "item1", Title: "Test"}}
		prompt := classifier.buildPrompt(articles, nil, nil)

		assert.Contains(t, prompt, "Respond with a JSON array of classification objects")
	})

	t.Run("parse JSON object response", func(t *testing.T) {
		classifier := &Classifier{
			config: config.LLMConfig{
				Classification: config.ClassificationConfig{
					UseJSONMode: true,
				},
			},
		}

		articles := []domain.Item{
			{GUID: "item1"},
			{GUID: "item2"},
		}

		response := `{
			"classifications": [
				{"guid": "item1", "score": 8, "explanation": "Good", "topics": ["tech"], "summary": "Apple unveils Vision Pro headset featuring revolutionary spatial computing capabilities with dual 4K displays per eye. New M3 chips deliver 50% performance boost over M2 through enhanced neural engine and GPU cores. Priced at $3,499, targeting professional and creative markets. Developer SDK released with over 1,000 apps already optimized. Battery life reaches 2 hours with external pack, addressing early concerns about portability."},
				{"guid": "item2", "score": 3, "explanation": "Bad", "topics": ["other"], "summary": "Local bakery Flour Power wins national award for innovative sourdough bread using ancient grain varieties. Owner Maria Chen credits success to 72-hour fermentation process inherited from grandmother. Bakery produces 500 loaves daily using locally sourced organic ingredients. Award includes $50,000 prize and cookbook deal. Plans expansion to three new locations across California by year end."}
			]
		}`

		classifications, err := classifier.parseResponse(response, articles)
		require.NoError(t, err)
		require.Len(t, classifications, 2)

		assert.Equal(t, "item1", classifications[0].GUID)
		assert.InEpsilon(t, 8.0, classifications[0].Score, 0.001)
		assert.Equal(t, "Good", classifications[0].Explanation)
		assert.Equal(t, []string{"tech"}, classifications[0].Topics)

		assert.Equal(t, "item2", classifications[1].GUID)
		assert.InEpsilon(t, 3.0, classifications[1].Score, 0.001)
	})

	t.Run("JSON mode with API call", func(t *testing.T) {
		// create test server that checks for JSON response format
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// decode request to check response format
			var req openai.ChatCompletionRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)

			// verify JSON response format is set
			assert.NotNil(t, req.ResponseFormat)
			assert.Equal(t, openai.ChatCompletionResponseFormatTypeJSONObject, req.ResponseFormat.Type)

			// return mock response in JSON object format
			resp := openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Content: `{"classifications": [{"guid": "item1", "score": 9}]}`,
						},
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := config.LLMConfig{
			Endpoint: server.URL + "/v1",
			APIKey:   "test-key",
			Model:    "gpt-4",
			Classification: config.ClassificationConfig{
				UseJSONMode: true,
			},
		}
		classifier := NewClassifier(cfg)

		articles := []domain.Item{{GUID: "item1", Title: "Test"}}
		classifications, err := classifier.ClassifyItems(context.Background(), ClassifyRequest{
			Articles: articles,
		})

		require.NoError(t, err)
		require.Len(t, classifications, 1)
		assert.Equal(t, "item1", classifications[0].GUID)
		assert.InEpsilon(t, 9.0, classifications[0].Score, 0.001)
	})
}

func TestClassifier_GeneratePreferenceSummary(t *testing.T) {
	// create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		// return mock preference summary
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "User prefers technical articles about Go, AI/ML, and backend development. Likes in-depth tutorials and implementation details. Dislikes sports and entertainment content.",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.LLMConfig{
		Endpoint: server.URL + "/v1",
		APIKey:   "test-key",
		Model:    "gpt-4",
	}
	classifier := NewClassifier(cfg)

	feedback := []domain.FeedbackExample{
		{
			Title:       "Go 1.22 Features",
			Description: "New features in Go",
			Content:     "Range-over-function iterators...",
			Feedback:    domain.FeedbackLike,
			Topics:      []string{"golang", "programming"},
		},
		{
			Title:    "Sports News",
			Feedback: domain.FeedbackDislike,
			Topics:   []string{"sports"},
		},
	}

	summary, err := classifier.GeneratePreferenceSummary(context.Background(), feedback)
	require.NoError(t, err)
	assert.Contains(t, summary, "technical articles")
	assert.Contains(t, summary, "Go")
}

func TestClassifier_UpdatePreferenceSummary(t *testing.T) {
	// create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return mock updated summary
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "User prefers technical articles about Go, AI/ML, backend development, and cloud infrastructure. Likes in-depth tutorials, implementation details, and performance optimizations. Dislikes sports, entertainment, and political content.",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.LLMConfig{
		Endpoint: server.URL + "/v1",
		APIKey:   "test-key",
		Model:    "gpt-4",
	}
	classifier := NewClassifier(cfg)

	currentSummary := "User prefers technical articles about Go, AI/ML, and backend development. Likes in-depth tutorials and implementation details. Dislikes sports and entertainment content."

	newFeedback := []domain.FeedbackExample{
		{
			Title:    "Kubernetes Best Practices",
			Feedback: domain.FeedbackLike,
			Topics:   []string{"kubernetes", "cloud"},
		},
		{
			Title:    "Political News",
			Feedback: domain.FeedbackDislike,
			Topics:   []string{"politics"},
		},
	}

	updatedSummary, err := classifier.UpdatePreferenceSummary(context.Background(), currentSummary, newFeedback)
	require.NoError(t, err)
	assert.Contains(t, updatedSummary, "cloud infrastructure")
	assert.Contains(t, updatedSummary, "political")
}

func TestClassifier_CustomPrompts(t *testing.T) {
	t.Run("custom generate summary prompt", func(t *testing.T) {
		customPrompt := "Custom prompt for testing: analyze feedback and create summary."

		// create test server that captures the request
		var capturedPrompt string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openai.ChatCompletionRequest
			json.NewDecoder(r.Body).Decode(&req)

			// capture the user message content
			for _, msg := range req.Messages {
				if msg.Role == openai.ChatMessageRoleUser {
					capturedPrompt = msg.Content
					break
				}
			}

			resp := openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Content: "Test summary",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := config.LLMConfig{
			Endpoint: server.URL + "/v1",
			APIKey:   "test-key",
			Model:    "gpt-4",
			Classification: config.ClassificationConfig{
				Prompts: config.ClassificationPrompts{
					GenerateSummary: customPrompt,
				},
			},
		}
		classifier := NewClassifier(cfg)

		feedback := []domain.FeedbackExample{
			{Title: "Test Article", Feedback: domain.FeedbackLike},
		}

		_, err := classifier.GeneratePreferenceSummary(context.Background(), feedback)
		require.NoError(t, err)

		// verify custom prompt was used
		assert.Contains(t, capturedPrompt, customPrompt)
	})

	t.Run("custom update summary prompt", func(t *testing.T) {
		customPrompt := "Custom update prompt: refine the summary with new data."

		// create test server that captures the request
		var capturedPrompt string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openai.ChatCompletionRequest
			json.NewDecoder(r.Body).Decode(&req)

			// capture the user message content
			for _, msg := range req.Messages {
				if msg.Role == openai.ChatMessageRoleUser {
					capturedPrompt = msg.Content
					break
				}
			}

			resp := openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Content: "Updated summary",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := config.LLMConfig{
			Endpoint: server.URL + "/v1",
			APIKey:   "test-key",
			Model:    "gpt-4",
			Classification: config.ClassificationConfig{
				Prompts: config.ClassificationPrompts{
					UpdateSummary: customPrompt,
				},
			},
		}
		classifier := NewClassifier(cfg)

		currentSummary := "Current summary"
		newFeedback := []domain.FeedbackExample{
			{Title: "New Article", Feedback: domain.FeedbackLike},
		}

		_, err := classifier.UpdatePreferenceSummary(context.Background(), currentSummary, newFeedback)
		require.NoError(t, err)

		// verify custom prompt was used
		assert.Contains(t, capturedPrompt, customPrompt)
	})
}

func TestClassifier_FullContentHandling(t *testing.T) {
	// create test server that captures the request
	var capturedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)

		// capture the request body
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)

		// capture the user prompt
		for _, msg := range req.Messages {
			if msg.Role == openai.ChatMessageRoleUser {
				capturedPrompt = msg.Content
				break
			}
		}

		// return mock response
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: `[{"guid": "item1", "score": 5.0, "explanation": "Test", "topics": ["test"], "summary": "Test summary"}]`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.LLMConfig{
		Endpoint:    server.URL + "/v1",
		APIKey:      "test-key",
		Model:       "gpt-4",
		Temperature: 0.3,
		MaxTokens:   500,
	}
	classifier := NewClassifier(cfg)

	t.Run("handle full content with multi-byte characters", func(t *testing.T) {
		// create article with long content including multi-byte characters
		content := strings.Repeat("a", 498) + "你好世界" + strings.Repeat("b", 1000) // long content with Chinese chars
		articles := []domain.Item{
			{
				GUID:        "item1",
				Title:       "Test Article",
				Description: "Test description",
				Content:     content,
			},
		}

		_, err := classifier.ClassifyItems(context.Background(), ClassifyRequest{
			Articles: articles,
		})
		require.NoError(t, err)

		// verify the full content is included (no truncation)
		assert.Contains(t, capturedPrompt, "Content: ")
		assert.Contains(t, capturedPrompt, content) // full content should be present
	})

	t.Run("handle emojis in full content", func(t *testing.T) {
		// create content with emojis
		content := strings.Repeat("x", 499) + "🚀🎉" + strings.Repeat("y", 1000) // long content with emojis
		articles := []domain.Item{
			{
				GUID:        "item1",
				Title:       "Test Article",
				Description: "Test description",
				Content:     content,
			},
		}

		_, err := classifier.ClassifyItems(context.Background(), ClassifyRequest{
			Articles: articles,
		})
		require.NoError(t, err)

		// verify the full content with emojis is included
		assert.Contains(t, capturedPrompt, "Content: ")
		assert.Contains(t, capturedPrompt, content) // full content including emojis should be present
	})
}

func TestClassifier_BatchProcessing(t *testing.T) {
	// create test server that returns batch classification response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)

		// return mock batch response
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: `[
  {
    "guid": "batch1",
    "score": 8.5,
    "explanation": "Relevant Go content",
    "topics": ["golang", "programming"],
    "summary": "Go 1.22 introduces range-over-function iterators and performance improvements."
  },
  {
    "guid": "batch2", 
    "score": 3.0,
    "explanation": "Not tech related",
    "topics": ["sports", "news"],
    "summary": "Football match results and player statistics from weekend games."
  },
  {
    "guid": "batch3",
    "score": 9.0,
    "explanation": "Excellent technical article",
    "topics": ["ai", "machine-learning"],
    "summary": "New transformer architecture achieves state-of-the-art results on language understanding benchmarks."
  }
]`,
					},
				},
			},
			Usage: openai.Usage{
				PromptTokens:     2500,
				CompletionTokens: 350,
				TotalTokens:      2850,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// create classifier with test server and batch configuration
	cfg := config.LLMConfig{
		Endpoint:    server.URL + "/v1",
		APIKey:      "test-key",
		Model:       "gpt-5",
		Temperature: 0.3,
		MaxTokens:   500,
		Classification: config.ClassificationConfig{
			BatchSize:    3,
			BatchTimeout: 5 * time.Second,
		},
	}
	classifier := NewClassifier(cfg)

	// test batch of articles
	articles := []domain.Item{
		{
			GUID:        "batch1",
			Title:       "Go 1.22 Released",
			Description: "New Go features",
			Content:     "Go 1.22 introduces exciting new features including range-over-function iterators and performance improvements...",
		},
		{
			GUID:        "batch2",
			Title:       "Sports News",
			Description: "Football results",
			Content:     "Manchester United vs Chelsea match results and statistics...",
		},
		{
			GUID:        "batch3",
			Title:       "AI Breakthrough",
			Description: "New transformer model",
			Content:     "Researchers develop new transformer architecture with improved performance on language understanding tasks...",
		},
	}

	// classify batch of articles
	ctx := context.Background()
	classifications, err := classifier.ClassifyItems(ctx, ClassifyRequest{
		Articles: articles,
	})
	require.NoError(t, err)

	// verify batch results
	assert.Len(t, classifications, 3)

	// check individual classifications
	classMap := make(map[string]domain.Classification)
	for _, class := range classifications {
		classMap[class.GUID] = class
	}

	assert.InEpsilon(t, 8.5, classMap["batch1"].Score, 0.001)
	assert.Contains(t, classMap["batch1"].Topics, "golang")
	assert.InEpsilon(t, 3.0, classMap["batch2"].Score, 0.001)
	assert.Contains(t, classMap["batch2"].Topics, "sports")
	assert.InEpsilon(t, 9.0, classMap["batch3"].Score, 0.001)
	assert.Contains(t, classMap["batch3"].Topics, "ai")
}

func TestClassifier_BatchConfiguration(t *testing.T) {
	t.Run("default batch configuration", func(t *testing.T) {
		cfg := config.LLMConfig{}
		classifier := NewClassifier(cfg)

		assert.Equal(t, 10, classifier.GetBatchSize())               // default
		assert.Equal(t, 5*time.Second, classifier.GetBatchTimeout()) // default
	})

	t.Run("custom batch configuration", func(t *testing.T) {
		cfg := config.LLMConfig{
			Classification: config.ClassificationConfig{
				BatchSize:    15,
				BatchTimeout: 10 * time.Second,
			},
		}
		classifier := NewClassifier(cfg)

		assert.Equal(t, 15, classifier.GetBatchSize())
		assert.Equal(t, 10*time.Second, classifier.GetBatchTimeout())
	})
}

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"gpt-5-preview", true},
		{"gpt-5-nano", true},
		{"GPT-5-NANO", true}, // case insensitive
		{"gpt-5", true},
		{"o1-preview", true},
		{"o1", true},
		{"O1-PREVIEW", true}, // case insensitive
		{"o3-mini", true},
		{"o4-turbo", true},
		{"gpt-4", false},
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"claude-3", false},
		{"llama3", false},
		{"custom-model-with-gpt-5-in-name", true}, // contains gpt-5
		{"", false},                               // empty model
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, isReasoningModel(tt.model))
		})
	}
}

func TestClassifier_GPT5_MaxCompletionTokens(t *testing.T) {
	// create test server that verifies correct token parameter
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)

		// for gpt-5 models, should use max_completion_tokens, not max_tokens
		assert.NotContains(t, reqBody, "max_tokens", "GPT-5 should not use max_tokens")
		assert.Contains(t, reqBody, "max_completion_tokens", "GPT-5 should use max_completion_tokens")
		assert.InDelta(t, 500, reqBody["max_completion_tokens"], 0.01)

		// reasoning models should not have temperature set
		assert.NotContains(t, reqBody, "temperature", "GPT-5 should not have temperature set")

		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: `{"classifications":[{"guid":"item1","score":8.0,"explanation":"Test","topics":["tech"],"summary":"Test summary"}]}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// test with GPT-5 model
	cfg := config.LLMConfig{
		Endpoint:    server.URL + "/v1",
		APIKey:      "test-key",
		Model:       "gpt-5-preview",
		Temperature: 0.7, // should be ignored
		MaxTokens:   500,
		Classification: config.ClassificationConfig{
			UseJSONMode: true,
		},
	}
	classifier := NewClassifier(cfg)

	articles := []domain.Item{{GUID: "item1", Title: "Test"}}

	ctx := context.Background()
	classifications, err := classifier.ClassifyItems(ctx, ClassifyRequest{Articles: articles})
	require.NoError(t, err)
	require.Len(t, classifications, 1)
}

func TestClassifier_RegularModel_MaxTokens(t *testing.T) {
	// create test server that verifies correct token parameter for regular models
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)

		// for regular models, should use max_tokens, not max_completion_tokens
		assert.Contains(t, reqBody, "max_tokens", "Regular models should use max_tokens")
		assert.NotContains(t, reqBody, "max_completion_tokens", "Regular models should not use max_completion_tokens")
		assert.InDelta(t, 500, reqBody["max_tokens"], 0.01)

		// regular models should have temperature set
		assert.Contains(t, reqBody, "temperature", "Regular models should have temperature set")
		assert.InDelta(t, 0.7, reqBody["temperature"], 0.01)

		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: `{"classifications":[{"guid":"item1","score":8.0,"explanation":"Test","topics":["tech"],"summary":"Test summary"}]}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// test with regular GPT-4 model
	cfg := config.LLMConfig{
		Endpoint:    server.URL + "/v1",
		APIKey:      "test-key",
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   500,
		Classification: config.ClassificationConfig{
			UseJSONMode: true,
		},
	}
	classifier := NewClassifier(cfg)

	articles := []domain.Item{{GUID: "item1", Title: "Test"}}

	ctx := context.Background()
	classifications, err := classifier.ClassifyItems(ctx, ClassifyRequest{Articles: articles})
	require.NoError(t, err)
	require.Len(t, classifications, 1)
}

func TestClassifier_ForbiddenPrefixHandling(t *testing.T) {
	callCount := 0
	// create test server that returns bad summaries on first call
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var content string
		if callCount == 1 {
			// first call returns summaries with forbidden prefixes
			content = `[
				{
					"guid": "item1",
					"score": 8.5,
					"explanation": "Relevant content",
					"topics": ["tech"],
					"summary": "The article discusses new features in Go 1.22 including range-over-function iterators."
				}
			]`
		} else {
			// retry returns corrected summary
			content = `[
				{
					"guid": "item1",
					"score": 8.5,
					"explanation": "Relevant content",
					"topics": ["tech"],
					"summary": "Go 1.22 introduces range-over-function iterators enabling cleaner iteration patterns."
				}
			]`
		}

		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: content,
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// create classifier with retry enabled
	cfg := config.LLMConfig{
		Endpoint:    server.URL + "/v1",
		APIKey:      "test-key",
		Model:       "gpt-4",
		Temperature: 0.3,
		MaxTokens:   500,
		Classification: config.ClassificationConfig{
			SummaryRetryAttempts: 2,
		},
	}
	classifier := NewClassifier(cfg)

	articles := []domain.Item{
		{
			GUID:  "item1",
			Title: "Go 1.22 Released",
		},
	}

	ctx := context.Background()
	classifications, err := classifier.ClassifyItems(ctx, ClassifyRequest{
		Articles: articles,
	})
	require.NoError(t, err)
	require.Len(t, classifications, 1)

	// should have retried and gotten good summary
	assert.Equal(t, 2, callCount, "should have made 2 calls (initial + 1 retry)")
	assert.Equal(t, "Go 1.22 introduces range-over-function iterators enabling cleaner iteration patterns.", classifications[0].Summary)
}

func TestClassifier_CleanSummary(t *testing.T) {
	cfg := config.LLMConfig{
		Classification: config.ClassificationConfig{
			ForbiddenSummaryPrefixes: []string{
				"The article discusses",
				"This post explores",
			},
		},
	}
	classifier := NewClassifier(cfg)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean forbidden prefix",
			input:    "The article discusses how Go 1.22 improves performance.",
			expected: "How Go 1.22 improves performance.",
		},
		{
			name:     "clean custom forbidden prefix",
			input:    "This post explores new features in Python.",
			expected: "New features in Python.",
		},
		{
			name:     "no change for good summary",
			input:    "Go 1.22 introduces new features.",
			expected: "Go 1.22 introduces new features.",
		},
		{
			name:     "handle empty summary",
			input:    "",
			expected: "",
		},
		{
			name:     "handle case insensitive",
			input:    "THE ARTICLE DISCUSSES important updates.",
			expected: "Important updates.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.cleanSummary(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClassifier_HasForbiddenPrefix(t *testing.T) {
	// test with default prefixes
	classifier := NewClassifier(config.LLMConfig{})

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "has forbidden prefix",
			input:    "The article discusses new features",
			expected: true,
		},
		{
			name:     "has another forbidden prefix",
			input:    "It explores the concept of",
			expected: true,
		},
		{
			name:     "no forbidden prefix",
			input:    "New features include improved performance",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "case insensitive check",
			input:    "THE ARTICLE DISCUSSES something",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.hasForbiddenPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClassifier_CustomForbiddenPrefixes(t *testing.T) {
	// test with custom prefixes
	cfg := config.LLMConfig{
		Classification: config.ClassificationConfig{
			ForbiddenSummaryPrefixes: []string{
				"In this article",
				"The study shows",
			},
		},
	}
	classifier := NewClassifier(cfg)

	assert.True(t, classifier.hasForbiddenPrefix("In this article we explore"))
	assert.True(t, classifier.hasForbiddenPrefix("The study shows that"))
	assert.False(t, classifier.hasForbiddenPrefix("The article discusses")) // not in custom list
	assert.False(t, classifier.hasForbiddenPrefix("Results indicate"))
}

func TestClassifier_Streaming(t *testing.T) {
	// SSE response covering the 2 articles from the classify prompt
	sseBody := `data: {"id":"cmpl-1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"["}}]}

data: {"id":"cmpl-1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"{\"guid\":\"item1\",\"score\":7,\"explanation\":\"ok\",\"topics\":[\"go\"],\"summary\":\"Go 1.22 ships range-over-func iterators with 50% faster compilation, better GC, and new toolchain management that simplifies Go version control across projects and CI pipelines today.\"}"}}]}

data: {"id":"cmpl-1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":",{\"guid\":\"item2\",\"score\":2,\"explanation\":\"no\",\"topics\":[\"sports\"],\"summary\":\"Manchester United beats Chelsea 3-1 with Bruno Fernandes scoring twice while Liverpool holds top spot after 2-2 draw with Arsenal at Emirates Stadium in Premier League weekend action.\"}]"}}]}

data: {"id":"cmpl-1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}

data: [DONE]

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		var body map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, true, body["stream"], "stream must be true when UseStreaming is set")

		w.Header().Set("Content-Type", "text/event-stream")
		_, err := w.Write([]byte(sseBody))
		assert.NoError(t, err)
	}))
	defer server.Close()

	cfg := config.LLMConfig{
		Endpoint:     server.URL + "/v1",
		APIKey:       "test-key",
		Model:        "gpt-test",
		Temperature:  0.3,
		MaxTokens:    100,
		UseStreaming: true,
	}
	classifier := NewClassifier(cfg)

	classifications, err := classifier.ClassifyItems(context.Background(), ClassifyRequest{
		Articles: []domain.Item{
			{GUID: "item1", Title: "Go 1.22 released"},
			{GUID: "item2", Title: "Football news"},
		},
	})
	require.NoError(t, err)
	require.Len(t, classifications, 2)
	assert.Equal(t, "item1", classifications[0].GUID)
	assert.InDelta(t, 7.0, classifications[0].Score, 0.01)
	assert.Equal(t, "item2", classifications[1].GUID)
}

func TestClassifier_Streaming_DefaultOff(t *testing.T) {
	// without UseStreaming, request must not set stream=true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		stream, _ := body["stream"].(bool)
		assert.False(t, stream)

		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{
				Message: openai.ChatCompletionMessage{Content: `[{"guid":"x","score":1,"explanation":"e","topics":["t"],"summary":"Short and punchy summary covering the main points without preamble and staying within the required 300-500 character window so the classifier accepts it."}]`},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := config.LLMConfig{
		Endpoint: server.URL + "/v1",
		APIKey:   "test-key",
		Model:    "gpt-test",
	}
	classifier := NewClassifier(cfg)

	_, err := classifier.ClassifyItems(context.Background(), ClassifyRequest{
		Articles: []domain.Item{{GUID: "x", Title: "t"}},
	})
	require.NoError(t, err)
}
