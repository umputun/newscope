# Newscope

<div align="center">
  <img src="server/static/img/logo.svg" alt="Newscope Logo" width="120">
</div>

[![Build Status](https://github.com/umputun/newscope/workflows/build/badge.svg)](https://github.com/umputun/newscope/actions) [![Coverage Status](https://coveralls.io/repos/github/umputun/newscope/badge.svg?branch=master)](https://coveralls.io/github/umputun/newscope?branch=master)

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

<details>
<summary>Screenshots</summary>

![Articles View](https://via.placeholder.com/800x450)
*Articles page with score filtering and topic tags*

![Feeds Management](https://via.placeholder.com/800x450)
*Managing RSS feed subscriptions*

![Content Extraction](https://via.placeholder.com/800x450)
*Full article content extraction*

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
  update_interval: 30    # minutes
  max_workers: 20

llm:
  endpoint: "https://api.openai.com/v1"
  api_key: "${OPENAI_API_KEY}"  # From environment
  model: "gpt-4o-mini"
  temperature: 0.3
  
  classification:
    feedback_examples: 50
    preference_summary_threshold: 25  # Number of new feedbacks before updating preference summary

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

### Providing Feedback

- **Like** - Articles you find interesting
- **Dislike** - Articles you want to avoid

The AI learns from your feedback to improve future scoring.

### Topic Preferences

Configure preferred and avoided topics in Settings to influence article scoring:
- **Preferred topics**: Increase article scores by 1-2 points
- **Avoided topics**: Decrease article scores by 1-2 points

This allows you to boost content you're interested in and filter out topics you want to avoid.

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
- Preference summaries update after configurable number of new feedbacks (default: 25)
- Updates are debounced to prevent excessive API calls
- Content extraction respects rate limits and robots.txt
- Database is SQLite, stored in `var/` directory

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