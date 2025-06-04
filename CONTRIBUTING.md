# Contributing to Newscope

Thank you for considering contributing to Newscope!

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check existing issues to avoid duplicates. When creating a bug report, include:

- Clear and descriptive title
- Steps to reproduce the issue
- Expected behavior vs actual behavior
- Your environment (OS, Go version, etc.)
- Relevant logs or error messages

### Suggesting Enhancements

Enhancement suggestions are welcome! Please provide:

- Clear and descriptive title
- Detailed description of the proposed functionality
- Why this enhancement would be useful
- Examples of how it would work

### Pull Requests

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Run linter (`golangci-lint run`)
6. Commit your changes (`git commit -m 'add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## Development Setup

### Prerequisites

- Go 1.24 or higher
- golangci-lint
- Docker (optional, for container builds)

### Building

```bash
# Local build
go build -o newscope ./cmd/newscope

# Docker image
docker build -t newscope .

# Multi-arch Docker build
docker buildx build --platform linux/amd64,linux/arm64 -t newscope .
```

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# With race detection
go test -race ./...
```

### Code Style

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable and function names
- Write clear comments for exported functions
- Keep functions focused and concise
- Add tests for new functionality

### Commit Messages

- Use clear and meaningful commit messages
- Start with a verb in present tense (add, fix, update, etc.)
- Keep the first line under 72 characters
- Reference issues when applicable

## Project Structure

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

## Getting Help

If you need help, you can:

- Open an issue for bugs or feature requests
- Check existing issues and pull requests
- Review the documentation in the README

## License

By contributing to Newscope, you agree that your contributions will be licensed under the MIT License.