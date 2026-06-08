# Architecture Decision Records (ADRs)

This directory contains the Architecture Decision Records (ADRs) for the SIN-Code Bundle.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [ADR-001](ADR-001-multi-agent-architecture.md) | Multi-Agent Architecture for SIN-Code | Accepted |
| [ADR-002](ADR-002-mcp-universal-interface.md) | MCP (Model Context Protocol) as Universal Tool Interface | Accepted |
| [ADR-003](ADR-003-sin-brain-memory.md) | 4-Tier Memory System (SIN-Brain) | Accepted |
| [ADR-004](ADR-004-codocs-standard.md) | CoDocs — Co-located Documentation | Accepted |
| [ADR-005](ADR-005-one-command-installer.md) | One-Command Installer (`install.sh`) | Accepted |
| [ADR-006](ADR-006-gitnexus-mandatory-graph.md) | GitNexus for Code Knowledge Graphs | Accepted |
| [ADR-007](ADR-007-plugin-extension-model.md) | Plugin Extension Model (TOML + subprocess) | Accepted |
| [ADR-008](ADR-008-go-125-deferral.md) | Go 1.25 Deferral — govulncheck stays warn-only | Accepted |

## What is an ADR?

An Architecture Decision Record (ADR) captures an important architectural decision made along with its context and consequences. ADRs are immutable: once accepted, they are not modified; superseded decisions are marked as `Deprecated` with a reference to the new ADR.

## Format

Each ADR follows this template:
- `# ADR-XXX: Title`
- `## Status` (Accepted, Proposed, Deprecated)
- `## Context`
- `## Decision`
- `## Consequences` (Positive + Negative)
