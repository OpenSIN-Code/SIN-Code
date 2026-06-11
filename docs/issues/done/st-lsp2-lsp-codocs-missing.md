# Issue: st-lsp2 — Missing CoDocs for lsp package

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-lsp2                                                     |
| Title       | Add CoDocs companion files for the lsp package             |
| Status      | open                                                        |
| Priority    | P3 (housekeeping, not user-facing)                          |
| Created     | 2026-06-11T12:00:00Z                                        |
| Reporter    | jeremy (pro-coder audit)                                    |
| Doc         | [docs/lsp-known-issues.md#3](../lsp-known-issues.md)        |
| Component   | internal/lsp/client.go, internal/lsp_cmd.go                 |
| Effort      | 30 minutes (2 CoDoc files)                                  |

## Summary

Neither `cmd/sin-code/internal/lsp/client.go.doc.md` nor `cmd/sin-code/internal/lsp_cmd.go.doc.md` exist. The lsp package is non-trivial (JSON-RPC 2.0 framing, server discovery, notification draining) and warrants CoDoc companions per the project's documentation standard.

## Files to Create

1. `cmd/sin-code/internal/lsp/client.go.doc.md` — CoDoc for the LSP client
2. `cmd/sin-code/internal/lsp_cmd.go.doc.md` — CoDoc for the lsp CLI command

## Acceptance Criteria

- [x] `client.go.doc.md` created with: purpose, deps, usage examples, known caveats
- [x] `lsp_cmd.go.doc.md` created with: purpose, flags, examples
- [x] Inline `# Purpose:` + `# Docs:` headers already in place
- [x] `sin codocs check` passes (verified manually)

## Resolution

Both CoDoc files were created in the same commit (st-lsp2 + st-lsp3):

- `cmd/sin-code/internal/lsp/client.go.doc.md` (62 lines) — purpose, key
  design decisions (framing-aware reader, notification draining loop,
  timeout-bounded reads, single-threaded per request), public API
  documentation, known limitations.
- `cmd/sin-code/internal/lsp_cmd.go.doc.md` (27 lines) — purpose, list of
  all 8 subcommands with examples, key dependencies (gopls, pyright, tsserver,
  rust-analyzer), usage examples.
