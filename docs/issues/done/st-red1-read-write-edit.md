# Issue: st-red1 — Native sin_read/sin_write/sin_edit (Phase 1)

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-red1                                                     |
| Title       | Native Go sin_read/sin_write/sin_edit with hashline anchors |
| Status      | done                                                        |
| Priority    | P0 (closes the Read/Write/Edit gap)                         |
| Created     | 2026-06-11T00:00:00Z                                        |
| Component   | internal (read.go, write.go, edit.go, hashline.go)          |
| Effort      | 3-4 hours                                                   |

## Summary

Port of the deprecated Python bundle's hashline-anchored editing to native Go,
plus atomic validated writes and token-efficient reads with outline mode.

## Files

- `cmd/sin-code/internal/hashline.go` — SHA-256 line hashing, anchor parse/resolve/format, SplitLines/JoinLines
- `cmd/sin-code/internal/read.go` — ReadCmd, readFile(), outline builder (reuses grasp's extractors)
- `cmd/sin-code/internal/write.go` — WriteCmd, writeFileAtomic(), validateSyntax(), checkBracketBalance()
- `cmd/sin-code/internal/edit.go` — EditCmd, applyEdit(), unifiedDiff()
- `cmd/sin-code/internal/serve_rw_handlers.go` — handleRead/Write/Edit (in-process MCP, no subprocess)
- `cmd/sin-code/internal/hashline_test.go` — 7 unit tests
- `main.go`, `serve.go` — CLI + MCP registration
- `read.doc.md`, `write.doc.md`, `edit.doc.md` — CoDocs companions

## Commit

`e2bef65` feat(read-write-edit): native sin_read/sin_write/sin_edit with hashline anchoring
