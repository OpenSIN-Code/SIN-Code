# Issue: st-lsp3 — lsp_live.txt not wired into CI

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-lsp3                                                     |
| Title       | Wire lsp_live.txt testscript into CI (behind build tag)     |
| Status      | open                                                        |
| Priority    | P2 (regression protection, not user-facing)                 |
| Created     | 2026-06-11T12:00:00Z                                        |
| Reporter    | jeremy (pro-coder audit)                                    |
| Doc         | [docs/lsp-known-issues.md#4](../lsp-known-issues.md)        |
| Component   | internal/lsp, testdata/scripts/lsp_live.txt, go-ci.yml     |
| Effort      | 2-3 hours (build tag + CI conditional)                      |

## Summary

`cmd/sin-code/testdata/scripts/lsp_live.txt` exists and exercises symbols / hover / definition / references / format against this repository. It is **not invoked** from the `e2e_test.go` test target because it requires:
1. gopls on `$PATH`
2. A built `sin-code` binary
3. A network/local Go file to LSP-query

These requirements make it unsuitable for the default test target — it should be opt-in or behind a build tag.

## Suggested Fix

1. Add a `//go:build lsp_live` build tag to a new `lsp_live_test.go` that invokes the testscript
2. CI runs the test conditionally: `if command -v gopls && [ -x ./sin-code ]; then go test -tags lsp_live ./cmd/sin-code/; fi`
3. Add the LSP live test to the `benchmark` or a new `lsp-live` CI job

## Acceptance Criteria

- [ ] `go test -tags lsp_live ./cmd/sin-code/` runs the testscript
- [ ] `go test ./cmd/sin-code/` (default) skips the live test
- [ ] go-ci.yml conditionally runs the live test when gopls is available
- [ ] `lsp_live.txt` validates against current gopls (v0.20+)

## Definition of Done

`lsp_live.txt` is wired into CI behind a build tag, and CI catches regressions in the LSP client.
