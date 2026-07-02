# GSD Context — Project Lifecycle State

## State Files

| File | Purpose | Format |
|---|---|---|
| `.gsd/PROJECT.md` | Project name, description, tech stack | Markdown with YAML frontmatter |
| `.gsd/ROADMAP.md` | All phases with priority and status | Markdown headings |
| `.gsd/STATE.md` | Current phase, history | Markdown |
| `.gsd/plans/phase-N.md` | Task plan for phase N | Markdown checklist |

## Phase Status Flow

```
planning → in-progress → completed
                ↓
             blocked
```

## Priority Levels

| Priority | Meaning | When to use |
|---|---|---|
| P0 | Critical | Blocks everything else |
| P1 | High | Core functionality |
| P2 | Medium | Important but not blocking |
| P3 | Optional | Nice to have |

## Wave Execution

Tasks within a phase are grouped into dependency-ordered waves:
- Wave 1: tasks with no dependencies (parallel)
- Wave 2: tasks depending on Wave 1 (parallel after Wave 1)
- Wave N: etc.

Use `sin-code gsd execute <phase-id>` to analyze waves.
