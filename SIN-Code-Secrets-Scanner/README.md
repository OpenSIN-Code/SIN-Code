# SIN-Code Secrets Scanner — Phase 8

🔐 **Secrets Scanner** — Detect leaked API keys, tokens, passwords, and credentials in code.

## Overview

Detects 22+ secret types across all files, including:

- **API Keys**: OpenAI, AWS, GitHub, Google, Stripe, Twilio, SendGrid, Mailgun, Heroku
- **Tokens**: Slack, JWT, Bearer, Discord, GitHub PAT
- **Credentials**: Database passwords, private keys, certificates
- **Config Files**: .env, Docker config, Kubernetes secrets, Terraform state

## Features

- 🔍 **Entropy-based filtering** — Reduces false positives using Shannon entropy
- 📊 **Severity classification** — Critical / High / Medium / Low
- 🎯 **22 detection rules** — Covering the most common secret types
- 🚀 **Fast scanning** — Pattern-matching with regex optimization
- 📋 **Multiple output formats** — Text (human-readable) and JSON (CI/CD)
- 🔒 **Secret masking** — Automatic masking in output to prevent leaks

## Quick Start

```bash
# Build
go build -o sin-secrets ./cmd/sin-secrets

# Scan a project
./sin-secrets scan ./my-project

# JSON output for CI/CD
./sin-secrets scan ./my-project --output json

# Filter by severity
./sin-secrets scan ./my-project --severity high

# Scan specific secret types only
./sin-secrets scan ./my-project --types api-key,token

# Enable entropy filtering (default: on)
./sin-secrets scan ./my-project --check-entropy

# List all rules
./sin-secrets list-rules
```

## Architecture

| Component | Purpose |
|-----------|---------|
| `pkg/rules` | 22 detection rules for secret types |
| `pkg/models` | Data structures (SecretFinding, SecretsResult, DetectionRule) |
| `internal/engine` | Pattern-matching engine with entropy calculation |
| `cmd/sin-secrets` | CLI with Cobra framework |

## Rules Reference

| ID | Name | Severity | Type | Confidence |
|----|------|----------|------|------------|
| SECRETS-001 | OpenAI API Key | Critical | api-key | High |
| SECRETS-002 | AWS Access Key ID | Critical | api-key | High |
| SECRETS-003 | AWS Secret Access Key | Critical | api-key | Medium |
| SECRETS-004 | GitHub Personal Access Token | Critical | api-key | High |
| SECRETS-005 | Slack Token | High | api-key | High |
| SECRETS-006 | Stripe API Key | Critical | api-key | High |
| SECRETS-007 | Google API Key | High | api-key | High |
| SECRETS-008 | Twilio API Key | High | api-key | High |
| SECRETS-009 | SendGrid API Key | High | api-key | High |
| SECRETS-010 | Mailgun API Key | High | api-key | High |
| SECRETS-011 | JWT Token | High | token | Medium |
| SECRETS-012 | Bearer Token / OAuth Token | High | token | Medium |
| SECRETS-013 | Generic API Key | Medium | api-key | Low |
| SECRETS-014 | Database Password | Critical | password | Medium |
| SECRETS-015 | Private Key (RSA/ECDSA/ED25519) | Critical | private-key | High |
| SECRETS-016 | Certificate / PEM | High | certificate | High |
| SECRETS-017 | .env File with Secrets | High | config-file | Low |
| SECRETS-018 | Docker Config / Registry Auth | High | config-file | Medium |
| SECRETS-019 | Kubernetes Secret | Critical | config-file | Medium |
| SECRETS-020 | Terraform Cloud Token / State | Critical | token | Medium |
| SECRETS-021 | Heroku API Key | High | api-key | Medium |
| SECRETS-022 | Discord Webhook / Bot Token | High | token | High |

## Integration with SIN-Code Security Bundle

```bash
# Via bundle CLI
sin-security scan ./my-project

# Secrets only
sin-security secrets ./my-project

# Skip secrets scan
sin-security scan ./my-project --skip-tools secrets
```

## Output Example

```
🔐 Secrets Scan Results
================================================================================
Path: ./my-project
Duration: 0.12s
Timestamp: 2026-06-06T18:00:00Z

📊 Summary
----------------------------------------
Files Scanned:  45
Secrets Found:  3

Status: FAILED

  Critical: 1  High: 2  Medium: 0  Low: 0

🔴 Leaked Secrets
--------------------------------------------------------------------------------
  [critical] SECRETS-001
  OpenAI API Key (api-key)
  File: ./config.py:3
  Match: sk-12**********************ef
  Entropy: 4.32
  Remediation: Remove from code. Use environment variables or a secret manager.

  [high] SECRETS-002
  AWS Access Key ID (api-key)
  File: ./aws_credentials:2
  Match: AKIA****************
  Remediation: Rotate the key immediately. Use IAM roles instead.
```

## License

MIT — See [LICENSE](../LICENSE)
