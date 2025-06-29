# Newscope Architecture Guide

Welcome to Newscope! This guide will help you understand how the application works and how to contribute effectively.

## What is Newscope?

Newscope is an RSS feed aggregator that uses AI to automatically classify and score articles based on their relevance to your interests. Think of it as a smart news reader that learns what you like and filters out the noise.

**Key Features:**
- Fetches articles from RSS/Atom feeds
- Extracts full article content from web pages
- Uses LLM (Large Language Models) to score articles 0-10 for relevance
- Learns from your feedback (likes/dislikes) to improve over time
- Provides a clean web UI and RSS export for high-scoring articles

## Architecture Overview

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Web UI    │     │   RSS Feed   │     │  REST API   │
│   (HTMX)    │     │   Export     │     │   (JSON)    │
└──────┬──────┘     └──────┬───────┘     └──────┬──────┘
       │                   │                     │
       └───────────────────┴─────────────────────┘
                           │
                    ┌──────▼──────┐
                    │   Server    │
                    │  (HTTP/API) │
                    └──────┬──────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
┌───────▼──────┐   ┌───────▼──────┐   ┌──────▼───────┐
│  Scheduler   │   │   Database   │   │     LLM      │
│              │   │   (SQLite)   │   │ Classifier   │
└───────┬──────┘   └──────────────┘   └──────────────┘
        │
┌───────▼───────────────────────────────┐
│          Processing Pipeline          │
│  ┌─────────┐  ┌──────────┐  ┌──────┐ │
│  │  Feed   │→ │ Content  │→ │ LLM  │ │
│  │ Fetcher │  │Extractor │  │Score │ │
│  └─────────┘  └──────────┘  └──────┘ │
└───────────────────────────────────────┘
```

## Core Concepts

Before diving into code, understand these key architectural patterns:

### 1. Interface Segregation
Interfaces are defined by the **consumer**, not the provider. This means:
- The `scheduler` package defines what it needs from a database
- The `server` package defines its own database interface
- This allows for better decoupling and easier testing

```go
// In scheduler/scheduler.go - consumer defines what it needs
type Database interface {
    GetFeeds(ctx context.Context, enabledOnly bool) ([]db.Feed, error)
    UpdateFeedFetched(ctx context.Context, feedID int64, nextFetch time.Time) error
    // ... only methods the scheduler actually uses
}
```

### 2. Dependency Injection
All components receive their dependencies through constructors:

```go
func New(cfg ConfigProvider, db Database, parser feed.Parser, extractor content.Extractor, classifier Classifier) *Scheduler {
    // Component is given everything it needs
}
```

### 3. Context-First Design
Every operation that might take time accepts a context:
- Enables cancellation and timeouts
- Propagates request-scoped values
- Essential for graceful shutdowns

### 4. Channel-Based Processing
The scheduler uses Go channels for efficient concurrent processing:
```go
Items to process → Channel → Workers → Extract & Classify → Database
```

## Component Guide

### Database Layer (`pkg/repository`)

**What it does:** Manages all data persistence using SQLite.

**Key files:**
- `db.go` - Database connection and core operations
- `schema.sql` - Database schema (embedded)
- `types.go` - Data structures

**Important tables:**
- `feeds` - RSS feed sources
- `items` - Individual articles
- `settings` - Key-value configuration store

**Finding things:**
- Feed operations: Look for methods starting with `Feed*` (e.g., `GetFeeds`, `UpdateFeedStatus`)
- Article operations: Methods with `Item*` (e.g., `GetClassifiedItems`, `UpdateItemFeedback`)
- Search functionality: `SearchItems` uses SQLite FTS5

### Feed System (`pkg/feed`)

**What it does:** Fetches and parses RSS/Atom feeds.

**Key components:**
- `Parser` - Wraps the gofeed library
- `Fetcher` - Downloads individual feeds
- `Manager` - Coordinates multiple feed operations

**Adding feed support:** Modify `parser.go` if you need to handle new feed formats or extract additional metadata.

### Content Extraction (`pkg/content`)

**What it does:** Extracts full article text from web pages.

**Key file:** `extractor.go`

**How it works:**
1. Downloads the article's web page
2. Uses go-trafilatura to extract content
3. Returns both plain text and rich HTML versions

**Customization:** Modify extraction options in `HTTPExtractor.Extract()`.

### LLM Classification (`pkg/llm`)

**What it does:** Uses AI to score articles and identify topics.

**Key concepts:**
- **Batch processing** - Classifies multiple articles in one API call
- **Feedback learning** - Uses your likes/dislikes to improve
- **Preference summaries** - Maintains a compressed summary of your preferences

**Key files:**
- `classifier.go` - Main classification logic
- `prompt.go` - Prompt construction (if separated)

**Customizing classification:** 
- Modify the system prompt in `buildClassificationPrompt()`
- Adjust scoring criteria or add new classification features

### Scheduler (`pkg/scheduler`)

**What it does:** Orchestrates the entire processing pipeline.

**Key responsibilities:**
1. Periodically fetches feeds
2. Manages the processing pipeline
3. Handles preference summary updates
4. Provides on-demand operations

**Important methods:**
- `Start()` - Begins periodic processing
- `UpdateFeedNow()` - Manually trigger feed update
- `updatePreferenceSummary()` - Manages AI preference learning

### Web Server (`server/`)

**What it does:** Provides the web UI and API.

**Key files:**
- `server.go` - Main server setup and routing
- `rest.go` - REST API handlers
- `htmx_handlers.go` - HTMX web UI handlers
- `adapters.go` - Database adapter for type conversion

**Adding new features:**
1. API endpoint: Add handler in `rest.go`, register in `routes()`
2. Web page: Add handler in `htmx_handlers.go`, create template
3. HTMX component: Create partial template, add handler

**Templates** (`server/templates/`):
- `base.html` - Layout wrapper
- `articles.html` - Main article list
- `article-card.html` - Individual article component
- Component templates can be rendered independently for HTMX

## Data Flow Examples

### When a new article arrives:

```
1. Scheduler triggers feed update
   - Checks which feeds need updating
   - Calls Parser to fetch RSS data

2. Parser returns new items
   - Deduplicates against existing articles
   - Saves to database

3. Item enters processing pipeline
   - Sent to processing channel
   - Worker picks it up

4. Worker processes item:
   - Extracts full content from article URL
   - Sends to LLM for classification
   - Saves results in single database update

5. Article appears in UI
   - With relevance score
   - Color-coded by score
   - Ready for user feedback
```

### When you click "Like":

```
1. HTMX sends POST to /api/v1/feedback/{id}/like

2. Server updates database
   - Sets user_feedback = 'like'
   - Records feedback_at timestamp

3. Triggers preference update
   - Scheduler checks if threshold reached
   - Updates preference summary if needed

4. Returns updated article HTML
   - HTMX swaps the article card
   - Shows feedback state
```

## Common Development Tasks

### Adding a new API endpoint

1. Define handler in `server/rest.go`:
```go
func (s *Server) myNewHandler(w http.ResponseWriter, r *http.Request) {
    // Implementation
}
```

2. Register route in `server/server.go`:
```go
r.HandleFunc("POST /api/v1/my-endpoint", s.myNewHandler)
```

### Adding a new database operation

1. Add method to `pkg/repository/db.go`:
```go
func (db *DB) MyNewOperation(ctx context.Context, param string) error {
    // Implementation
}
```

2. Add to consumer's interface:
```go
type Database interface {
    // existing methods...
    MyNewOperation(ctx context.Context, param string) error
}
```

### Modifying the classification prompt

Edit `pkg/llm/classifier.go`, find `buildClassificationPrompt()`:
- System prompt defines scoring criteria
- User prompt includes article data
- Feedback examples show the AI what you like

### Adding a new configuration option

1. Add to `pkg/config/config.go`:
```go
type LLMConfig struct {
    // existing fields...
    MyNewOption string `yaml:"my_new_option" jsonschema:"default=value"`
}
```

2. Use in relevant component:
```go
cfg.GetLLMConfig().MyNewOption
```

## Testing

### Running tests
```bash
go test ./...                    # All tests
go test ./pkg/repository/...     # Specific package
go test -run TestSpecificName    # Specific test
```

### Writing tests
- Use table-driven tests for multiple cases
- Generate mocks with `go generate ./...`
- Database tests use in-memory SQLite

Example test structure:
```go
func TestMyFeature(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    
    // Test cases
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        // cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## Code Organization

```
newscope/
├── cmd/newscope/          # Main application entry point
├── pkg/                   # Core packages
│   ├── config/           # Configuration management
│   ├── repository/       # Database layer
│   ├── feed/            # RSS feed handling
│   ├── content/         # Content extraction
│   ├── llm/             # AI classification
│   └── scheduler/       # Processing orchestration
├── server/               # HTTP server and web UI
│   ├── templates/       # HTML templates
│   └── static/          # CSS, JS, images
└── testdata/            # Test fixtures
```

## Debugging Tips

1. **Enable debug logging:**
   ```bash
   go run ./cmd/newscope --dbg
   ```

2. **Check the database:**
   ```bash
   sqlite3 newscope.db
   .tables                    # List tables
   .schema items             # Show table structure
   SELECT * FROM items LIMIT 5;  # Query data
   ```

3. **Test LLM classification:**
   - Check the `explanation` field in items table
   - Look for patterns in scores vs your expectations

4. **HTMX issues:**
   - Use browser DevTools Network tab
   - Check for `HX-Request: true` header
   - Verify response includes proper HTML

## Getting Help

1. **Read the tests** - They show real usage examples
2. **Check interfaces** - They document what each component needs
3. **Follow the types** - Go's type system guides you
4. **Ask questions** - Open an issue if something's unclear

Remember: The codebase follows standard Go conventions. When in doubt, check the [Effective Go](https://golang.org/doc/effective_go) guide.