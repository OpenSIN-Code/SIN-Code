# ADR-006: GitNexus for Code Knowledge Graphs

## Status

Accepted

## Context

Agents need structural understanding of codebases. We need a graph-based analysis tool.

## Decision

GitNexus (external npm package) is mandatory for:
- Pre-flight analysis before agents code
- Impact analysis (what breaks if I change X?)
- Context queries (what is this function?)
- Semantic search

Integration: MCP server + `sin gitnexus` commands

## Consequences

- Positive: Rich graph context
- Positive: Independent of SIN-Code
- Negative: External dependency
- Negative: Requires Node.js
