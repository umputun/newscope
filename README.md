<div align="center">
  <img src="server/static/img/full-logo.svg" alt="Newscope" width="425">
  <br>
  
  [![Build Status](https://github.com/umputun/newscope/workflows/build/badge.svg)](https://github.com/umputun/newscope/actions) [![Coverage Status](https://coveralls.io/repos/github/umputun/newscope/badge.svg?branch=master)](https://coveralls.io/github/umputun/newscope?branch=master)
</div>

Newscope is a self-hosted RSS feed reader that uses AI to score articles based on your interests. It learns from your feedback to filter out noise and surface content you actually want to read. Instead of drowning in hundreds of articles, you get a personalized feed with scores from 0-10, automatic topic extraction, and the ability to generate custom RSS feeds for any topic with quality thresholds.

## Features

In addition to intelligent feed curation, newscope provides:

- AI-powered article scoring (0-10) with explanations
- Automatic topic extraction and tagging
- Learning from your feedback (likes/dislikes) with adaptive preference summaries
- Topic preferences management (preferred/avoided topics)
- Full content extraction from article pages
- Custom RSS feed generation with filters
- Modern web UI with multiple view modes
- Real-time feed updates
- Full-text search with partial word matching

## Basic Usage

```bash
# Use the provided docker-compose.yml
# Create config.yml (see Configuration section below)
# Set your OpenAI API key
export OPENAI_API_KEY=your-api-key

# Start with docker-compose
docker-compose up -d
```

Open http://localhost:8080 to access the web interface.

### Quick Start

1. Add RSS feeds through the web UI
2. Configure topic preferences in Settings (optional)
3. Let the AI analyze and score articles
4. Provide feedback to improve recommendations
5. Subscribe to filtered RSS feeds in your reader

## Screenshots

<details>
<summary><b>View Screenshots</b></summary>

### Main Interface

<p align="center">
  <img src="docs/screenshots/articles-main.png" alt="Articles View - Light Theme" width="800">
  <br>
  <em>Main articles view with AI-generated scores and topic tags</em>
</p>

### Article Views

<p align="center">
  <img src="docs/screenshots/articles-condensed.png" alt="Condensed View" width="800">
  <br>
  <em>Condensed view for scanning through more articles quickly</em>
</p>

<p align="center">
  <img src="docs/screenshots/article-expanded.png" alt="Expanded Article View" width="800">
  <br>
  <em>Article card with summary and metadata</em>
</p>

<p align="center">
  <img src="docs/screenshots/article-content.png" alt="Article Content" width="800">
  <br>
  <em>Full article content extracted and displayed</em>
</p>

<p align="center">
  <img src="docs/screenshots/articles-filtered.png" alt="Filtered Articles" width="800">
  <br>
  <em>Articles filtered by score threshold</em>
</p>

### Settings

<p align="center">
  <img src="docs/screenshots/settings-general.png" alt="Settings - General" width="800">
  <br>
  <em>Configure topic preferences to influence article scoring</em>
</p>

<p align="center">
  <img src="docs/screenshots/settings-ai-preferences.png" alt="AI Preferences" width="800">
  <br>
  <em>View and manage AI-learned preferences based on your feedback</em>
</p>

### Feed Management

<p align="center">
  <img src="docs/screenshots/feeds.png" alt="Feeds Management" width="800">
  <br>
  <em>Manage RSS feed subscriptions with custom update intervals</em>
</p>

### Search & Discovery

<p align="center">
  <img src="docs/screenshots/search.png" alt="Search Interface" width="800">
  <br>
  <em>Search bar with advanced query support</em>
</p>

<p align="center">
  <img src="docs/screenshots/search-results.png" alt="Search Results" width="800">
  <br>
  <em>Full-text search results with filtering options</em>
</p>

### RSS Integration

<p align="center">
  <img src="docs/screenshots/rss-help.png" alt="RSS Help" width="800">
  <br>
  <em>Generate filtered RSS feeds for any RSS reader</em>
</p>

### Mobile View

<p align="center">
  <img src="docs/screenshots/mobile-view.png" alt="Mobile View - Light" width="375">
  &nbsp;&nbsp;&nbsp;&nbsp;
  <img src="docs/screenshots/mobile-view-dark.png" alt="Mobile View - Dark" width="375">
  <br>
  <em>Responsive mobile design in light and dark themes</em>
</p>

### Dark Theme Examples

<p align="center">
  <img src="docs/screenshots/articles-main-dark.png" alt="Articles View - Dark Theme" width="800">
  <br>
  <em>Main interface in dark theme</em>
</p>

<p align="center">
  <img src="docs/screenshots/articles-condensed-dark.png" alt="Condensed View - Dark Theme" width="800">
  <br>
  <em>Condensed view in dark theme</em>
</p>

<p align="center">
  <img src="docs/screenshots/settings-dark.png" alt="Settings - Dark Theme" width="800">
  <br>
  <em>Settings page in dark theme</em>
</p>

<p align="center">
  <img src="docs/screenshots/feeds-dark.png" alt="Feeds - Dark Theme" width="800">
  <br>
  <em>Feed management in dark theme</em>
</p>

</details>

## Installation

### From Source

```bash
git clone https://github.com/umputun/newscope.git
cd newscope
go build -o newscope ./cmd/newscope
```

### Docker

```bash
docker run -d \
  -p 8080:8080 \
  -v ./config.yml:/srv/config.yml:ro \
  -v ./var:/srv/var \
  -e OPENAI_API_KEY=$OPENAI_API_KEY \
  umputun/newscope:latest
```

### Docker Compose

```bash
# Create config.yml first
docker-compose up -d
```

## Configuration

Create `config.yml`:

```yaml
server:
  listen: ":8080"
  base_url: "http://localhost:8080"  # Change for production

database:
  dsn: "./var/newscope.db"

schedule:
  update_interval: 1m               # Scheduler run interval (default: 1m, checks which feeds need updating)
  max_workers: 20                   # Maximum concurrent workers
  cleanup_age: 168h                 # Maximum age for low-score articles (default: 1 week)
  cleanup_min_score: 5.0            # Minimum score to keep articles regardless of age
  cleanup_interval: 24h             # How often to run cleanup (default: daily)
  
  # Retry configuration for database operations (SQLite lock handling)
  retry_attempts: 5                 # Number of retry attempts (default: 5)
  retry_initial_delay: 100ms        # Initial retry delay (default: 100ms)
  retry_max_delay: 5s               # Maximum retry delay (default: 5s)
  retry_jitter: 0.3                 # Jitter factor 0-1 to avoid thundering herd (default: 0.3)

llm:
  endpoint: "https://api.openai.com/v1"
  api_key: "${OPENAI_API_KEY}"  # From environment
  model: "gpt-4o-mini"
  temperature: 0.3
  
  classification:
    feedback_examples: 50
    preference_summary_threshold: 10  # Number of new feedbacks before updating preference summary
    summary_retry_attempts: 3         # Retry if summary contains forbidden phrases (default: 3)
    # Optional: Custom forbidden prefixes (defaults provided if not specified)
    # forbidden_summary_prefixes: ["The article discusses", "Article analyzes", "Discusses"]

extraction:
  enabled: true
  timeout: "30s"
```

## Web Interface

### Adding Feeds

Navigate to **Feeds** page and add RSS/Atom feed URLs. Each feed can have a custom update interval.

### Viewing Articles

The **Articles** page provides:
- Score-based filtering (slider)
- Topic filtering (clickable tags)
- Source filtering (clickable feed names)
- View modes: Expanded (⊞) or Condensed (☰)
- Sort options: date, score, or source

### Searching Articles

Click the magnifying glass icon in the navigation bar to search across all articles:
- **Simple searches**: Just type a word to find it anywhere in articles (e.g., "GPT" finds "ChatGPT")
- **Advanced searches**: Use operators for complex queries:
  - `golang OR python` - Find articles about either topic
  - `AI AND ethics` - Find articles about both topics
  - `crypto NOT bitcoin` - Find crypto articles excluding bitcoin
  - `"exact phrase"` - Search for an exact phrase
- Search results can be filtered by score, topic, source, and liked status
- Results are sorted by relevance by default

### Providing Feedback

- **Like** - Articles you find interesting
- **Dislike** - Articles you want to avoid

The AI learns from your feedback to improve future scoring.

### Topic Preferences

Configure preferred and avoided topics in Settings to influence article scoring:
- **Preferred topics**: Increase article scores by 1-2 points
- **Avoided topics**: Decrease article scores by 1-2 points

This allows you to boost content you're interested in and filter out topics you want to avoid.

### AI-Learned Preferences

The system automatically learns your preferences based on your likes and dislikes:
- **Preference Summary**: A personalized description of what content you prefer and want to avoid
- **Automatic Learning**: Updates after every 10 feedback actions (configurable)
- **Manual Control**: Edit the preference summary directly in Settings
- **Enable/Disable**: Toggle preference learning on/off
- **Reset**: Clear all preferences and start fresh

To manage your preferences:
1. Go to Settings → AI-Learned Preferences
2. View your current preference summary
3. Click "Edit" to modify it manually
4. Toggle the switch to enable/disable learning
5. Use "Reset" to clear all preferences

### Content Extraction

Click "Extract Content" on any article to fetch and display the full text. Content is sanitized and formatted for readability.

## Custom RSS Feeds

Generate filtered RSS feeds for any RSS reader:

- `/rss` - All articles (default: score ≥ 5.0)
- `/rss/{topic}` - Topic-specific feed
- `/rss?min_score=X` - Custom score threshold

Examples:
- `/rss/golang?min_score=7.0` - High-quality Go articles
- `/rss/ai` - All AI-related articles

## Alternative LLM Support

Newscope works with any OpenAI-compatible API:

### Ollama

```yaml
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
  api_key: "not-needed"
```

### OpenRouter

```yaml
llm:
  endpoint: "https://openrouter.ai/api/v1"
  api_key: "${OPENROUTER_API_KEY}"
  model: "anthropic/claude-3-haiku"
```

### Azure OpenAI

```yaml
llm:
  endpoint: "https://YOUR-RESOURCE.openai.azure.com"
  api_key: "${AZURE_OPENAI_KEY}"
  model: "gpt-4"
```

## Things to Know

- Articles are scored 0-10 based on relevance to your interests
- Preferred topics boost scores by 1-2 points
- Avoided topics reduce scores by 1-2 points
- Feedback is used to generate preference summaries that adapt to your reading habits
- Preference summaries update after configurable number of new feedbacks (default: 10)
- Updates are debounced to prevent excessive API calls
- Content extraction respects rate limits and robots.txt
- Database is SQLite, stored in `var/` directory
- Old articles with low scores are automatically cleaned up:
  - Articles older than `cleanup_age` (default: 1 week) with scores below `cleanup_min_score` (default: 5.0) are removed
  - Articles with user feedback (likes/dislikes) are preserved regardless of score
  - Cleanup runs periodically based on `cleanup_interval` (default: daily)

## API Endpoints

### REST API

- `GET /api/v1/status` - Server status and statistics
- `POST /api/v1/feedback/{id}/{action}` - Submit feedback (like/dislike)
- `POST /api/v1/extract/{id}` - Extract article content
- `GET /api/v1/articles/{id}/content` - Get extracted content

### Feed Management

- `GET /api/v1/feeds` - List all feeds
- `POST /api/v1/feeds` - Create new feed
- `PUT /api/v1/feeds/{id}` - Update feed
- `DELETE /api/v1/feeds/{id}` - Delete feed

### Preference Management

- `GET /api/v1/preferences` - Get preference summary and metadata
- `PUT /api/v1/preferences` - Update preference summary
- `DELETE /api/v1/preferences` - Reset all preferences

### RSS Endpoints

- `GET /rss` - All articles feed
- `GET /rss/{topic}` - Topic-specific feed
- Query parameter: `min_score` (default: 5.0)

## All Application Options

```
Usage:
  newscope [OPTIONS]

Application Options:
  -c, --config=  config file (default: config.yml) [$CONFIG]
  -d, --dbg      debug mode [$DEBUG]
  -v, --version  show version

Help Options:
  -h, --help     Show this help message
```

## Credits

- [go-pkgz/rest](https://github.com/go-pkgz/rest) - REST toolkit
- [jmoiron/sqlx](https://github.com/jmoiron/sqlx) - Database extensions
- [sashabaranov/go-openai](https://github.com/sashabaranov/go-openai) - OpenAI client
- [mmcdole/gofeed](https://github.com/mmcdole/gofeed) - RSS parser