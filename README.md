# Newscope

[![Build Status](https://github.com/umputun/newscope/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/newscope/actions/workflows/ci.yml)
[![Coverage Status](https://coveralls.io/repos/github/umputun/newscope/badge.svg?branch=master)](https://coveralls.io/github/umputun/newscope?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/umputun/newscope)](https://goreportcard.com/report/github.com/umputun/newscope)

Brief description of your project.

## Features

- Feature 1
- Feature 2
- Feature 3

## Installation

```bash
go get -u github.com/umputun/newscope
```

Or download binary from [Releases](https://github.com/umputun/newscope/releases).

## Usage

```bash
newscope [options]
```

### Options

- `--listen` / `-l` - Listen address (default: `:8080`)
- `--dbg` - Enable debug mode
- `--version` / `-V` - Show version info
- `--no-color` - Disable color output
- `--help` / `-h` - Show help

## API

The server provides a REST API at `/api/v1`:

- `GET /api/v1/status` - Returns server status and version

## Building from source

```bash
make build
```

## Testing

```bash
make test
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.