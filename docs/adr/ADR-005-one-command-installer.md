# ADR-005: One-Command Installer (`install.sh`)

## Status

Accepted

## Context

The ecosystem has 15+ tools. Manual installation is error-prone.

## Decision

Single `install.sh` that:
1. Detects OS and prerequisites
2. Installs Python bundle
3. Builds 7 Go tools
4. Installs 8 Python subsystems
5. Registers all in opencode.json
6. Runs smoke tests

## Consequences

- Positive: 2-minute installation
- Positive: Idempotent (safe to re-run)
- Negative: Platform-specific edge cases
- Negative: Requires local dev environment
