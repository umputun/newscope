# Newscope: LLM-Powered RSS Aggregator - Simplified Design

## Executive Summary

Newscope is a personal RSS aggregator that uses LLM intelligence to filter and rank articles based on user interests and feedback. By leveraging external LLM APIs (OpenAI-compatible), we eliminate complex ML infrastructure while providing superior content classification with natural language understanding.

## Core Principles

1. **Simplicity First** - Minimal code, maximum intelligence
2. **LLM-Powered** - Delegate complex decisions to language models
3. **User Feedback Loop** - Learn from likes/dislikes without training
4. **Explainable** - Always show why content was recommended
5. **Privacy-Focused** - All data stays local, only API calls to LLM

## System Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Feed Fetcher  │────▶│Content Extractor│────▶│ LLM Classifier  │────▶│   Web UI        │
│   (Scheduled)   │     │ (Trafilatura)   │     │ (OpenAI API)    │     │   (HTMX)        │
└─────────────────┘     └─────────────────┘     └─────────────────┘     └─────────────────┘
         │                       │                         │                         │
         └───────────────────────┴─────────────────────────┴─────────────────────────┘
                                                │
                                  ┌────────────────────────┐
                                  │   SQLite Database      │
                                  │ (Minimal Schema)       │
                                  └────────────────────────┘
```

## Database Schema (Minimal)

```sql
-- Feed sources
CREATE TABLE feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT,
    description TEXT,
    last_fetched DATETIME,
    next_fetch DATETIME,
    fetch_interval INTEGER DEFAULT 1800, -- 30 minutes
    error_count INTEGER DEFAULT 0,
    last_error TEXT,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Articles with LLM classification
CREATE TABLE items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL,
    guid TEXT NOT NULL,
    title TEXT NOT NULL,
    link TEXT NOT NULL,
    description TEXT,
    content TEXT,        -- Original RSS content
    author TEXT,
    published DATETIME,
    
    -- Extracted content
    extracted_content TEXT,   -- Full article text
    extracted_at DATETIME,
    extraction_error TEXT,
    
    -- LLM classification results
    relevance_score REAL,     -- 0-10 score from LLM
    explanation TEXT,         -- Why this score
    topics JSON,             -- Detected topics/tags
    classified_at DATETIME,
    
    -- User feedback
    user_feedback TEXT,      -- 'like', 'dislike', 'spam', null
    feedback_at DATETIME,
    
    -- Metadata
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(feed_id, guid),
    FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);

-- User preferences and settings
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX idx_items_published ON items(published DESC);
CREATE INDEX idx_items_score ON items(relevance_score DESC);
CREATE INDEX idx_items_feedback ON items(user_feedback, feedback_at DESC);
CREATE INDEX idx_feeds_next ON feeds(next_fetch);
```

## Configuration

```yaml
# config.yml
app:
  name: "Newscope"
  version: "2.0.0"
  
server:
  listen: ":8080"
  read_timeout: "30s"
  write_timeout: "30s"

database:
  path: "./data/newscope.db"

llm:
  # OpenAI-compatible endpoint (OpenAI, Ollama, etc)
  endpoint: "http://localhost:11434/v1"  # Ollama example
  # endpoint: "https://api.openai.com/v1" # OpenAI example
  
  api_key: "${OPENAI_API_KEY}"  # From environment
  model: "llama3"                # or "gpt-4o-mini"
  temperature: 0.3               # Lower = more consistent
  max_tokens: 500
  timeout: "30s"
  
  # Classification settings
  classification:
    feedback_examples: 10      # Recent examples to include
    batch_size: 5             # Articles to classify at once

fetcher:
  workers: 3
  timeout: "30s"
  user_agent: "Newscope/2.0"

extractor:
  timeout: "30s"
  min_content_length: 100
  include_links: false
  
feeds:
  - url: "https://news.ycombinator.com/rss"
    name: "Hacker News"
    interval: "30m"
    
  - url: "https://lobste.rs/rss"
    name: "Lobsters"  
    interval: "1h"
```

## LLM Classification System

### Initial Setup

User provides their interests as natural language:
```
I'm interested in:
- Go programming and practical tutorials
- Database internals and optimization
- Distributed systems and architecture
- Open source projects

I want to avoid:
- Blockchain and cryptocurrency
- Political news
- Marketing and growth hacking
- Abstract theory without practical application
```

### Processing Pipeline

1. **Fetch new articles** from RSS feeds
2. **Extract full content** using Trafilatura
3. **Batch classify** extracted content with LLM (5-10 at a time)
4. **Include recent feedback** in prompt for personalization
5. **Store results** with score and explanation
6. **Display** articles above threshold

### Classification Prompt Template

```
You are a personal content curator. Score each article from 0-10 based on relevance.

USER INTERESTS:
{{interests}}

RECENTLY LIKED ARTICLES:
{{recent_likes}}

RECENTLY DISLIKED ARTICLES:
{{recent_dislikes}}

ARTICLES TO CLASSIFY:
[
  {
    "guid": "article1_guid",
    "title": "Article Title",
    "description": "RSS description",
    "content": "First 500 words of extracted content..."
  },
  ...
]

For each article, return:
{
  "guid": "article_guid",
  "score": 0-10,
  "explanation": "Brief reason for score",
  "topics": ["detected", "topics"]
}

Consider:
- Direct interest matches score higher
- Learn from feedback patterns
- Quality indicators (detailed content vs clickbait)
- Practical vs theoretical content
```

### Feedback Learning

```go
type FeedbackExample struct {
    Title       string
    Description string
    Feedback    string    // like/dislike
    Topics      []string  // From previous classification
}

func buildPromptWithFeedback(interests string, likes, dislikes []FeedbackExample) string {
    // Format recent feedback examples
    // Include patterns LLM should learn
}
```

## Implementation Phases

### Phase 1: Core Pipeline (Week 1)
- [x] Basic project structure
- [ ] Minimal database schema
- [ ] Feed fetcher (reuse existing)
- [ ] LLM client wrapper
- [ ] Basic classification flow
- [ ] CLI for testing

### Phase 2: Web Interface (Week 2)
- [ ] Article list with scores
- [ ] Feedback buttons (like/dislike/spam)
- [ ] Settings page for interests
- [ ] Feed management
- [ ] HTMX interactivity

### Phase 3: Optimization (Week 3)
- [ ] Batch classification
- [ ] Response caching
- [ ] Rate limiting
- [ ] Error handling
- [ ] Performance tuning

### Phase 4: Polish (Week 4)
- [ ] Explanation display
- [ ] Topic filtering
- [ ] RSS feed generation
- [ ] Docker packaging
- [ ] Documentation

## API Design

### REST Endpoints

```
# Articles
GET  /api/articles?limit=50&min_score=5.0
GET  /api/article/:id
POST /api/article/:id/feedback  {"feedback": "like|dislike|spam"}

# Feeds
GET  /api/feeds
POST /api/feeds                  {"url": "...", "name": "..."}
DELETE /api/feed/:id

# Settings
GET  /api/settings/interests
PUT  /api/settings/interests     {"interests": "..."}

# System
POST /api/classify/all          # Trigger classification
GET  /api/stats                 # Feedback stats
```

### Web Pages

```
GET  /                          # Article list
GET  /article/:id              # Read article
GET  /settings                 # Manage interests & feeds
GET  /feeds                    # Feed management
```

## Code Structure

```
newscope/
├── cmd/
│   └── newscope/
│       └── main.go           # Entry point
├── pkg/
│   ├── config/              # Configuration
│   │   └── config.go
│   ├── repository/          # Repository layer
│   │   ├── db.go
│   │   ├── schema.sql
│   │   └── models.go        # Simple structs
│   ├── feed/                # RSS fetching (reuse)
│   │   └── fetcher.go
│   ├── content/             # Content extraction (reuse)
│   │   └── extractor.go    # Trafilatura wrapper
│   ├── classifier/          # LLM integration
│   │   ├── llm.go          # OpenAI client wrapper
│   │   └── classifier.go   # Classification logic
│   └── web/                # HTTP server
│       ├── server.go
│       ├── handlers.go
│       └── templates/      # HTMX templates
├── config.yml
└── go.mod
```

## Key Implementation Details

### LLM Client Wrapper
```go
type LLMClient struct {
    client      *openai.Client
    model       string
    temperature float64
}

func (c *LLMClient) ClassifyBatch(ctx context.Context, articles []Article, prompt string) ([]Classification, error) {
    // Build prompt
    // Call LLM
    // Parse JSON response
    // Handle errors gracefully
}
```

### Classifier
```go
type Classifier struct {
    llm       *LLMClient
    db        *DB
    interests string
}

func (c *Classifier) ClassifyPending(ctx context.Context) error {
    // Get articles with extracted content but no classification
    articles, err := c.db.GetUnclassifiedArticles(100)
    
    // Get recent feedback for context
    likes, dislikes := c.db.GetRecentFeedback(10)
    
    // Process in batches
    for i := 0; i < len(articles); i += 5 {
        batch := articles[i:min(i+5, len(articles))]
        
        // Include first 500 words of content for each
        prompt := c.buildPrompt(batch, likes, dislikes)
        
        // Classify via LLM
        results := c.llm.ClassifyBatch(ctx, prompt)
        
        // Store results
        c.db.UpdateClassifications(results)
    }
}
```

### Feedback Handler
```go
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
    // Parse feedback
    // Store in database
    // Return updated article HTML (HTMX)
}
```

## Advantages of This Approach

1. **Drastically Simplified**
   - ~80% less code than ML-based solution
   - No training pipelines or model management
   - Single SQLite table for core functionality

2. **Better Classification**
   - LLMs understand context and nuance
   - Natural language interest definition
   - Explains reasoning in plain English

3. **Instant Personalization**
   - Feedback affects next classification immediately
   - No retraining or feature engineering
   - Adapts to changing interests

4. **Flexible Deployment**
   - Use Ollama for fully local/private setup
   - Use OpenAI for best quality
   - Easy to switch between providers

5. **Maintainable**
   - Mostly configuration and prompting
   - Standard web technologies (Go + HTMX)
   - No ML expertise required

## Cost Considerations

### Local (Ollama)
- **Free** after initial setup
- Requires ~8GB RAM for good models
- Slightly slower but private

### Cloud (OpenAI)
- ~$0.001 per article with GPT-4o-mini
- 1000 articles/day = ~$30/month
- Faster and more accurate

## Future Enhancements

1. **Smart Summarization** - LLM-generated summaries
2. **Topic Clustering** - Group similar articles
3. **Digest Generation** - Daily/weekly email summaries
4. **Multi-user** - Separate preferences per user
5. **Advanced Filtering** - Temporary interest boosts

## Success Metrics

- Classification accuracy > 80% after feedback
- Response time < 500ms for cached content  
- LLM API calls < $50/month for typical use
- User feedback rate > 10% (indicates engagement)
- False positive rate < 5% after tuning

This simplified approach delivers a better product with less complexity. The LLM handles the hard parts, we just orchestrate.