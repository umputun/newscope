# Code Smells Analysis

This document catalogs code smells and anti-patterns found in the Newscope codebase, prioritized by severity and impact.

## GUIDING PRINCIPLES

**Avoid Over-Engineering**: The goal is to improve code quality without making the project unnecessarily complex. Simple, clear solutions are preferred over elaborate abstractions.

**Key Guidelines:**
- **Method length is not a problem** if the method has a single, clear purpose and is easy to follow
- **Don't create abstractions** unless there's a clear, immediate benefit
- **Prefer simple fixes** that directly address the problem
- **Focus on readability and maintainability** over theoretical "best practices"
- **Only extract when it improves understanding** or reduces actual duplication

When evaluating fixes, ask:
1. Does this make the code easier to understand?
2. Does this solve a real problem we're experiencing?
3. Is the cure simpler than the disease?

## HIGH SEVERITY ISSUES

### 1. God Object - `server/server.go` (943 lines)
**Location:** Lines 1-943  
**Description:** The Server struct handles too many unrelated responsibilities:
- HTTP routing and middleware
- Template rendering and data preparation
- RSS feed generation
- Feed management endpoints
- Article processing and filtering
- Static file serving
- Error handling and logging

**Impact:** 
- Makes testing extremely difficult
- Violates Single Responsibility Principle
- High coupling between unrelated concerns
- Hard to maintain and extend
- Changes in one area affect unrelated functionality

**Recommended Solution:**
Break into separate, focused components:
```go
type HTTPServer struct {
    router *routegroup.Group
    middleware []Middleware
}

type TemplateRenderer struct {
    templates map[string]*template.Template
}

type FeedHandler struct {
    database Database
    scheduler Scheduler
}

type ArticleHandler struct {
    database Database
}

type RSSGenerator struct {
    baseURL string
}
```

### 2. Mixed Responsibilities - `server.go:579-666` - `articlesHandler()`
**Location:** Lines 579-666 (88 lines)  
**Description:** While the method is reasonably long, the real issue is mixing multiple concerns:
- HTTP request parsing (web layer)
- Business logic (filtering, data processing)
- Database operations (data layer)
- Response rendering (presentation layer)

**Impact:**
- Violates separation of concerns
- Hard to test business logic independently
- Changes to HTTP handling affect business logic

**Recommended Solution:**
Separate concerns, not necessarily method length:
```go
func (s *Server) articlesHandler(w http.ResponseWriter, r *http.Request) {
    // HTTP layer - only request/response handling
    filters := s.parseArticleFilters(r)
    
    // Delegate to business logic
    data, err := s.articleService.GetArticles(r.Context(), filters)
    if err != nil {
        s.renderError(w, err)
        return
    }
    
    // Presentation layer
    s.renderArticles(w, r, data)
}
```

### 3. Mixed Responsibilities - `scheduler.go:300-376` - `updateFeed()`
**Location:** Lines 300-376 (77 lines)  
**Description:** Mixes different concerns rather than being too long:
- Network operations (feed fetching)
- Business logic (duplicate detection)
- Database operations (item creation)
- Concurrency management (channel communication)
- Error handling and logging

**Impact:**
- Hard to test individual concerns
- Network issues affect database operations
- Difficult to retry specific operations

**Recommended Solution:**
Separate concerns by responsibility:
```go
func (s *Scheduler) updateFeed(ctx context.Context, feed *domain.Feed, processCh chan<- domain.Item) {
    // Single responsibility: coordinate feed update process
    parsedFeed, err := s.feedParser.Parse(ctx, feed.URL)
    if err != nil {
        s.handleFeedError(ctx, feed.ID, err)
        return
    }
    
    newItems := s.itemService.FilterNewItems(ctx, feed.ID, parsedFeed.Items)
    createdItems := s.itemService.CreateItems(ctx, feed.ID, newItems)
    s.queueForProcessing(processCh, createdItems)
    s.feedService.UpdateLastFetched(ctx, feed.ID)
}
```

## MEDIUM SEVERITY ISSUES

### 4. Parameter Lists - `llm/classifier.go:155-222` - `buildPromptWithSummary()`
**Location:** Lines 155-222  
**Description:** Function takes 4 parameters that are conceptually related:
```go
func buildPrompt(articles []domain.Item, feedbacks []domain.FeedbackExample, canonicalTopics []string, preferenceSummary string)
```

**Impact:**
- High coupling between parameters
- Difficult to extend with new prompt components
- Parameter order dependency

**Recommended Solution:**
```go
type PromptBuilder struct {
    Articles          []domain.Item
    Feedbacks         []domain.FeedbackExample
    CanonicalTopics   []string
    PreferenceSummary string
    TopicPreferences  TopicPreferences
}

func (pb *PromptBuilder) Build() string {
    // Build prompt logic
}
```

### 5. Data Clumps - Multiple locations
**Location:** Throughout codebase  
**Description:** Frequent parameter patterns where the same group of parameters appear together:
- `(minScore float64, topic string, feedName string, limit int)` - appears in multiple filter functions
- `(title string, description string, content string)` - content processing functions
- `(ctx context.Context, id int64, timestamp time.Time)` - database operations

**What are Data Clumps?**
Groups of data that frequently appear together and represent a cohesive concept. They're a sign of missed abstraction opportunities.

**Impact:**
- Parameter order confusion (easy to mix up similar parameters)
- High change amplification (adding a new filter requires updating many signatures)
- Repetitive validation and processing logic
- Missed domain modeling opportunities

**When to Fix vs. Leave Alone:**
✅ **Fix when:**
- Parameters appear together in 3+ places
- They represent a clear domain concept
- Adding new related data is likely

❌ **Don't fix when:**
- Only 1-2 occurrences
- Parameters aren't conceptually related
- It would create unnecessary indirection

**Simple Solution (when warranted):**
```go
// Only if this pattern appears frequently and represents a real concept
type ArticleFilters struct {
    MinScore float64
    Topic    string
    FeedName string
    Limit    int
}

// Simple helper, not a complex abstraction
func (af ArticleFilters) IsValid() bool {
    return af.MinScore >= 0 && af.Limit > 0
}
```

### 6. Feature Envy - `content/extractor.go:151-276`
**Location:** Lines 151-276  
**Description:** Extractor functions know too much about HTML node internals:
```go
func extractRichContent(doc *html.Node) string {
    // Directly manipulates html.Node internals
    // Deep knowledge of HTML structure
}
```

**Impact:**
- Tight coupling to HTML parsing library
- Hard to test without real HTML
- Difficult to swap HTML processors

**Recommended Solution:**
```go
type HTMLProcessor interface {
    ExtractText(node *html.Node) string
    ExtractRichContent(node *html.Node) string
    FindElements(node *html.Node, selector string) []*html.Node
}

type GoQueryProcessor struct{}
type PlainHTMLProcessor struct{}
```

### 7. Magic Numbers and Strings - Multiple locations
**Location:** Various files  

**Examples:**
```go
// server.go:324
baseURL := "http://localhost:8080"  // Should be configurable

// llm/classifier.go:92
maxRetries := 3  // Should be constant

// scheduler.go:133
processCh := make(chan domain.Item, 100)  // Buffer size should be configurable

// repository/item.go:253
time.Sleep(50 * time.Millisecond)  // Retry delay should be configurable
```

**Impact:**
- Configuration scattered throughout code
- Hard to adjust behavior without code changes
- No central place for tuning parameters

**Recommended Solution:**
```go
const (
    DefaultChannelBufferSize = 100
    DefaultRetryCount        = 3
    DefaultRetryDelay        = 50 * time.Millisecond
)

type ServerConfig struct {
    BaseURL string
    Timeout time.Duration
}
```

### 8. Complex Conditionals - `server.go:668-700`
**Location:** Lines 668-700 - `handleHTMXArticlesRequest()`  
**Description:** Nested conditionals for response handling:
```go
if r.Header.Get("HX-Request") != "" {
    if strings.Contains(r.Header.Get("HX-Target"), "articles-container") {
        // Complex logic
    } else {
        // Different complex logic
    }
}
```

**Impact:**
- Hard to follow logic flow
- Difficult to test all branches
- Error-prone when adding new conditions

**Recommended Solution:**
```go
func (s *Server) handleHTMXArticlesRequest(w http.ResponseWriter, r *http.Request, data ArticleData) {
    switch s.getResponseType(r) {
    case ResponseTypeArticleCards:
        s.renderArticleCards(w, data)
    case ResponseTypeFullPage:
        s.renderFullArticlePage(w, data)
    case ResponseTypeFilters:
        s.renderFiltersOnly(w, data)
    }
}
```

### 9. Unnecessary Public Exports - Multiple packages
**Location:** Throughout codebase  
**Description:** Many types, methods, and functions are exported (public) when they're only used within their own package, creating unnecessary API surface.

**High Impact Examples:**
```go
// pkg/repository/item.go - Should be private
type itemSQL struct {        // Only used internally for SQL operations
type topicsSQL []string      // Only used internally

// server/server.go - Should be private  
func (s *Server) generateRSSFeed()     // Only used internally
func (s *Server) renderPage()          // Only used internally
func (s *Server) writeTopicDropdown()  // Only used internally

// pkg/content/extractor.go - Should be private
func extractRichContent()              // Only used internally
func handleElementNode()               // Only used internally
```

**Impact:**
- Confusing public API - unclear what's intended for external use
- Accidental coupling - other packages might use internal functions
- Harder to refactor - public items create implicit contracts
- Documentation noise - godoc shows irrelevant internal details

**Simple Solution:**
Make items private by using lowercase names:
```go
// Before (public)
type ItemSQL struct {}
func (s *Server) GenerateRSSFeed() {}

// After (private)  
type itemSQL struct {}
func (s *server) generateRSSFeed() {}
```

**When to Keep Public:**
✅ Used by other packages
✅ Intended as public API
✅ Used in tests from different packages
❌ Only used within same package
❌ Internal implementation details

### 10. Inconsistent Error Handling - Multiple locations
**Location:** Throughout codebase  

**Examples:**
```go
// repository/item.go:295 - Custom error type
return &criticalError{err: fmt.Errorf("update failed: %w", err)}

// scheduler.go:183 - Simple log and continue
lgr.Printf("[WARN] failed to extract: %v", err)
return

// feed/parser.go:39 - Wrapped errors
return nil, fmt.Errorf("parse feed: %w", err)
```

**Impact:**
- Inconsistent debugging experience
- Unclear error handling strategies
- Different error context levels

**Recommended Solution:**
Standardize error handling:
```go
type ErrorLevel int

const (
    ErrorLevelWarning ErrorLevel = iota
    ErrorLevelError
    ErrorLevelCritical
)

func (s *Service) handleError(err error, level ErrorLevel, context string) error {
    wrapped := fmt.Errorf("%s: %w", context, err)
    
    switch level {
    case ErrorLevelWarning:
        s.logger.Warn(wrapped)
        return nil
    case ErrorLevelError:
        s.logger.Error(wrapped)
        return wrapped
    case ErrorLevelCritical:
        s.logger.Error(wrapped)
        return &CriticalError{wrapped}
    }
    
    return wrapped
}
```

## LOW-MEDIUM SEVERITY ISSUES

### 12. Poor Naming - Multiple locations
**Location:** Various files  

**Examples:**
```go
type itemSQL struct {}        // Should be ItemEntity or ItemRecord
type classificationSQL []string  // Should be TopicsList or TopicArray
func generateRSSFeed()       // Should be buildRSSFeed (builds, doesn't generate)
```

**Impact:**
- Unclear intent and purpose
- Misleading abstractions
- Harder to understand code flow

**Recommended Solution:**
```go
type ItemEntity struct {}
type TopicsList []string
func (s *Server) buildRSSFeed() []byte
```

### 13. Primitive Obsession - Multiple locations
**Location:** Various files  

**Description:** Overuse of primitive types (string, float64, int) instead of domain-specific types, but only problematic when it causes real issues.

**Examples that might warrant fixing:**
```go
type Classification struct {
    Score float64  // Scores can be invalid (negative, > 10)
    GUID  string   // GUIDs can be empty or malformed
}

func ProcessFeed(url string) // URLs can be invalid
```

**When to Fix vs. Leave Alone:**
✅ **Fix when:**
- Invalid values cause runtime errors
- Business rules need to be enforced
- Values have specific formats/constraints
- Type confusion is happening in practice

❌ **Don't fix when:**
- Primitives work fine and are clear
- No validation/business rules needed
- It would add complexity without benefit

**Simple Solution (only when needed):**
```go
// Only add validation if you're actually having problems with invalid values
type Score float64

func NewScore(s float64) (Score, error) {
    if s < 0 || s > 10 {
        return 0, fmt.Errorf("score must be between 0 and 10, got %f", s)
    }
    return Score(s), nil
}

// Keep it simple - just validation, not elaborate APIs
```

### 14. Dead Code - `repository/repository.go:117-135`
**Location:** Lines 117-135  
**Description:** `criticalError` type and `isLockError` function may be unused

**Impact:**
- Code bloat and confusion
- Unclear if error types are actually needed

**Recommended Solution:**
- Audit usage of these types
- Remove if unused or document usage clearly
- Consolidate error handling if multiple similar types exist

### 15. Switch Statement Smell - `content/extractor.go:205-246`
**Location:** Lines 205-246 in `handleElementNode()`  
**Description:** Large element type handling that could be polymorphic

**Impact:**
- Hard to extend with new HTML elements
- Violates Open/Closed Principle

**Recommended Solution:**
```go
type ElementHandler interface {
    CanHandle(tagName string) bool
    Process(node *html.Node) string
}

type ParagraphHandler struct{}
type LinkHandler struct{}
type HeaderHandler struct{}

type ElementProcessor struct {
    handlers []ElementHandler
}

func (ep *ElementProcessor) Process(node *html.Node) string {
    for _, handler := range ep.handlers {
        if handler.CanHandle(node.Data) {
            return handler.Process(node)
        }
    }
    return ""
}
```

### 16. Long Parameter Lists - `repository/item.go:252`
**Location:** Line 252 - `UpdateItemProcessed()`  
**Description:** Takes 4 related parameters:
```go
func UpdateItemProcessed(ctx context.Context, itemID int64, extraction *domain.ExtractedContent, classification *domain.Classification) error
```

**Impact:**
- High coupling between parameters
- Easy to mix up parameter order

**Recommended Solution:**
```go
type ProcessingResult struct {
    ItemID         int64
    Extraction     *domain.ExtractedContent
    Classification *domain.Classification
}

func (r *ItemRepository) UpdateItemProcessed(ctx context.Context, result ProcessingResult) error
```

## LOW SEVERITY ISSUES

### 17. Inappropriate Intimacy - Scheduler and Repository packages
**Location:** Throughout scheduler package  
**Description:** Scheduler knows too much about repository internals

**Impact:**
- Tight coupling between layers
- Hard to swap repository implementations

**Recommended Solution:**
Use more abstract interfaces with fewer methods per interface

### 18. Shotgun Surgery - Adding new article fields
**Location:** Multiple files  
**Description:** Adding a new article field requires changes in:
- `domain/item.go`
- `repository/item.go` (SQL struct)
- `server/server.go` (template data)
- Database schema

**Impact:**
- High change amplification
- Easy to miss updating one location

**Recommended Solution:**
- Use more generic field mapping
- Consider code generation for boilerplate
- Create field mapping utilities

### 19. Code Duplication - Error handling patterns
**Location:** Multiple repository files  
**Description:** Similar error wrapping patterns repeated:
```go
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
```

**Impact:**
- Repetitive code
- Inconsistent error messages

**Recommended Solution:**
```go
func wrapRepositoryError(operation string, err error) error {
    return fmt.Errorf("repository %s failed: %w", operation, err)
}
```

### 20. Performance Issues - Database queries
**Location:** `repository/item.go:141-159` - `GetItems()`  
**Description:** Query may lack proper indexing and pagination

**Impact:**
- Slow performance on large datasets
- Memory issues with large result sets

**Recommended Solution:**
- Add proper database indexes
- Implement cursor-based pagination
- Add query explain analysis

## ARCHITECTURAL CONCERNS

### 21. Tight Coupling - LLM package dependency
**Location:** `scheduler/scheduler.go:21-22`  
**Description:** Scheduler directly imports LLM package

**Impact:**
- Cannot easily swap LLM implementations
- Makes testing harder

**Recommended Solution:**
Define classifier interface in scheduler package:
```go
package scheduler

type Classifier interface {
    Classify(ctx context.Context, req ClassifyRequest) ([]domain.Classification, error)
}
```

### 22. Configuration Sprawl - `config/config.go:14-76`
**Location:** Lines 14-76  
**Description:** Monolithic configuration structure

**Impact:**
- Changes to one component affect others
- Hard to validate component-specific config

**Recommended Solution:**
```go
type Config struct {
    Server     ServerConfig
    Database   DatabaseConfig
    LLM        LLMConfig
    Scheduler  SchedulerConfig
}

// Each component validates its own config
func (sc ServerConfig) Validate() error
```

## PRIORITY RECOMMENDATIONS

**Philosophy: Fix only what's causing real problems. Simple solutions preferred.**

### Immediate (High Impact, Low Effort) - Do These First
1. **Fix unnecessary public exports** - make internal functions/types private (~25 items)
2. **Extract constants** for magic numbers and strings - simple and immediate benefit
3. **Fix naming** issues (itemSQL → itemEntity, etc.) - clarity with zero complexity cost
4. **Remove dead code** after usage audit - reduces confusion
5. **Standardize error handling** - pick one pattern and use it consistently

### Short Term (High Impact, Medium Effort) - Consider These
1. **Address God Object** in `server.go` - but only split what's actually unrelated
2. **Extract magic numbers** to constants/config - makes tuning easier
3. **Fix complex conditionals** - replace nested ifs with clear switch statements

### Medium Term (Only If Problems Arise)
1. **Parameter objects** - only if you're actually confusing parameter order
2. **Mixed responsibilities** - only split if you need to test parts independently
3. **Data clumps** - only if the pattern appears 3+ times and represents a real concept

### Long Term (Probably Don't Do)
1. **Domain value objects** - only if you're having validation/type confusion issues in practice
2. **HTML processing abstraction** - only if you need to swap HTML processors
3. **Dependency injection** - only if testing is actually difficult

### Questions to Ask Before "Fixing" Anything:
1. **Is this causing a real problem right now?**
2. **Will this make the code easier to understand for new contributors?**
3. **Is the solution simpler than the problem?**
4. **Am I fixing this because it bothers me or because it's actually problematic?**

## TOOLS TO HELP

Consider using these tools to prevent future code smells:
- **golangci-lint** with additional linters enabled
- **SonarQube** for code quality metrics
- **gocyclo** for cyclomatic complexity analysis
- **ineffassign** for unused variable detection
- **misspell** for typo detection

## NOTES

**Remember: The goal is working, maintainable software, not perfect abstractions.**

This analysis represents a snapshot of the codebase. As the code evolves, new smells may appear and existing ones may be resolved. 

**Key Principle**: Code quality improvements should make the code easier to work with, not more complex. If a "fix" introduces more concepts, interfaces, or indirection than it removes, it's probably not worth doing.

**When in doubt, ask**: "Would this change help someone new to the codebase understand what's happening faster?" If the answer is no, skip the change.