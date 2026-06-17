---
name: skill-code-docs
description: Collaborative document coauthoring for READMEs, ADRs, specs, design docs, RFCs, API docs, and changelogs via MCP. Use for structured document workflows with the user.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
lifecycle: external
---

# skill-code-docs

## Overview

Create documents collaboratively through a structured workflow: gather context → propose outline → draft sections → review → render → export.

## When to Use

- "write a README"
- "create an ADR" / "draft a spec"
- "design doc" / "RFC" / "API docs"
- "changelog"
- Interactive coauthoring with the user.

## When NOT to Use

- `.doc.md` companion files for code (use `skill-code-codocs`).
- Image generation (use `skill-design-image`).
- Inline code comments (use `skill-code-codocs`).

## Core Process

```
START → GATHER CONTEXT → OUTLINE → DRAFT → REVIEW → RENDER → EXPORT
```

1. `doc_start` — choose type and title.
2. `doc_context_gather` — collect project context and goals.
3. `doc_outline_propose` — generate outline from template + context.
4. `doc_section_draft` — draft each section with clarifying questions.
5. `doc_review` — check completeness, accuracy, clarity.
6. `doc_format_render` — render to markdown / html / pdf.
7. `doc_export` — save to file, git commit, or share link.

## Doc Types

| Type | Use case |
|---|---|
| README | Project overview |
| ADR | Architecture Decision Record |
| SPEC | Technical specification |
| DESIGN | Design document |
| RFC | Request for comments |
| API | API documentation |
| CHANGELOG | Release notes |

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll write the doc in one shot." | Structured drafting produces better docs. |
| "I don't need user input." | Clarifying questions surface missing context. |
| "Review is optional." | Review catches gaps before export. |

## Red Flags

- Skipping context gathering.
- Drafting without an outline.
- Skipping review.
- Exporting to wrong destination.

## Verification

- [ ] Doc type and title chosen.
- [ ] Context gathered.
- [ ] Outline approved.
- [ ] All sections drafted.
- [ ] Review passed.
- [ ] Rendered and exported to correct destination.
