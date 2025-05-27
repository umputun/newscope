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
   - image_url (TEXT) - Featured image
   - content_hash (TEXT) - Hash for duplicate detection
   - language (TEXT) - Detected language code
   - content_extracted (BOOLEAN DEFAULT 0)
   - created_at, updated_at (DATETIME)
   - UNIQUE(feed_id, guid) - Prevent duplicates
   ```

3. **content** - Extracted full-text content
   ```sql
   - id (INTEGER PRIMARY KEY)
   - item_id (INTEGER UNIQUE, FK → items.id)
   - content (TEXT NOT NULL) - Extracted full text
   - text_content (TEXT) - Plain text version
   - language (TEXT) - Detected language
   - extracted_at (DATETIME) - Extraction timestamp
   - extractor (TEXT) - Extraction method used
   - error (TEXT) - Extraction error if any
   ```

4. **classifications** - LLM-based article classifications
   ```sql
   - id (INTEGER PRIMARY KEY)
   - guid (TEXT NOT NULL) - Article GUID
   - score (REAL NOT NULL) - Relevance score (0-10)
   - explanation (TEXT) - Brief explanation of score
   - topics (JSON) - Array of relevant topics
   - classified_at (DATETIME) - Classification timestamp
   - model_name (TEXT) - LLM model used
   - UNIQUE(guid) - One classification per article
   ```

5. **feedback** - User feedback for training
   ```sql
   - id (INTEGER PRIMARY KEY)
   - guid (TEXT NOT NULL) - Article GUID
   - title (TEXT NOT NULL) - Article title for reference
   - feedback (TEXT NOT NULL) - 'like' or 'dislike'
   - topics (JSON) - User-provided topics
   - created_at (DATETIME) - Feedback timestamp
   ```

6. **settings** - Key-value configuration store
   ```sql
   - key (TEXT PRIMARY KEY)
   - value (TEXT NOT NULL)
   - created_at, updated_at (DATETIME)
   ```

#### Indexes and Performance

**Strategic Indexes:**
```sql
- idx_feeds_next_fetch ON feeds(next_fetch) - Scheduler queries
- idx_items_feed_id ON items(feed_id) - Feed-item lookups
- idx_items_published ON items(published DESC) - Chronological queries
- idx_items_content_extracted ON items(content_extracted) - Extraction queue
- idx_classifications_score ON classifications(score DESC) - High-scoring articles
- idx_feedback_created ON feedback(created_at DESC) - Recent feedback
```

**Full-Text Search:**
```sql
CREATE VIRTUAL TABLE items_fts USING fts5(
    title, description, content
);

-- Triggers to maintain FTS index
CREATE TRIGGER items_fts_insert AFTER INSERT ON items
CREATE TRIGGER items_fts_update AFTER UPDATE ON items  
CREATE TRIGGER items_fts_delete AFTER DELETE ON items
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

**Workflow:**
1. **Feed Updates**: Fetch new articles from RSS feeds
2. **Content Extraction**: Extract full text from article URLs
3. **Classification**: Send articles to LLM for scoring
4. **Storage**: Save classifications to database

### 7. Server (`server/`)

HTTP server providing REST API and web UI.

**Components:**
- **Server**: Main HTTP server using routegroup
  - Middleware support (recovery, throttling, auth)
  - JSON API endpoints
  - HTMX-based web UI with Go templates
  
- **DBAdapter**: Adapts database types to domain types
  - Converts SQL null types to Go types
  - Implements server's Database interface

**Current Endpoints:**
- `GET /` - Web UI home page
- `GET /api/v1/items` - List classified items with scores
- `GET /api/v1/feeds` - List configured feeds
- `POST /api/v1/feedback` - Submit user feedback
- `GET /api/v1/config` - Get current configuration
- `POST /api/v1/config` - Update configuration

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
       └─> New items saved to database

2. Content extraction (if enabled)
   └─> Extractor fetches article full text
       └─> Content saved to database

3. LLM Classification
   └─> Batch articles for classification
   └─> Include recent feedback examples
   └─> Send to LLM API
   └─> Parse scores and topics
   └─> Save classifications

4. User Interface
   └─> Display articles sorted by score
   └─> Show explanation and topics
   └─> Collect user feedback
```

### Classification Process:

```
Articles → Build Prompt with:
  - Article title, description, content
  - Previous user feedback examples
  - System prompt instructions
→ LLM API Call
→ Response: Score (0-10), Explanation, Topics
→ Store in classifications table
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