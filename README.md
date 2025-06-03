# Newscope

<div align="center">
  <img src="server/static/img/logo.svg" alt="Newscope Logo" width="120">
  
  **AI-Powered RSS Feed Curator**
  
  [![Build Status](https://github.com/umputun/newscope/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/newscope/actions/workflows/ci.yml)
  [![Coverage Status](https://coveralls.io/repos/github/umputun/newscope/badge.svg?branch=master)](https://coveralls.io/github/umputun/newscope?branch=master)
  [![Go Report Card](https://goreportcard.com/badge/github.com/umputun/newscope)](https://goreportcard.com/report/github.com/umputun/newscope)
  [![Go Version](https://img.shields.io/badge/go-1.24%2B-blue.svg)](https://go.dev/)
  [![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
</div>

## Overview

Newscope is an intelligent RSS feed aggregator that uses AI to automatically classify, score, and filter articles based on your interests. It learns from your feedback to continuously improve its recommendations, turning overwhelming RSS feeds into a personalized, high-quality reading experience.

## Key Features

- **AI-Powered Classification**: Automatically scores articles from 0-10 based on relevance
- **Smart Topic Detection**: Identifies and tags articles with relevant topics
- **Learning from Feedback**: Improves recommendations based on your likes/dislikes
- **Content Extraction**: Fetches and displays full article content
- **Custom RSS Feeds**: Generate filtered RSS feeds by topic and minimum score
- **Modern Web UI**: Clean, responsive interface with both expanded and condensed views
- **Real-time Updates**: Automatic feed updates with configurable intervals

## How It Works

1. **Feed Aggregation**: Newscope fetches articles from your configured RSS feeds
2. **AI Classification**: Each article is analyzed by an LLM (OpenAI, Ollama, etc.) to:
   - Assign a relevance score (0-10)
   - Extract key topics
   - Provide an explanation for the score
3. **Personalization**: Your feedback (likes/dislikes) trains the system to better understand your preferences
4. **Smart Filtering**: View articles filtered by score, topic, source, or date
5. **Content Extraction**: Click to extract and read full article content without leaving the app

## Installation

### Prerequisites

- Go 1.24 or higher
- SQLite (included with most systems)
- An OpenAI API key (or compatible LLM endpoint)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/umputun/newscope.git
cd newscope

# Build the application
go build -o newscope ./cmd/newscope

# Or use make
make build
```

### Docker (Coming Soon)

```bash
docker run -p 8080:8080 -v ./data:/data umputun/newscope
```

## Configuration

Create a `config.yml` file in the application directory:

```yaml
server:
  listen: ":8080"
  timeout: "30s"
  page_size: 50
  base_url: "http://localhost:8080"  # Change for production

database:
  dsn: "./var/newscope.db"
  max_open_conns: 10

schedule:
  update_interval: 30    # minutes
  max_workers: 20

llm:
  endpoint: "https://api.openai.com/v1"
  api_key: "${OPENAI_API_KEY}"  # From environment
  model: "gpt-4o-mini"
  temperature: 0.3
  max_tokens: 2000
  
  classification:
    feedback_examples: 50
    use_json_mode: true
    preferred_topics:
      - "programming"
      - "golang"
      - "ai"
      - "machine learning"
    avoided_topics:
      - "politics"
      - "celebrity"

extraction:
  enabled: true
  timeout: "30s"
  max_concurrent: 5
```

### Environment Variables

```bash
export OPENAI_API_KEY="your-api-key-here"
```

## Usage

### Starting the Server

```bash
# Run with default config
./newscope

# Run with custom config
./newscope -c /path/to/config.yml

# Run in debug mode
./newscope -d
```

### Command Line Options

- `-c`, `--config` - Path to config file (default: `config.yml`)
- `-d`, `--dbg` - Enable debug logging
- `-v`, `--version` - Show version and exit

### Web Interface

Open http://localhost:8080 in your browser.

#### Adding RSS Feeds

1. Navigate to the **Feeds** page
2. Click "Add New Feed"
3. Enter the feed URL and optional title
4. Set the fetch interval (default: 30 minutes)

#### Viewing Articles

1. Go to the **Articles** page
2. Use filters to refine your view:
   - **Min Score**: Slide to filter by relevance score
   - **Topic**: Select specific topics of interest
   - **Feed**: Filter by source
   - **Sort**: Order by date, score, or source
3. Switch between Expanded (⊞) and Condensed (☰) views

#### Providing Feedback

- Click 👍 to like articles that interest you
- Click 👎 to dislike irrelevant articles
- The AI learns from your feedback to improve future recommendations

#### Extracting Content

- Click "Extract Content" to fetch and display the full article
- Content is sanitized and formatted for easy reading
- Click "Hide" to collapse the content

### Custom RSS Feeds

Newscope can generate filtered RSS feeds for your favorite RSS reader:

1. Go to the **RSS Help** page
2. Use the RSS Builder to create custom feed URLs
3. Subscribe to feeds like:
   - `/rss` - All articles above score 5.0
   - `/rss/golang` - Golang articles
   - `/rss/ai?min_score=7.0` - AI articles with score ≥ 7.0

## Advanced Features

### Topic Preferences

Configure preferred and avoided topics in `config.yml`:
- **Preferred topics**: Boost scores by 1-2 points
- **Avoided topics**: Reduce scores by 1-2 points

### Custom System Prompt

Customize the AI's behavior with a system prompt:

```yaml
llm:
  system_prompt: |
    You are a tech news curator focusing on:
    - Software development best practices
    - Emerging technologies
    - Open source projects
    Rate articles based on technical depth and practical value.
```

### Using Alternative LLMs

Newscope supports any OpenAI-compatible API:

```yaml
# For Ollama
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"

# For OpenRouter
llm:
  endpoint: "https://openrouter.ai/api/v1"
  api_key: "${OPENROUTER_API_KEY}"
  model: "anthropic/claude-3-haiku"
```

## API Endpoints

### REST API

- `GET /api/v1/status` - Server status
- `POST /api/v1/feedback/{id}/{action}` - Submit feedback (like/dislike)
- `POST /api/v1/extract/{id}` - Extract article content
- `GET /api/v1/articles/{id}/content` - Get extracted content
- `POST /api/v1/feeds` - Create new feed
- `PUT /api/v1/feeds/{id}` - Update feed
- `DELETE /api/v1/feeds/{id}` - Delete feed

### RSS Endpoints

- `GET /rss` - All articles (default: score ≥ 5.0)
- `GET /rss/{topic}` - Articles for specific topic
- Query parameters:
  - `min_score` - Minimum score filter

## Development

### Project Structure

```
newscope/
├── cmd/newscope/       # Main application entry point
├── pkg/
│   ├── config/         # Configuration management
│   ├── content/        # Content extraction
│   ├── domain/         # Domain models
│   ├── feed/           # RSS feed parsing
│   ├── llm/            # AI classification
│   ├── repository/     # Database layer
│   └── scheduler/      # Feed update scheduler
├── server/             # HTTP server and handlers
├── templates/          # HTML templates
└── static/            # CSS, JS, images
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./pkg/llm/...
```

### Building from Source

```bash
# Build for current platform
make build

# Build for multiple platforms
make release

# Run tests
make test

# Run linter
make lint
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [Go](https://golang.org/)
- UI powered by [HTMX](https://htmx.org/)
- Icons from [Font Awesome](https://fontawesome.com/)
- Content extraction by [Trafilatura](https://github.com/adbar/trafilatura)

---

<div align="center">
  Made by <a href="https://github.com/umputun">Umputun</a>
</div>