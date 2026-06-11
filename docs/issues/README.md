# Issue Tracker

This directory contains **active and historical issues** for the sin-code bundle. Each issue is a Markdown file with a unique ID (format: `st-{4 chars}`) and follows a consistent structure.

> **Note:** This is the **Git-tracked** issue tracker. There's also a runtime bbolt store at `/tmp/sinator-issues.db` for ephemeral audit data (auto-completed/completed events). The Git tracker is the source of truth for planning.

## Index

| ID | Title | Priority | Status | Plan / ADR | Target |
|----|-------|----------|--------|------------|--------|
| [st-phw1](done/st-phw1-plugin-hook-wiring.md) | Plugin hooks → todo event wiring | P0 | **done** | [plugin-completion](../plans/plugin-system-completion.md) | v2.5.0 |
| [st-ptm2](done/st-ptm2-plugin-tools-mcp.md) | Plugin tools → MCP integration | P0 | **done** | [plugin-completion](../plans/plugin-system-completion.md) | v2.5.0 |
| [st-bvm3](st-bvm3-bubbletea-v2-migration.md) | Bubbletea v1.3.10 → v2.x migration | P2 | done | [bubbletea-v2-upgrade](../plans/bubbletea-v2-upgrade.md) | v3.0.0 |
| [st-gvc4](done/st-gvc4-govulncheck-blocking.md) | Re-enable govulncheck as blocking CI gate | P3 | **done** | [ADR-008](../adr/ADR-008-go-125-deferral.md) (superseded) | Go 1.25 stable |
| [st-pwt5](done/st-pwt5-plugin-wire-testfix.md) | Fix pre-existing TestE2E/plugin_wire failure | P2 | **done** | (none) | v2.5.0 |

## Priority Legend

- **P0** — Blocks next minor release (v2.5.0)
- **P1** — Blocks next major release (v3.0.0)
- **P2** — Should be done; not user-facing critical
- **P3** — Cleanup / follow-up; no functional impact

## Status Legend

- **open** — Work not yet started
- **in-progress** — Actively being worked on
- **blocked** — Waiting on external dependency
- **review** — PR open, awaiting review
- **done** — Completed and shipped

## Format

Each issue follows this structure:

```markdown
# Issue: <id> — <title>

| Field       | Value                            |
|-------------|----------------------------------|
| ID          | st-xxxx                          |
| Title       | <one-line summary>               |
| Status      | open / in-progress / blocked     |
| Priority    | P0 / P1 / P2 / P3                |
| Created     | YYYY-MM-DDTHH:MM:SSZ             |
| Reporter    | <who found it>                   |
| Plan        | <link to docs/plans/xxx.md>      |
| Component   | <affected package>               |
| Effort      | <rough estimate>                 |
| Blocks      | <other issue IDs>                |
| Blocked by  | <other issue IDs>                |

## Summary
## Symptoms
## Expected Behavior
## Acceptance Criteria
## Files Touched
## Definition of Done
```

## Cross-References

- **Plans** are in [`../plans/`](../plans/)
- **ADRs** are in [`../adr/`](../adr/)
- **CHANGELOG** is in [`../../CHANGELOG.md`](../../CHANGELOG.md)
- **Source TODO comments** link back to these issue IDs via `TODO(st-xxxx):`

## History

Issues are **immutable** once `done`. To retire an issue:
1. Move the file to `docs/issues/done/` (create the dir)
2. Update the status to `done`
3. Add a closing note at the bottom with the commit/tag that closed it
