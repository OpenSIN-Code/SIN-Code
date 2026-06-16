# prp/parse.go — Markdown I/O

The on-disk format. Same shape as the instinct package — YAML
frontmatter (machine) + Markdown body (human / agent).

## File layout

```markdown
---
id: my-change
title: My change
phase: planned
goal: ...
created_at: 2026-06-16T...
updated_at: 2026-06-16T...
tasks:
  - id: t1
    title: ...
    state: todo
  - id: t2
    title: ...
    state: todo
---

# My change

## Goal

...

## Context

...

## Plan

...

## Acceptance Criteria

...
```

## Section parser

The body is split on `## ` headings into a `map[name]body`. Names
are lowercased for stable matching. `Goal` from the frontmatter
wins over the body section (so the YAML stays the source of truth).

## Why hand-rolled, not `goldmark`/`blackfriday`

We only need section extraction, not full markdown rendering.
Adding a markdown library for a 30-line scanner would bloat the
binary for no gain.
