# ADR-004: CoDocs — Co-located Documentation

## Status

Accepted

## Context

Documentation becomes stale. We need a documentation strategy that stays in sync with code.

## Decision

Every code file gets a companion `.doc.md` file:
- `tool.py` → `tool.doc.md`
- Explains what, why, dependencies
- Inline comments explain how
- Verified by `sin codocs check`

## Consequences

- Positive: Documentation stays close to code
- Positive: Machine-readable (Markdown)
- Negative: Double file count
- Negative: Requires discipline
