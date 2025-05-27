package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/db"
)

func TestClassifier_ClassifyArticles(t *testing.T) {
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
    "topics": ["golang", "programming", "backend"]
  },
  {
    "guid": "item2", 
    "score": 3.0,
    "explanation": "Not relevant to tech interests",
    "topics": ["sports", "news"]
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
	articles := []db.Item{
		{
			GUID:             "item1",
			Title:            "Go 1.22 Released",
			Description:      "New features in Go",
			ExtractedContent: "Go 1.22 brings exciting new features...",
		},
		{
			GUID:        "item2",
			Title:       "Sports News",
			Description: "Latest football results",
		},
	}

	// test feedback examples
	feedback := []db.FeedbackExample{
		{
			Title:    "Previous Go Article",
			Feedback: "like",
			Topics:   []string{"golang"},
		},
	}

	// classify articles
	ctx := context.Background()
	classifications, err := classifier.ClassifyArticles(ctx, articles, feedback)
	require.NoError(t, err)
	require.Len(t, classifications, 2)

	// check first classification
	assert.Equal(t, "item1", classifications[0].GUID)
	assert.InEpsilon(t, 8.5, classifications[0].Score, 0.001)
	assert.Equal(t, "Highly relevant Go programming content", classifications[0].Explanation)
	assert.Equal(t, []string{"golang", "programming", "backend"}, classifications[0].Topics)

	// check second classification
	assert.Equal(t, "item2", classifications[1].GUID)
	assert.InEpsilon(t, 3.0, classifications[1].Score, 0.001)
	assert.Equal(t, "Not relevant to tech interests", classifications[1].Explanation)
	assert.Equal(t, []string{"sports", "news"}, classifications[1].Topics)
}

func TestClassifier_ClassifyArticles_EmptyInput(t *testing.T) {
	cfg := config.LLMConfig{
		APIKey: "test-key",
		Model:  "gpt-4",
	}
	classifier := NewClassifier(cfg)

	ctx := context.Background()
	classifications, err := classifier.ClassifyArticles(ctx, []db.Item{}, nil)
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

	articles := []db.Item{
		{
			GUID:             "item1",
			Title:            "Test Article",
			Description:      "Test description",
			ExtractedContent: "Long content that should be truncated " + strings.Repeat("x", 500),
		},
	}

	feedback := []db.FeedbackExample{
		{
			Title:    "Liked Article",
			Feedback: "like",
			Topics:   []string{"tech", "ai"},
		},
		{
			Title:    "Disliked Article",
			Feedback: "dislike",
		},
	}

	prompt := classifier.buildPrompt(articles, feedback)

	// check feedback section
	assert.Contains(t, prompt, "Based on user feedback:")
	assert.Contains(t, prompt, "like article: Liked Article")
	assert.Contains(t, prompt, "Topics: tech, ai")
	assert.Contains(t, prompt, "dislike article: Disliked Article")

	// check articles section
	assert.Contains(t, prompt, "Classify these articles:")
	assert.Contains(t, prompt, "GUID: item1")
	assert.Contains(t, prompt, "Title: Test Article")
	assert.Contains(t, prompt, "Description: Test description")
	assert.Contains(t, prompt, "Content: Long content")
	assert.Contains(t, prompt, "...")

	// check instruction
	assert.Contains(t, prompt, "Respond with a JSON array")
}

func TestClassifier_parseResponse(t *testing.T) {
	classifier := &Classifier{config: config.LLMConfig{}}

	articles := []db.Item{
		{GUID: "item1"},
		{GUID: "item2"},
		{GUID: "item3"},
	}

	tests := []struct {
		name        string
		response    string
		wantErr     bool
		wantCount   int
		checkResult func(t *testing.T, classifications []db.Classification)
	}{
		{
			name: "valid json array",
			response: `[
				{"guid": "item1", "score": 7.5, "explanation": "Good", "topics": ["tech"]},
				{"guid": "item2", "score": 2.0, "explanation": "Bad", "topics": []}
			]`,
			wantCount: 2,
			checkResult: func(t *testing.T, classifications []db.Classification) {
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
			checkResult: func(t *testing.T, classifications []db.Classification) {
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
			checkResult: func(t *testing.T, classifications []db.Classification) {
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

func TestClassifier_JSONMode(t *testing.T) {
	t.Run("build prompt with JSON mode", func(t *testing.T) {
		classifier := &Classifier{
			config: config.LLMConfig{
				Classification: config.ClassificationConfig{
					UseJSONMode: true,
				},
			},
		}

		articles := []db.Item{{GUID: "item1", Title: "Test"}}
		prompt := classifier.buildPrompt(articles, nil)
		
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

		articles := []db.Item{{GUID: "item1", Title: "Test"}}
		prompt := classifier.buildPrompt(articles, nil)
		
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

		articles := []db.Item{
			{GUID: "item1"},
			{GUID: "item2"},
		}

		response := `{
			"classifications": [
				{"guid": "item1", "score": 8, "explanation": "Good", "topics": ["tech"]},
				{"guid": "item2", "score": 3, "explanation": "Bad", "topics": ["other"]}
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
			Endpoint:    server.URL + "/v1",
			APIKey:      "test-key",
			Model:       "gpt-4",
			Classification: config.ClassificationConfig{
				UseJSONMode: true,
			},
		}
		classifier := NewClassifier(cfg)

		articles := []db.Item{{GUID: "item1", Title: "Test"}}
		classifications, err := classifier.ClassifyArticles(context.Background(), articles, nil)
		
		require.NoError(t, err)
		require.Len(t, classifications, 1)
		assert.Equal(t, "item1", classifications[0].GUID)
		assert.InEpsilon(t, 9.0, classifications[0].Score, 0.001)
	})
}