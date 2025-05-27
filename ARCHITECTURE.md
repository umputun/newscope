# Newscope Architecture

## Overview

Newscope is a modern RSS feed aggregator that uses LLM-based classification to identify and rank relevant articles. The system follows a modular architecture with clear separation of concerns, using interfaces defined on the consumer side and dependency injection for testability.

## Core Components

### 1. Configuration (`pkg/config`)

Manages all application configuration through YAML files with schema validation.

- **Config**: Main configuration struct containing all subsystems
- **Feed**: Individual feed configuration (URL, name, update interval)
- **ExtractionConfig**: Content extraction settings (timeout, concurrency, options)
- **LLMConfig**: LLM API configuration for classification
- **ClassificationConfig**: Classification-specific settings (score threshold, batch size, JSON mode)
- **ServerConfig**: HTTP server settings (address, timeouts)
- **Verify()**: Validates configuration against JSON schema

```
config.yaml → Config struct → Validation → Application components
```

### 2. Database Layer (`pkg/db`)

SQLite-based persistence using pure Go driver (modernc.org/sqlite) with sqlx for query building.

**Core Characteristics:**
- Pure Go SQLite driver (no CGO dependency)
- WAL mode for concurrent readers
- Foreign key constraints enforced
- Full-text search with FTS5
- Automatic schema initialization and migrations

#### Storage Schema

**Primary Tables:**

1. **feeds** - RSS/Atom feed sources
   ```sql
   - id (INTEGER PRIMARY KEY)
   - url (TEXT UNIQUE NOT NULL) - Feed URL
   - title (TEXT) - Feed title
   - description (TEXT) - Feed description
   - last_fetched (DATETIME) - Last successful fetch
   - next_fetch (DATETIME) - Scheduled next fetch
   - fetch_interval (INTEGER DEFAULT 1800) - Seconds between fetches
   - last_error (TEXT) - Last fetch error message
   - error_count (INTEGER DEFAULT 0) - Consecutive error count
   - priority (INTEGER DEFAULT 1) - Fetch priority
   - enabled (BOOLEAN DEFAULT 1) - Feed active status
   - created_at, updated_at (DATETIME)
   ```

2. **items** - Individual articles/posts
   ```sql
   - id (INTEGER PRIMARY KEY)
   - feed_id (INTEGER NOT NULL, FK → feeds.id)
   - guid (TEXT NOT NULL) - Unique identifier from feed
   - title (TEXT NOT NULL) - Article title
   - link (TEXT NOT NULL) - Article URL
   - description (TEXT) - Summary/excerpt
   - content (TEXT) - Original content from feed
   - author (TEXT) - Author name
   - published (DATETIME) - Publication date
   - extracted_content (TEXT) - Full extracted article text
   - extracted_at (DATETIME) - When content was extracted
   - extraction_error (TEXT) - Any extraction error
   - relevance_score (REAL DEFAULT 0) - LLM score (0-10)
   - explanation (TEXT) - LLM explanation for score
   - topics (JSON) - Topics identified by LLM
   - classified_at (DATETIME) - When classified
   - user_feedback (TEXT) - 'like', 'dislike', 'spam'
   - feedback_at (DATETIME) - When feedback given
   - created_at, updated_at (DATETIME)
   - UNIQUE(feed_id, guid) - Prevent duplicates
   ```

3. **settings** - Key-value configuration store
   ```sql
   - key (TEXT PRIMARY KEY)
   - value (TEXT NOT NULL)
   - created_at, updated_at (DATETIME)
   ```

#### Indexes and Performance

**Strategic Indexes:**
```sql
- idx_items_feed_published ON items(feed_id, published DESC)
- idx_items_classification ON items(classified_at, relevance_score DESC)
- idx_items_feedback ON items(feedback_at DESC)
- idx_items_extraction ON items(extracted_at)
```

### 3. Feed System (`pkg/feed`)

Handles RSS/Atom feed parsing and management.

**Components:**
- **Parser**: Wraps gofeed library for feed parsing
  - Configurable timeout
  - Converts external feed format to internal types
  
- **Fetcher**: Retrieves and parses individual feeds
  - HTTP client with timeout
  - Error handling and retry logic
  
- **Manager**: Orchestrates multiple feed operations
  - Concurrent feed fetching
  - Item deduplication
  - Integration with database storage

**Interfaces** (defined by consumers):
```go
// In manager.go
type ConfigProvider interface {
    GetFeeds() []config.Feed
    GetExtractionConfig() config.ExtractionConfig
}

type Fetcher interface {
    Fetch(ctx context.Context, feedURL, feedName string) ([]types.Item, error)
}
```

### 4. Content Extraction (`pkg/content`)

Extracts full article content from web pages using go-trafilatura.

- **HTTPExtractor**: Main extractor implementation
  - Configurable timeout and user agent
  - Options: minimum text length, include images/links
  - Returns structured ExtractResult with metadata
  
- **ExtractResult**: Contains extracted text, metadata, and errors

### 5. LLM Classification (`pkg/llm`)

Uses OpenAI-compatible APIs to classify and score articles based on relevance.

**Key Features:**
- Support for any OpenAI-compatible endpoint (OpenAI, Ollama, etc.)
- Batch processing of multiple articles
- Feedback-based prompt enhancement
- Configurable system prompts
- JSON mode for structured responses

**Components:**
- **Classifier**: Main classification logic
  - Builds prompts with feedback examples
  - Handles batch classification
  - Parses LLM responses
  - Supports both JSON object and array response formats

**Data Flow:**
```
Articles + Feedback → Build Prompt → LLM API → Parse Response → Classifications
```

### 6. Scheduler (`pkg/scheduler`)

Manages periodic feed updates, content extraction, and classification with worker pools.

**Key Features:**
- Separate intervals for feed updates, extraction, and classification
- Configurable worker pool sizes
- Rate limiting support
- Graceful shutdown with context cancellation
- LLM classification integration
- On-demand operations (UpdateFeedNow, ExtractContentNow, ClassifyNow)

**Workflow:**
1. **Feed Updates**: Fetch new articles from RSS feeds
2. **Content Extraction**: Extract full text from article URLs
3. **Classification**: Send articles to LLM for scoring
4. **Storage**: Save classifications to database

**Interfaces** (defined by consumers):
```go
type Database interface {
    GetFeedsToFetch(ctx context.Context, limit int) ([]db.Feed, error)
    GetItemsNeedingExtraction(ctx context.Context, limit int) ([]db.Item, error)
    GetUnclassifiedItems(ctx context.Context, limit int) ([]db.Item, error)
    GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error)
    UpdateClassifications(ctx context.Context, classifications []db.Classification, itemsByGUID map[string]int64) error
    // ... and more
}

type Classifier interface {
    ClassifyArticles(ctx context.Context, items []db.Item, feedbacks []db.FeedbackExample) ([]db.Classification, error)
}
```

### 7. Server (`server/`)

HTTP server providing REST API and web UI with server-side rendering.

**Components:**
- **Server**: Main HTTP server using routegroup
  - Middleware support (recovery, throttling, auth)
  - JSON API endpoints
  - HTMX-based web UI with Go templates
  - Server-side rendering with no JavaScript required
  
- **DBAdapter**: Adapts database types to domain types
  - Converts SQL null types to Go types
  - Implements server's Database interface
  - Joins with feeds table for enriched data
  - Supports filtering by score and topic

**Web UI Features:**
- **Articles Page**: Main view showing classified articles
  - Score visualization with progress bars
  - Topic tags and filtering
  - Real-time score filtering with range slider
  - Like/dislike feedback buttons
  - Extracted content display
  - HTMX for dynamic updates without page reload

**Current Endpoints:**
- `GET /` - Articles page (redirects to /articles)
- `GET /articles` - Main articles view with filtering
- `GET /feeds` - Feed management (planned)
- `GET /settings` - Settings page (planned)

**API Endpoints:**
- `GET /api/v1/status` - Server status
- `POST /api/v1/feedback/{id}/{action}` - Submit user feedback (like/dislike)
- `POST /api/v1/extract/{id}` - Trigger content extraction for an item
- `POST /api/v1/classify-now` - Trigger immediate classification
- `GET /api/v1/articles/{id}/content` - Get extracted content for display

**Templates:**
- `base.html` - Base layout with navigation
- `articles.html` - Articles listing with filters
- `article-card.html` - Reusable article card component

### 8. Main Application (`cmd/newscope`)

Entry point that wires all components together.

**Initialization Flow:**
1. Load and validate configuration
2. Initialize database with migrations
3. Create parser and content extractor
4. Initialize LLM classifier
5. Initialize scheduler with all components
6. Create server with database adapter
7. Start scheduler and server
8. Handle graceful shutdown on signals

## Data Flow

### Feed Update and Classification Flow:

```
1. Scheduler triggers feed update
   └─> Parser fetches RSS feeds
       └─> New items saved to items table

2. Content extraction (if enabled)
   └─> Extractor fetches article full text
       └─> Updates items.extracted_content field

3. LLM Classification
   └─> Batch unclassified articles
   └─> Include recent user feedback as examples
   └─> Send to LLM API (with JSON mode if supported)
   └─> Parse scores, explanations, and topics
   └─> Update items table with classification data

4. Web UI Display
   └─> Query items with JOIN on feeds for feed names
   └─> Filter by minimum score and topics
   └─> Display with HTMX for dynamic updates
   └─> Collect user feedback (like/dislike)
```

### User Interaction Flow:

```
User browses articles → Adjusts score filter → HTMX updates list
                    ↓
           Clicks "Show Content" → AJAX loads extracted text
                    ↓
           Provides feedback → Updates item → Re-renders card
                    ↓
      Feedback used in future classifications
```

### Classification Process:

```
Unclassified Items → Build Batch Prompt:
  - System prompt with instructions
  - Recent user feedback examples (likes/dislikes)
  - Article data (title, description, extracted content)
→ LLM API Call (JSON mode if available)
→ Parse Response:
  - Score (0-10)
  - Brief explanation
  - List of topics
→ Update items table directly
```

## Design Principles

1. **Interface Segregation**: Interfaces are defined on the consumer side, not the producer
2. **Dependency Injection**: All components receive dependencies through constructors
3. **Context Propagation**: All operations support context for cancellation and timeouts
4. **Error Handling**: Explicit error returns, no panics in library code
5. **Testability**: Mock generation with moq, high test coverage
6. **Pure Go**: No CGO dependencies for easy cross-compilation
7. **LLM Flexibility**: Support for any OpenAI-compatible API

## Configuration Example

```yaml
llm:
  endpoint: "https://api.openai.com/v1"
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-4o-mini"
  temperature: 0.3
  max_tokens: 500
  classification:
    score_threshold: 5.0
    feedback_examples: 10
    batch_size: 5
    use_json_mode: true

scheduler:
  feed_update_interval: 30m
  extraction_interval: 5m
  classification_interval: 10m
  workers: 5

extraction:
  enabled: true
  timeout: 30s
  max_concurrent: 5
```

## Testing Strategy

- **Unit Tests**: Each package has comprehensive tests
- **Mock Generation**: Using moq with go:generate directives
- **Integration Tests**: Database tests with in-memory SQLite
- **Table-Driven Tests**: Preferred testing pattern
- **LLM Tests**: Mock HTTP servers for API testing

## Security Considerations

- API keys stored in environment variables
- SQL injection prevention through parameterized queries
- Rate limiting on API endpoints
- Input validation and sanitization
- Secure defaults in configuration

## Future Enhancements

- Multiple LLM provider support
- Fine-tuning support for open models
- Advanced prompt engineering options
- Real-time classification updates
- Export functionality for training data
- WebSub support for instant updates
- Multi-user support with personalized models