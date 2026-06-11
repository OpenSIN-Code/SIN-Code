# Issue: st-cov1 — Raise internal/ test coverage from 68.2% to ≥80%

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-cov1                                                     |
| Title       | Raise internal/ package test coverage from 68.2% to ≥80%  |
| Status      | open                                                        |
| Priority    | P2 (code quality, not user-facing)                          |
| Created     | 2026-06-11T12:00:00Z                                        |
| Reporter    | jeremy (pro-coder audit)                                    |
| Component   | cmd/sin-code/internal/ (all sub-packages)                   |
| Effort      | 4-8 hours (distributed across sub-packages)                 |
| Blocks      | v2.6.0 "raise coverage to ≥80%" goal from CHANGELOG         |

## Summary

`go test ./cmd/sin-code/internal/ -cover` reports **68.2%** as of v2.5.0.
The CHANGELOG historically claimed 93.6% (v1.0.9) but that was the
`cmd/sin-code` package only. Full project coverage including all
sub-packages is **68.2%**.

## Why Coverage Dropped

The 5 new phases (3-5) added ~3000 LOC of new code (index, AST, edit,
benchmarks) with limited direct test coverage:

| Sub-package | ~LOC | Notes |
|---|---|---|
| internal/lsp | 1500+ | `lsp_cmd.go` is mostly 0% (lspRun, lspSetup, langForPath, printLSPResult) |
| internal/memory | 800+ | `openMemoryStore`, `truncate` are 0% |
| internal/orchestrator | 1000+ | `loadAllAgents`, `runOrchestrator` are 0% |
| internal/agent | 500+ | `fetchModels`, `openAgentInEditor` are 0% |
| internal/ast (provider) | 200+ | `findSymbol`, `outlineEngineFor` (now tested in v2.5.0) |
| internal/index | 600+ | `allIndexedPaths`, `clear`, `remove`, `mockFileInfo` methods are 0% |

## Test Priorities (estimated impact)

1. **internal/lsp**: ~+3-4% — many MCP handler wrappers, easy to mock
2. **internal/memory**: ~+1-2% — openMemoryStore needs NIM env (use mocks)
3. **internal/orchestrator**: ~+2-3% — loadAllAgents + runOrchestrator
4. **internal/agent**: ~+1% — fetchModels, openAgentInEditor
5. **internal/index**: ~+0.5% — small uncovered surface

**Total estimated gain: ~8-10%** → coverage would reach ~76-78%.

## Acceptance Criteria

- [ ] `go test ./cmd/sin-code/internal/ -cover` reports ≥80%
- [ ] All lsp_cmd.go functions have ≥1 direct test
- [ ] All orchestrator_cmd.go functions have ≥1 direct test
- [ ] All memory_cmd.go functions have ≥1 direct test
- [ ] index_store.go uncovered methods (clear, remove, rootPath, hasFile) tested
- [ ] CHANGELOG v2.6.0 entry claims "coverage raised to X%"

## Definition of Done

`go test ./cmd/sin-code/internal/ -cover` reports ≥80% coverage, with
specific tests for all P1 functions in the sub-packages listed above.

## Note

This issue tracks only the **delta** from v2.5.0 (68.2%) to the 80% target.
The 93.6% claim in v1.0.9 was package-specific and cannot be directly
compared to the full-project coverage metric.
