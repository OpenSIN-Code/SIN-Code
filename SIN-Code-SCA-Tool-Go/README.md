# SIN-Code SCA Tool (Go-Native)

Software Composition Analysis (SCA) scanner that parses package lock files and queries vulnerabilities via OSV.dev API.

## Features

- **Multi-ecosystem support**: npm, PyPI, Go, Maven
- **OSV.dev integration**: Batch queries for efficient vulnerability lookups
- **Zero dependencies**: Single static binary, no Python runtime needed
- **Fast**: Native Go parsing, concurrent batch processing

## Installation

```bash
go build -o sin-sca-go ./cmd/sca
```

## Usage

```bash
# Scan current directory
./sin-sca-go

# Scan specific project
./sin-sca-go -path /path/to/project

# Output to file
./sin-sca-go -path /path/to/project -output results.json
```

## Ecosystem Detection

| File | Ecosystem |
|------|-----------|
| `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `package.json` | npm |
| `requirements.txt`, `Pipfile.lock`, `poetry.lock` | PyPI |
| `go.mod` | Go |
| `pom.xml` | Maven |

## Testing

```bash
go test -v ./...
```

## Architecture

- `pkg/models` - Shared data models (Vulnerability, Package, ScanResult)
- `internal/osv` - OSV.dev API client (query + batch query)
- `internal/parser` - Dependency parsers for all ecosystems
- `internal/scanner` - Main orchestration logic
- `cmd/sca` - CLI entry point

## License

MIT - OpenSIN-Code Project
