# ADR-003: 4-Tier Memory System (SIN-Brain)

## Status

Accepted

## Context

AI agents need persistent memory across sessions. We needed a memory system that scales from short-term to long-term.

## Decision

4-tier memory system:
- Core: Critical conventions (always recalled)
- Recall: Recent context (last 7 days)
- Episodic: Task history (last 30 days)
- Consolidated: Long-term knowledge (summaries)

Implementation: SQLite + FTS5 for full-text search

## Consequences

- Positive: Fast local queries
- Positive: No external dependencies
- Negative: Single-user (no multi-user support)
- Negative: Requires periodic consolidation
