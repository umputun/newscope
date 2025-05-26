# Newscope: Intelligent Personal RSS News Aggregator - System Design

## Executive Summary

Newscope is an intelligent RSS aggregator that learns from user preferences to create a personalized news feed. It combines rule-based filtering with machine learning to deliver relevant content while filtering out noise. The system is designed as a single-user application with potential for multi-user expansion.

## Core Objectives

1. **Aggregate** multiple RSS feeds into a unified stream
2. **Extract** full article content beyond RSS summaries  
3. **Classify** articles using both predefined rules and learned preferences
4. **Learn** from user feedback to improve recommendations
5. **Serve** filtered content as a clean, personalized RSS feed
6. **Provide** an intuitive UI for training and management

## System Architecture

### High-Level Components

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Feed Fetcher  │────▶│Content Extractor│────▶│  Classifier     │
│   (Scheduled)   │     │   (Readability) │     │  (Rules + ML)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
         │                       │                         │
         │                       │                         │
         ▼                       ▼                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                        SQLite Database                           │
│  (Articles, Feeds, Scores, Feedback, Models, Settings)         │
└─────────────────────────────────────────────────────────────────┘
         ▲                       ▲                         ▲
         │                       │                         │
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   HTTP Server   │     │   Web UI        │     │  ML Trainer     │
│   (RSS + API)   │     │   (HTMX)        │     │  (Background)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Detailed Component Design

#### 1. Feed Manager
- **Scheduler**: Cron-based feed fetching with per-feed intervals
- **Fetcher**: Concurrent HTTP client with timeout and retry logic
- **Parser**: Supports RSS 2.0, Atom, JSON Feed formats
- **Deduplication**: GUID-based with URL and content hash fallbacks
- **Error Handling**: Exponential backoff, circuit breaker pattern

#### 2. Content Extractor
- **Primary**: go-readability for main content extraction
- **Fallbacks**: 
  - OpenGraph/Twitter Card metadata
  - JSON-LD structured data
  - Custom extractors for popular sites
- **Quality Checks**:
  - Minimum content length
  - Text-to-HTML ratio
  - Language detection
- **Performance**:
  - Worker pool with per-domain rate limiting
  - Response caching
  - Timeout handling

#### 3. Classification Engine

##### Two-Stage Classification Pipeline:

**Stage 1: Rule-Based Scoring**
```
score = Σ(keyword_matches × position_weight × category_weight)

position_weights:
  - title: 3.0
  - first_paragraph: 2.0  
  - body: 1.0

matching:
  - exact: 1.0
  - fuzzy (>0.8 similarity): 0.8
  - stemmed: 0.6
```

**Stage 2: ML-Based Scoring**
- Initial: Naive Bayes with TF-IDF features
- Advanced: Logistic Regression with feature engineering
- Features:
  - Word frequencies (uni/bi-grams)
  - Topic overlap scores
  - Source reputation
  - Temporal features (time of day, day of week)
  - Content structure (length, media presence)

**Score Combination**:
```
final_score = α×rule_score + β×ml_score + γ×source_boost + δ×recency_factor

where:
- α, β, γ, δ are configurable weights (sum to 1.0)
- source_boost based on historical CTR per feed
- recency_factor = exp(-λ × hours_old)
```

#### 4. Storage Layer

**Database Schema (Extended)**:

```sql
-- Core content tables
CREATE TABLE feeds (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    last_fetched TIMESTAMP,
    next_fetch TIMESTAMP,
    fetch_interval INTEGER,
    enabled BOOLEAN DEFAULT 1,
    priority INTEGER DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    last_error TEXT,
    avg_score REAL, -- historical average score
    metadata JSON -- feed-specific settings
);

CREATE TABLE articles (
    id INTEGER PRIMARY KEY,
    feed_id INTEGER REFERENCES feeds(id),
    guid TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    content TEXT,
    content_hash TEXT,
    author TEXT,
    published TIMESTAMP,
    fetched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    language TEXT,
    read_time INTEGER, -- estimated minutes
    media_count INTEGER,
    extraction_method TEXT,
    UNIQUE(feed_id, guid)
);

-- Classification tables
CREATE TABLE categories (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    keywords JSON NOT NULL,
    is_positive BOOLEAN NOT NULL,
    weight REAL DEFAULT 1.0,
    parent_id INTEGER REFERENCES categories(id),
    active BOOLEAN DEFAULT 1
);

CREATE TABLE article_scores (
    article_id INTEGER REFERENCES articles(id) ON DELETE CASCADE,
    rule_score REAL,
    ml_score REAL,
    source_score REAL,
    recency_score REAL,
    final_score REAL NOT NULL,
    explanation JSON, -- why this score
    scored_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    model_version INTEGER,
    PRIMARY KEY (article_id)
);

-- User interaction tables
CREATE TABLE user_feedback (
    id INTEGER PRIMARY KEY,
    article_id INTEGER REFERENCES articles(id),
    feedback_type TEXT NOT NULL, -- 'interesting', 'boring', 'spam'
    feedback_value INTEGER, -- 1-5 scale for nuanced feedback
    feedback_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    time_spent INTEGER, -- seconds spent reading
    used_for_training BOOLEAN DEFAULT 0,
    UNIQUE(article_id)
);

CREATE TABLE user_actions (
    id INTEGER PRIMARY KEY,
    article_id INTEGER REFERENCES articles(id),
    action TEXT NOT NULL, -- 'view', 'click', 'share', 'save'
    action_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    context JSON
);

-- ML model storage
CREATE TABLE ml_models (
    id INTEGER PRIMARY KEY,
    model_type TEXT NOT NULL,
    model_data BLOB NOT NULL,
    feature_config JSON,
    training_stats JSON, -- precision, recall, f1
    sample_count INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT 0
);

-- System configuration
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value JSON NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Performance indexes
CREATE INDEX idx_articles_published ON articles(published DESC);
CREATE INDEX idx_articles_feed ON articles(feed_id, published DESC);
CREATE INDEX idx_scores_final ON article_scores(final_score DESC);
CREATE INDEX idx_feedback_training ON user_feedback(used_for_training, feedback_at);
CREATE INDEX idx_articles_url_hash ON articles(url, content_hash);
```

#### 5. Web Interface

**Pages**:
1. **Dashboard** (`/`)
   - Article list with infinite scroll
   - Quick feedback buttons
   - Filtering by score/date/source
   - Keyboard navigation

2. **Article View** (`/article/:id`)
   - Clean reading experience
   - Feedback widget
   - "Why recommended" explanation
   - Related articles

3. **Training Interface** (`/train`)
   - Batch feedback mode
   - Side-by-side comparison
   - Undo functionality

4. **Settings** (`/settings`)
   - Feed management
   - Category configuration  
   - Model performance metrics
   - Export/import options

**API Endpoints**:
```
# Content
GET    /api/articles?offset=0&limit=50&min_score=0.5
GET    /api/article/:id
POST   /api/article/:id/feedback
DELETE /api/article/:id

# Feeds  
GET    /api/feeds
POST   /api/feeds
PUT    /api/feed/:id
DELETE /api/feed/:id
POST   /api/feed/:id/refresh

# Classification
GET    /api/categories
PUT    /api/categories
GET    /api/article/:id/score
POST   /api/classifier/retrain

# System
GET    /api/stats
GET    /api/health
POST   /api/export
POST   /api/import
```

## Configuration Structure

```yaml
# config.yml
app:
  name: "Newscope"
  version: "1.0.0"
  environment: "production"

server:
  listen: ":8080"
  base_url: "https://newscope.example.com"
  read_timeout: "30s"
  write_timeout: "30s"

database:
  path: "./data/newscope.db"
  max_connections: 10
  busy_timeout: "5s"

fetcher:
  workers: 5
  timeout: "30s"
  user_agent: "Newscope/1.0 (+https://github.com/umputun/newscope)"
  max_redirects: 3
  rate_limit:
    requests_per_second: 2
    burst: 5

extractor:
  timeout: "30s"
  max_content_size: 5242880  # 5MB
  min_content_length: 100
  quality_threshold: 0.3
  cache_ttl: "24h"

classifier:
  # Weights for score combination (must sum to 1.0)
  weights:
    rule: 0.4
    ml: 0.4
    source: 0.1
    recency: 0.1
  
  # Score thresholds
  thresholds:
    include: 0.5      # Minimum score for RSS inclusion
    highlight: 0.8    # High-interest threshold
    spam: -0.5        # Auto-reject threshold
  
  # ML settings
  ml:
    enabled: true
    min_feedback_samples: 50
    retrain_interval: "24h"
    feature_max: 10000  # Max features for vectorization
    
  # Recency decay
  recency:
    half_life_hours: 72
    max_age_days: 7

feeds:
  - url: "https://news.ycombinator.com/rss"
    name: "Hacker News"
    interval: "30m"
    priority: 1
    
  - url: "https://lobste.rs/rss"
    name: "Lobsters"  
    interval: "1h"
    priority: 2

topics:
  interests:
    - name: "golang"
      keywords: 
        - "go"
        - "golang"
        - "go programming"
        - "gopher"
      weight: 2.0
      
    - name: "distributed"
      keywords:
        - "distributed systems"
        - "microservices"
        - "consensus"
        - "raft"
        - "kafka"
      weight: 1.5
      
    - name: "ai/ml"
      keywords:
        - "machine learning"
        - "artificial intelligence"
        - "neural network"
        - "deep learning"
        - "LLM"
        - "transformer"
      weight: 1.8
      
  avoid:
    - name: "blockchain"
      keywords:
        - "blockchain"
        - "bitcoin"
        - "ethereum"
        - "crypto"
        - "web3"
        - "nft"
      weight: -2.0
      
    - name: "politics"
      keywords:
        - "election"
        - "democrat"
        - "republican"
        - "congress"
        - "senate"
      weight: -1.5

output:
  title: "My Newscope Feed"
  description: "Personalized technology news"
  max_items: 100
  max_age: "7d"
  
maintenance:
  article_retention: "90d"
  vacuum_interval: "7d"
  backup_interval: "1d"
  backup_retention: 7
```

## Machine Learning Strategy

### Phase 1: Naive Bayes Baseline
```go
type NaiveBayesClassifier struct {
    WordCounts      map[string]map[bool]int  // word -> class -> count
    ClassCounts     map[bool]int             // class -> total docs
    VocabularySize  int
    Alpha          float64                   // Smoothing parameter
}

// Features: Bag of words with TF-IDF weighting
// Training: Incremental updates possible
// Inference: O(n) where n is document length
```

### Phase 2: Logistic Regression
```go
type LogisticRegression struct {
    Weights        []float64
    Bias          float64
    LearningRate  float64
    Regularization float64  // L2 penalty
    Features      FeatureVectorizer
}

// Features: TF-IDF + engineered features
// Training: SGD with mini-batches
// Inference: O(m) where m is feature dimension
```

### Feature Engineering
1. **Text Features**:
   - TF-IDF on title and content
   - N-grams (1-2)
   - Named entity counts
   - Sentiment scores

2. **Metadata Features**:
   - Source reputation (historical CTR)
   - Time features (hour, day, weekend)
   - Content length buckets
   - Media presence flags

3. **Interaction Features**:
   - Category match scores
   - Previous feedback on similar articles
   - Author history (if available)

### Training Pipeline
```
1. Collect feedback batch (every N samples or T time)
2. Prepare training data:
   - Balance positive/negative samples
   - Extract features
   - Split train/validation
3. Train new model:
   - Cross-validation for hyperparameters
   - Early stopping on validation loss
4. Evaluate:
   - Precision/Recall/F1
   - A/B test against current model
5. Deploy:
   - Atomic model swap
   - Keep last N models for rollback
```

## Implementation Plan

### Milestone 1: Core Infrastructure (Week 1-2)
- [x] Project setup and configuration
- [ ] Database schema and migrations
- [ ] Basic models and DAL
- [ ] Feed fetching with gofeed
- [ ] Scheduled job system
- [ ] Logging and metrics

### Milestone 2: Content Pipeline (Week 2-3)
- [ ] Content extraction with go-readability
- [ ] Quality assessment
- [ ] Deduplication logic
- [ ] Error handling and retries
- [ ] Performance optimization

### Milestone 3: Classification (Week 3-4)
- [ ] Rule-based classifier
- [ ] Score calculation and storage
- [ ] RSS feed generation
- [ ] Basic HTTP server
- [ ] API endpoints

### Milestone 4: Web UI (Week 4-5)
- [ ] Article list view
- [ ] Article reader
- [ ] Feedback system
- [ ] HTMX interactions
- [ ] Responsive design

### Milestone 5: Machine Learning (Week 5-6)
- [ ] Feature extraction
- [ ] Naive Bayes implementation
- [ ] Training pipeline
- [ ] Model persistence
- [ ] A/B testing framework

### Milestone 6: Polish & Operations (Week 6-7)
- [ ] Performance optimization
- [ ] Monitoring and alerting
- [ ] Documentation
- [ ] Docker packaging
- [ ] Deployment scripts

## Technical Stack

### Core Dependencies
```go
// Feed Processing
"github.com/mmcdole/gofeed"           // RSS/Atom parsing
"github.com/go-shiori/go-readability" // Content extraction

// Database
"modernc.org/sqlite"                  // Pure Go SQLite

// Web Framework  
"github.com/go-pkgz/rest"            // REST helpers
"github.com/go-pkgz/routegroup"      // Route grouping

// Background Jobs
"github.com/robfig/cron/v3"          // Cron scheduler

// Configuration
"gopkg.in/yaml.v3"                   // YAML parsing

// ML Libraries
"gonum.org/v1/gonum/mat"             // Matrix operations
"github.com/kljensen/snowball"       // Stemming

// Utilities
"github.com/google/uuid"             // UUID generation
"github.com/dustin/go-humanize"      // Human-friendly formatting
```

### Development Tools
- Testing: `testify`, `testcontainers-go`
- Mocking: `moq`
- Benchmarking: `go test -bench`
- Profiling: `pprof`
- Linting: `golangci-lint v2`

## Operational Considerations

### Monitoring & Metrics
```prometheus
# Key metrics to track
newscope_feeds_total{status="success|failure"}
newscope_articles_processed_total{action="fetched|extracted|classified"}
newscope_classification_score_histogram
newscope_feedback_total{type="positive|negative"}
newscope_model_accuracy_gauge
newscope_processing_duration_seconds{stage="fetch|extract|classify"}
```

### Error Handling Strategy
1. **Transient Errors**: Exponential backoff with jitter
2. **Permanent Errors**: Mark feed as failing after N attempts
3. **Partial Failures**: Process what's possible, log failures
4. **Data Corruption**: Validation at every stage, rollback capability

### Performance Targets
- Feed fetch: < 5s per feed (P95)
- Content extraction: < 2s per article (P95)
- Classification: < 100ms per article
- RSS generation: < 200ms for 100 items
- Web UI response: < 100ms (P95)

### Security Considerations
1. **Input Sanitization**: 
   - HTML sanitization for extracted content
   - SQL injection prevention (prepared statements)
   - XSS prevention in web UI

2. **Resource Limits**:
   - Max content size per article
   - Rate limiting per feed
   - Timeout on all external calls

3. **Privacy**:
   - No external analytics
   - Local-only data storage
   - Optional data export

## Future Enhancements

### Phase 2 Features
1. **Multi-device Sync**: 
   - API tokens for external clients
   - Read position syncing
   - Mobile app

2. **Advanced ML**:
   - Deep learning with embeddings
   - Collaborative filtering (multi-user)
   - Topic modeling (LDA)

3. **Content Enhancement**:
   - Summary generation
   - Translation support
   - Full-text search

4. **Social Features**:
   - Share collections
   - Public reading lists
   - Commentary system

### Scaling Path
1. **Database**: SQLite → PostgreSQL with read replicas
2. **Processing**: Single binary → Microservices
3. **ML**: In-process → Dedicated ML service
4. **Storage**: Local → S3-compatible object storage

## Success Criteria

### Functional
- Successfully processes 95%+ of configured feeds
- Correctly extracts content from 80%+ of articles
- Classification precision > 70% after training
- User feedback improves precision by 15%+

### Non-Functional  
- Zero data loss during normal operation
- Graceful degradation on component failure
- Single binary under 50MB
- Memory usage under 500MB for typical usage
- SQLite database under 1GB for 100k articles

## Risk Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| Feed blocking | No new content | User agent rotation, rate limiting, proxy support |
| Content extraction failures | Poor UX | Multiple extraction strategies, fallbacks |
| Model overfitting | Bad recommendations | Cross-validation, regularization, data augmentation |
| Database growth | Performance degradation | Retention policies, archival, query optimization |
| Feedback gaming | Model poisoning | Anomaly detection, feedback rate limits |

## Development Principles

1. **Simplicity First**: Start with simple solutions, iterate based on real usage
2. **User Control**: Every automated decision should be overrideable
3. **Transparency**: Show why articles were recommended
4. **Privacy**: All data stays local, no telemetry without consent
5. **Performance**: Fast enough to be pleasant on modest hardware
6. **Reliability**: Better to show something than nothing

## Testing Strategy

### Unit Tests
- Core algorithms (classification, scoring)
- Database operations
- Feed parsing edge cases
- Content extraction scenarios

### Integration Tests
- Full pipeline: feed → extract → classify → serve
- API endpoint testing
- Database migrations
- Error scenarios

### Performance Tests
- Load testing with 1000+ feeds
- Memory profiling under load
- Database query optimization
- Concurrent request handling

### User Acceptance Tests
- Classification accuracy with real feeds
- UI responsiveness
- Feedback loop effectiveness
- Edge case handling

This plan provides a comprehensive roadmap for building Newscope as a production-ready personal RSS aggregator with intelligent filtering capabilities.