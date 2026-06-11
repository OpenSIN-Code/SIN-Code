# Issue: st-lsp3 — lsp_live.txt not wired into CI

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-lsp3                                                     |
| Title       | Wire lsp_live.txt testscript into CI (behind build tag)     |
| Status      | **done** (resolved 2026-06-11)                              |
| Priority    | P2 (regression protection, not user-facing)                 |
| Created     | 2026-06-11T12:00:00Z                                        |
| Resolved    | 2026-06-11T13:30:00Z                                        |
| Reporter    | jeremy (pro-coder audit)                                    |
| Doc         | [docs/lsp-known-issues.md#4](../lsp-known-issues.md)        |
| Component   | internal/lsp, testdata/scripts/lsp_live.txt, go-ci.yml     |
| Effort      | 2-3 hours (build tag + CI conditional)                      |

## Summary

`cmd/sin-code/testdata/scripts/lsp_live.txt` exists and exercises symbols / hover / definition / references / format against this repository. It is **not invoked** from the `e2e_test.go` test target by default because it requires:
1. gopls on `$PATH`
2. A built `sin-code` binary
3. A real Go file to LSP-query

These requirements make it unsuitable for the default test target — it should be opt-in or behind a build tag.

## Resolution

Added `cmd/sin-code/lsp_live_test.go` with `//go:build lsp_live` build tag.
When the tag is set, `TestLspLive` runs all testscripts in `testdata/scripts/`
including `lsp_live.txt` (which exercises symbols/hover/definition/references/format
against gopls v0.20+). When the tag is NOT set, the test is excluded from
the build entirely.

CI integration in `.github/workflows/go-ci.yml`:
- `Install gopls` step: `go install golang.org/x/tools/gopls@latest`
- `Test LSP live (build tag, opt-in)` step: `go test -tags lsp_live ./cmd/sin-code/ -run TestLspLive -count=1`

Verified locally: `go test -tags lsp_live ./cmd/sin-code/ -run TestLspLive` — all 8 testscript cases
(orchestrator_plan, harvest_help, grasp_help, orchestrator_run, todo_ready, todo_deps,
todo_stats, **lsp_live**) pass.

## Acceptance Criteria

- [x] `go test -tags lsp_live ./cmd/sin-code/` runs the testscript
- [x] `go test ./cmd/sin-code/` (default) skips the live test
- [x] go-ci.yml conditionally runs the live test (gopls installed via `go install golang.org/x/tools/gopls@latest`)
- [x] `lsp_live.txt` validates against current gopls (v0.22.0 verified)

## Definition of Done

`lsp_live.txt` is wired into CI behind a build tag, and CI catches regressions in the LSP client.
