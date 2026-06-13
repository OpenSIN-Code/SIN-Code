# serve.doc.md — Unified MCP Server

Starts an MCP (Model Context Protocol) server that exposes all 15 in-tree
sin-code subcommands as MCP tools over stdio or HTTP transport.

## What it does

- Registers all in-tree tools in a `toolDef` table and adds them to the
  `go-sdk/mcp.Server`.
- Supports `stdio` transport (default) for MCP clients like opencode,
  Claude Code, or WebUI-v2.
- Supports `http` transport (with `--port` and `--transport=http`) that
  mounts the WebUI v2 HTTP API at `/api/v1/*` (issue #52).
- Plugin tools (from `internal/plugins`) are registered dynamically under
  `sin_plugin_<plugin>_<tool>` names.

## Tool registration pattern

Each in-tree tool is a `toolDef` struct in the `registerAllMCPTools`
function:

```go
type toolDef struct {
    name        string
    description string
    handler     func(ctx context.Context, args map[string]any) (string, error)
    schema      map[string]any
}
```

The handler receives JSON-RPC arguments as `map[string]any` and returns
a string result. Errors are wrapped by the dispatch loop into MCP error
responses (never panics; never returns an unescaped Go error string).

## Tool count history

| Version | Count | Notes |
|---------|-------|-------|
| v3.0.0 | 13 | Initial serve.go: discover, execute, map, grasp, scout, harvest, orchestrate, ibd, poc, sckg, adw, oracle, efm |
| v3.4.0 | 31+ | todo (12), memory (4), notifications (3), orchestrator (3), agents (3), lsp (1), read/write/edit (3) |
| v3.11.0 | 33+ | security scan + sbom generate (issue #36, this fix) |

## Security / SBOM tool details

### sin_security_scan (#36)

- Wraps the existing `security` CLI subcommand (security.go).
- Calls `runSecurityScan` directly — same code path as the CLI.
- Timeout ceiling: 3600s at MCP layer; per-tool timeout enforced by
  `runWithTimeout` in security.go.
- `strict` flag is accepted but does NOT propagate as an MCP error;
  the caller inspects `Summary.Issues` instead.
- Permission: `allow` (read-only — never mutates the scanned tree).

### sin_sbom_generate (#36)

- Wraps the existing `sbom` CLI subcommand (sbom.go).
- Calls `generateSBOM` → `generateSPDX` / `generateCycloneDX` directly.
- Path-escape guard: `output` parameter is rejected if it escapes the
  scan root.
- Permission: `allow` (read-only by default; the output sandbox is belt-and-suspenders).

### Orchestrator task type vs. MCP tool name

The orchestrator (`internal/orchestrator/model.go:30`) defines
`TaskSecurity`. The MCP tool name is `sin_security_scan`. These are
different namespaces — the former is for the agent-loop task system,
the latter is for the MCP wire protocol. No collision.

## Dependencies

- `github.com/modelcontextprotocol/go-sdk/mcp` — MCP server framework
- `github.com/spf13/cobra` — CLI dispatch (via cobra commands)
- `internal/apiweb` — WebUI v2 HTTP endpoints
- `internal/plugins` — dynamic plugin tool registration
- `internal/security.go` — `runSecurityScan`, `SecurityResult`, etc.
- `internal/sbom.go` — `generateSBOM`, `SPDXDocument`, `CycloneDXDocument`
- `internal/serve_rw_handlers.go` — `stringArg`, `intArg`, `boolArg` helpers

## Known caveats

- **No in-process caching:** each tool call spawns a subprocess via
  `runSubcommandRaw`. This is intentional — it isolates tool failures
  and prevents goroutine leaks. For performance-sensitive tool chains,
  prefer direct function calls (as security + sbom now do).
- **String-only results:** all handlers return `(string, error)`. Binary
  results or structured content types are serialized as inline JSON.
