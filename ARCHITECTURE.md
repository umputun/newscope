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
   - extracted_rich_content (TEXT) - HTML formatted extracted content
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

**Performance Optimizations:**
- JSON operations using SQLite's json_each for efficient topic queries
- Pre-joined queries to minimize N+1 problems
- WAL mode for better concurrent read performance
- Batch operations for classification updates

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

Extracts full article content from web pages using go-trafilatura with rich HTML formatting support.

- **HTTPExtractor**: Main extractor implementation
  - Configurable timeout and user agent
  - Options: minimum text length, include images/links
  - Returns structured ExtractResult with metadata
  
- **ExtractResult**: Contains both plain text and rich HTML content
  - `Content`: Plain text version
  - `RichContent`: HTML formatted version with preserved structure
  - Supports common HTML tags (p, h1-h6, ul, ol, li, blockquote, strong, em, code, pre)
  - Automatic HTML escaping for security

### 5. LLM Classification (`pkg/llm`)

Uses OpenAI-compatible APIs to classify and score articles based on relevance with persistent learning through preference summaries.

**Key Features:**
- Support for any OpenAI-compatible endpoint (OpenAI, Ollama, etc.)
- Batch processing of multiple articles
- Feedback-based prompt enhancement
- Configurable system prompts
- JSON mode for structured responses
- Persistent preference learning through summaries

**Components:**
- **Classifier**: Main classification logic
  - Builds prompts with feedback examples and preference summaries
  - Handles batch classification
  - Parses LLM responses
  - Supports both JSON object and array response formats
  
- **Preference Summary System**: Learns from all historical feedback
  - Generates initial summary after 50 feedback items
  - Updates summary every 20 new feedback items
  - Maintains compressed knowledge of user preferences
  - Included in classification prompts for better accuracy

**Methods:**
- `ClassifyArticles()`: Standard classification with recent feedback
- `ClassifyArticlesWithSummary()`: Enhanced classification with preference summary
- `GeneratePreferenceSummary()`: Creates initial preference summary from feedback history
- `UpdatePreferenceSummary()`: Updates existing summary with new feedback patterns

**Data Flow:**
```
Articles + Preference Summary + Recent Feedback → Build Prompt → LLM API → Parse Response → Classifications
```

### 6. Scheduler (`pkg/scheduler`)

Manages periodic feed updates and content processing with a channel-based architecture and preference summary management.

**Key Features:**
- Single feed update interval with concurrent processing
- Channel-based item processing pipeline
- Combined extraction and classification in one step
- errgroup with concurrency limit for worker management
- Graceful shutdown with context cancellation
- On-demand operations (UpdateFeedNow, ExtractContentNow)
- Automatic preference summary generation and updates

**Workflow:**
1. **Preference Summary Management**: Check and update preference summary if needed
2. **Feed Updates**: Periodically fetch new articles from RSS feeds
3. **Channel Pipeline**: New items are sent to a processing channel
4. **Processing Worker**: Consumes items from channel, extracts content and classifies with preference summary
5. **Concurrent Processing**: Uses errgroup.SetLimit() for concurrency control

**Preference Summary Lifecycle:**
- Generates initial summary after 50 feedback items
- Updates summary every 20 new feedback items
- Stores summary and count in settings table
- Includes summary in all classification requests

**Interfaces** (defined by scheduler):
```go
type Database interface {
    GetFeed(ctx context.Context, id int64) (*db.Feed, error)
    GetFeeds(ctx context.Context, enabledOnly bool) ([]db.Feed, error)
    UpdateFeedFetched(ctx context.Context, feedID int64, nextFetch time.Time) error
    UpdateFeedError(ctx context.Context, feedID int64, errMsg string) error
    
    GetItem(ctx context.Context, id int64) (*db.Item, error)
    CreateItem(ctx context.Context, item *db.Item) error
    ItemExists(ctx context.Context, feedID int64, guid string) (bool, error)
    ItemExistsByTitleOrURL(ctx context.Context, title, url string) (bool, error)
    UpdateItemProcessed(ctx context.Context, itemID int64, content, richContent string, classification db.Classification) error
    UpdateItemExtraction(ctx context.Context, itemID int64, content, richContent string, err error) error
    GetRecentFeedback(ctx context.Context, feedbackType string, limit int) ([]db.FeedbackExample, error)
    GetTopics(ctx context.Context) ([]string, error)
    GetFeedbackCount(ctx context.Context) (int64, error)
    GetFeedbackSince(ctx context.Context, offset int64, limit int) ([]db.FeedbackExample, error)
    GetSetting(ctx context.Context, key string) (string, error)
    SetSetting(ctx context.Context, key, value string) error
}

type Classifier interface {
    ClassifyArticles(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string) ([]db.Classification, error)
    ClassifyArticlesWithSummary(ctx context.Context, articles []db.Item, feedbacks []db.FeedbackExample, canonicalTopics []string, preferenceSummary string) ([]db.Classification, error)
    GeneratePreferenceSummary(ctx context.Context, feedback []db.FeedbackExample) (string, error)
    UpdatePreferenceSummary(ctx context.Context, currentSummary string, newFeedback []db.FeedbackExample) (string, error)
}
```

### 7. Server (`server/`)

HTTP server providing REST API and web UI with server-side rendering.

**Interfaces** (defined by server):
```go
type Database interface {
    GetFeeds(ctx context.Context) ([]types.Feed, error)
    GetItems(ctx context.Context, limit, offset int) ([]types.Item, error)
    GetClassifiedItems(ctx context.Context, minScore float64, topic string, limit int) ([]types.ItemWithClassification, error)
    GetClassifiedItem(ctx context.Context, itemID int64) (*types.ItemWithClassification, error)
    UpdateItemFeedback(ctx context.Context, itemID int64, feedback string) error
    GetTopics(ctx context.Context) ([]string, error)
    GetAllFeeds(ctx context.Context) ([]db.Feed, error)
    CreateFeed(ctx context.Context, feed *db.Feed) error
    UpdateFeedStatus(ctx context.Context, feedID int64, enabled bool) error
    DeleteFeed(ctx context.Context, feedID int64) error
}

type Scheduler interface {
    UpdateFeedNow(ctx context.Context, feedID int64) error
    ExtractContentNow(ctx context.Context, itemID int64) error
    ClassifyNow(ctx context.Context) error
}
```

**Components:**
- **Server**: Main HTTP server using routegroup
  - Middleware support (recovery, throttling, auth)
  - JSON API endpoints
  - HTMX-based web UI with Go templates
  - Server-side rendering with no JavaScript required
  
- **DBAdapter**: Adapts database types to domain types
  - Converts SQL null types to Go types
  - Implements server's Database interface
  - Delegates all database operations to db package
  - Joins with feeds table for enriched data (FeedTitle field)
  - Supports filtering by score and topic
  - Efficient GetTopics using SQL json_each for unique topic extraction

**Web UI Features:**
- **Articles Page**: Main view showing classified articles
  - Score visualization with progress bars
  - Topic tags and filtering
  - Real-time score filtering with range slider
  - Like/dislike feedback buttons
  - Extracted content display with rich HTML formatting
  - HTMX for dynamic updates without page reload

- **Feeds Page**: Feed management interface
  - List all feeds with status indicators
  - Add new feeds with custom fetch intervals
  - Enable/disable feeds
  - Trigger immediate feed updates
  - Delete feeds
  - Error display for failed feeds

- **RSS Export**: Machine-readable RSS feeds
  - Filter by minimum score
  - Filter by topic
  - Uses proper XML encoding for safety
  - Includes article scores and topics in feed

**Current Endpoints:**
- `GET /` - Articles page (redirects to /articles)
- `GET /articles` - Main articles view with filtering
- `GET /feeds` - Feed management page
- `GET /settings` - Settings page (not implemented)
- `GET /rss` - RSS feed of classified articles (supports min_score parameter)
- `GET /rss/{topic}` - RSS feed filtered by topic

**API Endpoints:**
- `GET /api/v1/status` - Server status
- `POST /api/v1/feedback/{id}/{action}` - Submit user feedback (like/dislike)
- `POST /api/v1/extract/{id}` - Trigger content extraction and classification for an item
- `GET /api/v1/articles/{id}/content` - Get extracted content for display
- `POST /api/v1/feeds` - Create a new feed
- `POST /api/v1/feeds/{id}/enable` - Enable a feed
- `POST /api/v1/feeds/{id}/disable` - Disable a feed
- `POST /api/v1/feeds/{id}/fetch` - Trigger immediate feed fetch
- `DELETE /api/v1/feeds/{id}` - Delete a feed

**Templates:**
- `base.html` - Base layout with navigation
- `articles.html` - Articles listing with filters
- `article-card.html` - Reusable article card component
- `feeds.html` - Feed management page
- `feed-card.html` - Reusable feed card component

**Template Optimization:**
- Templates are pre-parsed at startup for better performance
- Page templates are cached separately to avoid naming conflicts
- Component templates can be reused across pages

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

### Feed Update and Processing Flow:

```
1. Scheduler triggers feed update
   └─> Parser fetches RSS feeds
       └─> New items saved to items table
       └─> Items sent to processing channel

2. Processing Pipeline (per item)
   └─> Extract article full text
   └─> Classify with LLM immediately
   └─> Store both results in single DB update

3. Web UI Display
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
8. **Performance First**: Pre-parsed templates, efficient SQL queries, proper indexing
9. **Type Safety**: Strong typing throughout, minimal interface{} usage

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
  update_interval: 30    # minutes
  max_workers: 5

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

## Recent Improvements

- **Rich Content Support**: Articles now preserve HTML formatting during extraction
- **RSS Feed Generation**: Proper XML encoding using encoding/xml package
- **Template Performance**: Pre-parsed templates at startup for faster rendering
- **Database Optimizations**: Efficient topic queries using SQLite json_each
- **Code Quality**: Reduced complexity, removed redundant code, improved type safety
- **Feed Management**: Complete CRUD operations for feeds via web UI

## Future Enhancements

- Multiple LLM provider support
- Fine-tuning support for open models
- Advanced prompt engineering options
- Real-time classification updates
- Export functionality for training data
- WebSub support for instant updates
- Multi-user support with personalized models
- OPML import/export for feed lists
- Keyboard shortcuts for power users
- Dark mode support