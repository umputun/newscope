# Newscope Architecture

## Overview

Newscope is an RSS feed aggregator with content extraction capabilities. The system follows a modular architecture with clear separation of concerns, using interfaces defined on the consumer side and dependency injection for testability.

## Core Components

### 1. Configuration (`pkg/config`)

Manages all application configuration through YAML files with schema validation.

- **Config**: Main configuration struct containing all subsystems
- **Feed**: Individual feed configuration (URL, name, update interval)
- **ExtractionConfig**: Content extraction settings (timeout, concurrency, options)
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
   - metadata (JSON) - Additional feed metadata
   - enabled (BOOLEAN DEFAULT 1) - Feed active status
   - avg_score (REAL) - Average article score
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
   - categories (JSON) - Array of category strings
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
   - extractor_meta (JSON) - Extractor metadata
   - error (TEXT) - Extraction error if any
   ```

4. **categories** - Classification system
   ```sql
   - id (INTEGER PRIMARY KEY)
   - name (TEXT UNIQUE NOT NULL) - Category name
   - keywords (JSON) - Array of keywords
   - is_positive (BOOLEAN DEFAULT 1) - Sentiment indicator
   - weight (REAL DEFAULT 1.0) - Category importance
   - parent_id (INTEGER, FK → categories.id) - Hierarchy support
   - active (BOOLEAN DEFAULT 1) - Category status
   - created_at, updated_at (DATETIME)
   ```

5. **item_categories** - Many-to-many junction
   ```sql
   - item_id (INTEGER, FK → items.id ON DELETE CASCADE)
   - category_id (INTEGER, FK → categories.id ON DELETE CASCADE)
   - confidence (REAL DEFAULT 1.0) - Assignment confidence
   - PRIMARY KEY (item_id, category_id)
   ```

6. **article_scores** - Scoring and ranking
   ```sql
   - article_id (INTEGER PRIMARY KEY, FK → items.id)
   - rule_score (REAL) - Rule-based score
   - ml_score (REAL) - ML model score
   - source_score (REAL) - Source reputation score
   - recency_score (REAL) - Time-based score
   - final_score (REAL NOT NULL) - Combined score
   - explanation (JSON) - Score breakdown
   - scored_at (DATETIME) - Scoring timestamp
   - model_version (INTEGER) - ML model version used
   ```

7. **user_feedback** - User interaction tracking
   ```sql
   - id (INTEGER PRIMARY KEY)
   - article_id (INTEGER, FK → items.id)
   - feedback_type (TEXT NOT NULL) - Type of feedback
   - feedback_value (INTEGER) - Numeric value
   - feedback_at (DATETIME) - Timestamp
   - time_spent (INTEGER) - Reading time in seconds
   - used_for_training (BOOLEAN DEFAULT 0)
   - UNIQUE(article_id) - One feedback per article
   ```

8. **user_actions** - Detailed action logging
   ```sql
   - id (INTEGER PRIMARY KEY)
   - article_id (INTEGER, FK → items.id)
   - action (TEXT NOT NULL) - Action type
   - action_at (DATETIME) - Timestamp
   - context (JSON) - Additional context
   ```

9. **ml_models** - Machine learning model storage
   ```sql
   - id (INTEGER PRIMARY KEY)
   - model_type (TEXT NOT NULL) - Model identifier
   - model_data (BLOB NOT NULL) - Serialized model
   - feature_config (JSON) - Feature configuration
   - training_stats (JSON) - Training metrics
   - sample_count (INTEGER) - Training samples
   - created_at (DATETIME)
   - is_active (BOOLEAN DEFAULT 0) - Active model flag
   ```

10. **settings** - Key-value configuration store
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
- idx_items_feed_published ON items(feed_id, published DESC) - Feed timeline
- idx_items_content_extracted ON items(content_extracted) - Extraction queue
- idx_items_url_hash ON items(link, content_hash) - Duplicate detection
- idx_scores_final ON article_scores(final_score DESC) - Top articles
- idx_feedback_training ON user_feedback(used_for_training, feedback_at)
- idx_actions_article ON user_actions(article_id, action_at)
- idx_categories_active ON categories(active)
- idx_item_categories_item ON item_categories(item_id)
- idx_item_categories_category ON item_categories(category_id)
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

#### SQLite Configuration

**Pragmas for Optimization:**
```sql
PRAGMA foreign_keys = ON;          -- Enforce referential integrity
PRAGMA journal_mode = WAL;         -- Write-Ahead Logging
PRAGMA synchronous = NORMAL;       -- Balance safety/performance
PRAGMA cache_size = -64000;        -- 64MB cache
PRAGMA temp_store = MEMORY;        -- Memory for temp tables
```

#### Data Access Patterns

**Common Query Patterns:**

1. **Feed Updates**
   - Get feeds due for update: `WHERE next_fetch <= NOW() AND enabled = 1`
   - Batch upsert items: `INSERT ... ON CONFLICT(feed_id, guid) DO UPDATE`
   - Update feed metadata: Transaction with error handling

2. **Content Extraction**
   - Queue unprocessed: `WHERE content_extracted = 0 ORDER BY published DESC`
   - Store extracted content: Insert with FTS update trigger
   - Handle extraction errors: Update item and create error record

3. **Article Retrieval**
   - Recent articles: `ORDER BY published DESC LIMIT ? OFFSET ?`
   - By category: Join through item_categories
   - Full-text search: Query FTS table then join results
   - High-scoring: `WHERE final_score >= ? ORDER BY final_score DESC`

4. **Statistics and Analytics**
   - Feed performance: Aggregate scores, error rates
   - Category distribution: Count and confidence aggregation
   - User engagement: Action counts and time analysis

**Transaction Patterns:**
```go
// Complex operations use transactions
err := db.InTransaction(ctx, func(tx *sqlx.Tx) error {
    // Multiple related operations
    // All succeed or all rollback
})
```

#### Null Value Handling

All nullable columns use sql.Null* types:
- `sql.NullString` for optional text
- `sql.NullTime` for optional timestamps
- `sql.NullInt64` for optional integers
- `sql.NullFloat64` for optional decimals
- `sql.NullBool` for optional booleans

This ensures proper NULL handling and prevents zero-value ambiguity.

#### Migration Strategy

Schema is versioned and applied automatically:
1. Check current schema version
2. Apply pending migrations in order
3. Update schema version
4. All wrapped in transaction

Future migrations will be added as needed while maintaining backward compatibility.

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
  - Memory storage (temporary)

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

### 5. Scheduler (`pkg/scheduler`)

Manages periodic feed updates and content extraction with worker pools.

**Key Features:**
- Separate intervals for feed updates and content extraction
- Configurable worker pool sizes
- Rate limiting support
- Graceful shutdown with context cancellation

**Interfaces** (defined locally):
```go
type Database interface {
    GetEnabledFeeds(ctx context.Context) ([]db.Feed, error)
    GetItemsWithoutContent(ctx context.Context, limit int) ([]db.Item, error)
    UpdateFeed(ctx context.Context, feed *db.Feed) error
    CreateContent(ctx context.Context, content *db.Content) error
    // ... other methods
}

type Parser interface {
    Parse(ctx context.Context, url string) (*types.Feed, error)
}

type Extractor interface {
    Extract(ctx context.Context, url string) (content.ExtractResult, error)
}
```

### 6. Server (`server/`)

HTTP server providing REST API and (planned) web UI.

**Components:**
- **Server**: Main HTTP server using routegroup
  - Middleware support (recovery, throttling, auth)
  - JSON API endpoints
  - Planned: HTMX + Go templates for UI
  
- **DBAdapter**: Adapts database types to domain types
  - Converts SQL null types to Go types
  - Implements server's Database interface
  
**Current Endpoints:**
- `GET /ping` - Health check
- `GET /status` - Server status
- `GET /api/v1/feeds` - List feeds
- `GET /api/v1/items` - List items
- `GET /feeds/{topic}` - RSS feed by topic (planned)

### 7. Main Application (`cmd/newscope`)

Entry point that wires all components together.

**Initialization Flow:**
1. Load and validate configuration
2. Initialize database with migrations
3. Create parser and content extractor
4. Initialize scheduler with dependencies
5. Create server with database adapter
6. Start scheduler and server
7. Handle graceful shutdown on signals

## Data Flow and Structure Transformations

### Feed Update Flow:

```
1. Scheduler triggers update
   └─> db.Feed (from database)
   
2. Parser fetches and parses
   └─> gofeed.Feed (external library struct)
       └─> types.Feed (domain type)
           - Title, Description, Link
           - Items[]types.Item
   
3. Scheduler processes items
   └─> types.Item → db.Item conversion
       - GUID, Title, Link, Description
       - Published (time.Time → sql.NullTime)
       - Author (string → sql.NullString)
   
4. Database storage
   └─> db.UpdateFeed() - updates last_fetched, error status
   └─> db.UpsertItems() - inserts/updates items
```

### Content Extraction Flow:

```
1. Scheduler queries pending items
   └─> []db.Item (content_extracted = false)
   
2. Extractor processes each item
   └─> URL from db.Item.Link
       └─> content.ExtractResult
           - Text (main content)
           - Title, Author, Date
           - Language, Images, Links
   
3. Database storage
   └─> db.Content
       - ItemID (foreign key)
       - FullContent (string)
       - ExtractedAt (time.Time)
       - ExtractionError (sql.NullString)
```

### API Data Flow:

```
1. HTTP Request → Server Handler
   
2. Server → DBAdapter → Database
   └─> Query execution
   
3. Data transformation chain:
   a) List Feeds:
      db.Feed → types.Feed
      - sql.NullString → string (empty if null)
      - sql.NullTime → *time.Time (nil if null)
   
   b) List Items:
      db.Item → types.Item
      - sql.NullString.String → string
      - sql.NullTime.Time → time.Time
      - Adds FeedName from join
   
   c) Items with Content:
      db.ItemWithContent → types.ItemWithContent
      - Embeds types.Item
      - ExtractedContent from db.Content.FullContent
      - ExtractedAt from db.Content.ExtractedAt
   
4. JSON Response
   └─> types.* structs → JSON
       - Null values → empty strings or null
       - Times → ISO 8601 format
```

### Data Structure Mappings:

**Configuration → Runtime:**
```yaml
# config.yaml
feeds:
  - url: "https://example.com/rss"
    name: "Example"
    interval: 30m
```
→
```go
config.Feed{
    URL:      "https://example.com/rss",
    Name:     "Example", 
    Interval: 30 * time.Minute,
}
```

**External Feed → Domain:**
```go
// gofeed.Item (external)
{
    Title:       "Article Title",
    Link:        "https://...",
    Description: "Summary...",
    Published:   "Mon, 02 Jan 2006",
    GUID:        "unique-id",
}
```
→
```go
// types.Item (domain)
{
    FeedName:    "Example Feed",
    Title:       "Article Title",
    Link:        "https://...",
    Description: "Summary...",
    Published:   time.Time{...},
    GUID:        "unique-id",
}
```

**Domain → Database:**
```go
// types.Item
{
    FeedName:    "Example", // used to lookup feed_id
    Title:       "Title",
    Link:        "https://...",
    Description: "Text",
    Published:   time.Time{...},
}
```
→
```go
// db.Item
{
    FeedID:      123, // resolved from FeedName
    Title:       "Title",
    Link:        "https://...",
    Description: sql.NullString{String: "Text", Valid: true},
    Published:   sql.NullTime{Time: ..., Valid: true},
}
```

**Database → API Response:**
```go
// db.ItemWithContent (from JOIN query)
{
    Item: db.Item{...},
    FullContent: sql.NullString{String: "Article text", Valid: true},
    ExtractedAt: sql.NullTime{Time: ..., Valid: true},
}
```
→
```go
// types.ItemWithContent (API response)
{
    Item: types.Item{
        GUID:        "guid1",
        Title:       "Title",
        FeedName:    "Example",
        Description: "Summary",
        // ... other fields
    },
    ExtractedContent: "Article text",
    ExtractedAt:      &time.Time{...},
}
```

## Design Principles

1. **Interface Segregation**: Interfaces are defined on the consumer side, not the producer
2. **Dependency Injection**: All components receive dependencies through constructors
3. **Context Propagation**: All operations support context for cancellation and timeouts
4. **Error Handling**: Explicit error returns, no panics in library code
5. **Testability**: Mock generation with moq, high test coverage
6. **Pure Go**: No CGO dependencies for easy cross-compilation

## Testing Strategy

- **Unit Tests**: Each package has comprehensive tests
- **Mock Generation**: Using moq with go:generate directives
- **Integration Tests**: Database tests with in-memory SQLite
- **Table-Driven Tests**: Preferred testing pattern

## Future Enhancements

Areas marked for future development:
- Web UI with HTMX and Go templates
- Full REST API implementation
- User authentication and multi-tenancy
- Advanced feed management features
- Content categorization and search
- WebSub support for real-time updates