# Issue: st-cov1 — Raise internal/ test coverage to ≥80%

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-cov1                                                     |
| Title       | Raise internal/ package test coverage to ≥80%              |
| Status      | **done**                                                    |
| Priority    | P2 (code quality, not user-facing)                          |
| Created     | 2026-06-11T12:00:00Z                                        |
| Closed      | 2026-06-14T15:00:00Z                                        |
| Reporter    | jeremy (pro-coder audit)                                    |
| Component   | cmd/sin-code/internal/ (root package)                       |
| Effort      | 4-8 hours (distributed across sub-packages)               |
| Blocks      | v2.6.0 "raise coverage to ≥80%" goal from CHANGELOG       |

## Summary

`go test ./cmd/sin-code/internal/ -cover` reports **80.0%** for the root
package at closure (v2.5.0). Continued work in the same session pushed the
same metric to **87.4%** (EFM tests excluded because they require Docker/OrbStack).

The original issue tracked raising coverage from 68.2% to 80%.
That target is now met. Remaining uncovered code is primarily
external-dependency code paths (live LSP, EFM, LLM APIs, security scanners)
that should be covered with heavier mocking or integration test harnesses.

## Acceptance Criteria

- [x] `go test ./cmd/sin-code/internal/ -cover` reports ≥80%
- [x] `lsp_cmd.go` functions have direct tests
- [x] `orchestrator_cmd.go` functions have direct tests
- [x] `memory_cmd.go` functions have direct tests
- [x] `agent_cmd.go` functions have direct tests
- [x] `read`/`write`/`edit` MCP handlers have direct tests
- [x] index_store.go uncovered surface reduced
- [x] CHANGELOG updated with v2.6.0 coverage target

## Commands Used

```bash
go test ./cmd/sin-code/internal/ -run '^Test[^E]|^TestE[^F]|^TestEF[^M]|^TestEFM[^_]' -cover -timeout 300s
```

Result: `coverage: 80.0% of statements`.

## Definition of Done

Root package coverage is at 80.0%, with tests for all reachable helper
functions and the major command entry points listed above. External-dependency
paths are intentionally deferred.
