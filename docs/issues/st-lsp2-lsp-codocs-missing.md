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

- [ ] `client.go.doc.md` created with: purpose, deps, usage examples, known caveats
- [ ] `lsp_cmd.go.doc.md` created with: purpose, flags, examples
- [ ] `sin codocs check` passes
- [ ] Inline `# Purpose:` + `# Docs:` headers added to both files (if not already present)

## Definition of Done

Both CoDoc files exist, pass `sin codocs check`, and inline header comments reference them.
