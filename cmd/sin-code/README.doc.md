# `cmd/sin-code/` — Unified Go Binary

**What it does:** Houses the unified `sin-code` Go binary that consolidates 13 specialized SIN-Code analysis and manipulation tools into a single cobra-based CLI. Replaces 13 separate Go binaries with one.

**Created:** v1.0.0 (2026-06-06)

## Files

| File | Purpose |
|------|---------|
| `main.go` | Root cobra command, registers all 13 subcommands + serve |
| `main.go.doc.md` | This file (architecture overview) |
| `internal/common.go` | Shared utilities (error printing) |
| `internal/discover.go` | `sin-code discover` subcommand |
| `internal/execute.go` | `sin-code execute` subcommand |
| `internal/map.go` | `sin-code map` subcommand |
| `internal/grasp.go` | `sin-code grasp` subcommand |
| `internal/scout.go` | `sin-code scout` subcommand |
| `internal/harvest.go` | `sin-code harvest` subcommand |
| `internal/orchestrate.go` | `sin-code orchestrate` subcommand |
| `internal/ibd.go` | `sin-code ibd` subcommand |
| `internal/poc.go` | `sin-code poc` subcommand |
| `internal/sckg.go` | `sin-code sckg` subcommand |
| `internal/adw.go` | `sin-code adw` subcommand |
| `internal/oracle.go` | `sin-code oracle` subcommand |
| `internal/efm.go` | `sin-code efm` subcommand |
| `internal/serve.go` | `sin-code serve` — MCP server exposing all 13 tools |
| `internal/*_test.go` | Unit tests (20 tests, all pass) |

## Dependencies

- `github.com/spf13/cobra` v1.10.2 — CLI framework
- `github.com/modelcontextprotocol/go-sdk` v1.6.1 — MCP server SDK
- `github.com/google/jsonschema-go` v0.4.3 — JSON schema for MCP tools
- `golang.org/x/oauth2` — MCP OAuth (unused but transitive)
- `github.com/segmentio/encoding/json` — MCP JSON encoding
- Go 1.25.11 (per go.mod — fixes 36 standard library CVEs)

## Build & Install

```bash
go build -o ~/.local/bin/sin-code ./cmd/sin-code
```

## Integration Points

- **Python `sin` CLI** — `sin sin-code run <tool> -- <args>` routes through this binary
- **opencode MCP** — `~/.config/opencode/opencode.json` registers `sin-code serve` as one MCP server
- **Infra-SIN-OpenCode-Stack** — same registration, synced via `bin/sin-sync`

## Opencode MCP Registration (canonical)

```json
"sin-code": {
  "command": ["/Users/jeremy/.local/bin/sin-code", "serve"],
  "description": "SIN-Code unified toolchain (13 tools)",
  "enabled": true,
  "type": "local"
}
```

This single entry replaces 7 separate `sin-discover`, `sin-execute`, etc. entries.
