# SIN-Code Container Tool (Go-Native)

Container security scanner that wraps Trivy, audits Dockerfiles, and inspects Docker images for security vulnerabilities and misconfigurations.

## Features

- **Trivy Integration**: Scans container images and filesystems for vulnerabilities
- **Dockerfile Auditing**: Best-practice checks, secret detection, curl|bash detection
- **Docker Image Inspection**: Metadata extraction (when Docker is available)
- **Multi-severity Support**: Critical, High, Medium, Low, Unknown
- **Hadolint Integration**: Optional Hadolint integration for Dockerfile linting
- **Zero dependencies**: Single static binary, no Python runtime needed
- **Fast**: Native Go parsing, efficient JSON processing

## Installation

```bash
go build -o sin-container-go ./cmd/container
```

## Usage

```bash
# Scan a filesystem (CI/CD mode without Docker)
./sin-container-go -path /path/to/project

# Scan a Docker image (requires Docker)
./sin-container-go -image alpine:3.18

# Scan with Dockerfile audit
./sin-container-go -path /path/to/project -dockerfile /path/to/Dockerfile

# Output to file
./sin-container-go -path /path/to/project -output results.json

# Set failure threshold
./sin-container-go -path /path/to/project -fail-on critical
```

## Dockerfile Checks

| Rule | Description | Severity |
|------|-------------|----------|
| SIN001 | Container runs as root / missing USER | HIGH |
| SIN002 | FROM uses :latest tag | MEDIUM |
| SIN003 | ADD used where COPY suffices | LOW |
| SIN004 | Secrets detected in ENV | CRITICAL |
| SIN005 | Too many RUN commands (layer bloat) | LOW |
| SIN006 | curl \| bash pattern detected | CRITICAL |

## Testing

```bash
go test -v ./...
```

## Architecture

- `pkg/models` - Shared data models (Vulnerability, Misconfiguration, DockerfileIssue, DockerImageInfo, ScanResult)
- `internal/trivy` - Trivy CLI wrapper and JSON parser
- `internal/scanner` - Main orchestration (Dockerfile audit, image inspection, status determination, recommendations)
- `cmd/container` - CLI entry point

## Docker Notes

This tool is designed to work with or without Docker:
- **With Docker**: Image metadata inspection + full container scanning
- **Without Docker**: Filesystem scanning via Trivy `fs` mode + Dockerfile auditing
- On macOS with OrbStack: Docker commands should work transparently if `docker` CLI is available

## License

MIT - OpenSIN-Code Project
