---
name: skill-ecosystem-context
description: Unified context bridge that queries SCKG, sin-brain, GitNexus, and local SQLite in a single MCP call.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# skill-ecosystem-context

## Overview

Query multiple context sources at once when an agent needs cross-source knowledge.

## When to Use

- Agent needs code structure AND user preferences AND recent decisions.
- Need a single answer that blends SCKG, sin-brain, GitNexus, and local SQLite.

## When NOT to Use

- Only one source is needed (use the source directly).
- No sources are available.

## Core Process

```
IDENTIFY SOURCES → QUERY EACH → MERGE → SUMMARIZE
```

1. Detect which context sources are available.
2. Query relevant sources in parallel.
3. Merge results, deduplicate, and rank.
4. Summarize into a single coherent response.

## Sources

- **SCKG**: code structure, symbols, relationships.
- **sin-brain**: global rules, preferences.
- **GitNexus**: execution flows, impact analysis.
- **local SQLite**: project-specific memory.

## Verification

- [ ] At least one source responded.
- [ ] Answer cites source(s).
- [ ] No hallucinated facts outside source content.
