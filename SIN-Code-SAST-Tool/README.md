# SIN-Code SAST Tool — Phase 7

🔍 **Static Application Security Testing (SAST)** — Go-based source code vulnerability scanner.

## Overview

Detects 20+ vulnerability categories across 10+ languages, including:

- **Injection**: SQL Injection, Command Injection, XSS
- **Secrets**: Hardcoded API Keys, Passwords, JWT Tokens
- **Cryptography**: Insecure Hash (MD5/SHA1), Weak Random
- **Access Control**: Path Traversal, Open Redirect, SSRF
- **Deserialization**: Insecure YAML/JSON/Pickle parsing
- **Configuration**: Debug Mode, Insecure TLS, HTTP without HTTPS
- **Logging**: Sensitive Data in Logs
- **Concurrency**: Race Conditions (TOCTOU)

## Quick Start

```bash
# Build
go build -o sin-sast ./cmd/sin-sast

# Scan a project
./sin-sast scan ./my-project

# JSON output
./sin-sast scan ./my-project --output json

# Filter by severity
./sin-sast scan ./my-project --severity high

# Scan specific languages only
./sin-sast scan ./my-project --languages python,go,javascript

# List all rules
./sin-sast list-rules
```

## Architecture

| Component | Purpose |
|-----------|---------|
| `pkg/rules` | 20+ built-in detection rules (OWASP Top 10, CWE mapping) |
| `pkg/models` | Data structures (SASTFinding, SASTResult, Rule) |
| `internal/engine` | Pattern-matching engine with regex-based detection |
| `cmd/sin-sast` | CLI with Cobra framework |

## Rules Reference

| ID | Name | Severity | CWE | Languages |
|----|------|----------|-----|-----------|
| SAST-001 | SQL Injection (Raw Query) | Critical | CWE-89 | All |
| SAST-002 | SQL Injection (String Formatting) | Critical | CWE-89 | All |
| SAST-003 | Command Injection | Critical | CWE-78 | All |
| SAST-004 | Reflected XSS | High | CWE-79 | Web |
| SAST-005 | Path Traversal | High | CWE-22 | All |
| SAST-006 | Hardcoded API Key | High | CWE-798 | All |
| SAST-007 | Hardcoded Password | High | CWE-798 | All |
| SAST-008 | Hardcoded JWT/Auth Token | High | CWE-798 | All |
| SAST-009 | Insecure Hash (MD5) | Medium | CWE-327 | All |
| SAST-010 | Insecure Hash (SHA1) | Medium | CWE-327 | All |
| SAST-011 | Weak Random Number Generator | Medium | CWE-338 | All |
| SAST-012 | Insecure Deserialization | Critical | CWE-502 | All |
| SAST-013 | Server-Side Request Forgery (SSRF) | High | CWE-918 | All |
| SAST-014 | Open Redirect | Medium | CWE-601 | Web |
| SAST-015 | Insecure TLS Configuration | High | CWE-295 | All |
| SAST-016 | Debug Mode Enabled | Medium | CWE-489 | All |
| SAST-017 | Hardcoded Debug Credentials | High | CWE-798 | All |
| SAST-018 | Sensitive Data in Logs | Medium | CWE-532 | All |
| SAST-019 | Race Condition (TOCTOU) | Medium | CWE-367 | Compiled |
| SAST-020 | HTTP Without TLS | Low | CWE-319 | All |

## Integration with SIN-Code Security Bundle

```bash
# Via bundle CLI
sin-security scan ./my-project

# SAST only
sin-security sast ./my-project

# Skip SAST
sin-security scan ./my-project --skip-tools sast
```

## Output Formats

- **Text**: Human-readable with color-coded severity
- **JSON**: Machine-readable for CI/CD pipelines
- **SARIF**: (planned) for GitHub/CodeQL integration

## License

MIT — See [LICENSE](../LICENSE)
