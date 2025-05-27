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

**Models:**
- `Feed`: RSS/Atom feed sources with metadata
- `Item`: Individual articles/posts from feeds
- `Content`: Extracted full-text content
- `Category`: Tags/categories for items
- `ItemWithContent`: Joined view of items with their content
- `FeedWithStats`: Feed with aggregated statistics

**Key Features:**
- Automatic schema migrations on startup
- SQL null types handling (NullString, NullTime, etc.)
- Transaction support for atomic operations
- Prepared statements for performance

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