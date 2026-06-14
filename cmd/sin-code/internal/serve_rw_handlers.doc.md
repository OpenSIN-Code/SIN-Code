# `serve_rw_handlers.doc.md` — Direct MCP Read / Write / Edit Handlers

Implements the `sin_read`, `sin_write`, and `sin_edit` MCP tools. Unlike the
other handlers, these call the internal read/write/edit functions directly
instead of spawning a subprocess, keeping the edit loop hot path fast.

## What it does

- `handleRead` — parses arguments, resolves the path, and calls `readFile`.
- `handleWrite` — parses path and content, then calls `writeFileAtomic`.
- `handleEdit` — builds an `editRequest` from the arguments and calls `applyEdit`.
- Returns the result as JSON-formatted MCP text content.

## Files that import / touch it

- `cmd/sin-code/internal/serve.go` — `registerAllMCPTools` registers these handlers.
- `cmd/sin-code/internal/read.go` — `readFile` implementation.
- `cmd/sin-code/internal/write.go` — `writeFileAtomic` implementation.
- `cmd/sin-code/internal/edit.go` — `applyEdit` implementation.
- `cmd/sin-code/internal/serve_rw_handlers_test.go` — unit tests for validation,
  success, and error paths.

## Important config values & limits

- `readDefaultMaxBytes` is used as the default size guard for `sin_read`.
- `DefaultDriftWindow` is the default anchor drift tolerance for `sin_edit`.
- `handleWrite` validates required `path` and `content` arguments up front.

## Usage examples

These handlers are invoked by MCP clients as `sin_read`, `sin_write`, and
`sin_edit`.

## Known caveats / footguns

- `handleEdit` accepts all edit modes (anchor, symbol, string, dry-run, delete).
  Invalid edits are returned as MCP error results.
- `filepath.Abs` can only fail if the working directory is unavailable; this
  branch is hard to hit in tests.
