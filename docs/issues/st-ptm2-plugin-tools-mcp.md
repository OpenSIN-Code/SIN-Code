# Issue: st-ptm2 — Plugin tools → MCP tools integration

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-ptm2                                                     |
| Title       | Plugin `[[tools]]` not exposed as MCP tools                 |
| Status      | done                                                        |
| Priority    | P0 (blocks v2.5.0 release)                                  |
| Created     | 2026-06-08T13:51:00Z                                        |
| Reporter    | jeremy (via sin-code v2.4.0 audit)                          |
| Plan        | [docs/plans/plugin-system-completion.md](../plans/plugin-system-completion.md) |
| Component   | plugins, serve (MCP)                                        |
| Effort      | 3-4 hours                                                   |
| Blocked by  | st-phw1                                                     |
| Blocks      | none                                                        |

## Summary

The plugin registry exposes a `MCPTools()` method (returns `[]mcp.Tool` for plugin-defined `[[tools]]` entries), but `internal/serve.go` only registers the 40 built-in `sin_*` tools and never iterates the plugin registry. The net effect: even if a user writes a plugin that defines a tool (e.g. `slack-notify`), the MCP server has no way to expose it to MCP clients like Claude Desktop or opencode.

## Why This Is A P0

Without this, plugins can only extend the **CLI** (subcommands, agents), not the **MCP surface** — which is the primary interface for AI agents. The plugin system is half-broken from the user's perspective.

## Plugin Tool Manifest Format

```toml
[[tools]]
name = "slack-notify"
description = "Send a Slack notification to a channel"
schema = '''
{
  "type": "object",
  "properties": {
    "channel": {"type": "string"},
    "message": {"type": "string"}
  },
  "required": ["channel", "message"]
}
'''
exec = "slack-notify"
```

## Expected Behavior

1. User drops a plugin at `~/.local/share/sin-code/plugins/slack/plugin.toml`
2. User starts MCP server: `sin-code serve` (or `sin-code mcp`)
3. `tools/list` from the MCP client includes `sin_plugin_slack_notify` (namespaced: `sin_plugin_<plugin>_<tool>`)
4. `tools/call` with `sin_plugin_slack_notify` invokes `slack-notify <args>` and returns stdout as the result
5. Error handling: missing binary → return MCP error `-32603` with descriptive message; non-zero exit → return error result

## Acceptance Criteria

- [ ] `serve.go` calls `pluginRegistry.MCPTools()` in its tool registration loop
- [ ] Plugin tool handler spawns the plugin's `exec` binary with JSON args via stdin
- [ ] Plugin tool result schema is `{content: [{type: "text", text: "<stdout>"}]}`
- [ ] Missing binary → MCP error code `-32603` (internal error)
- [ ] Plugin tools appear in `sin-code serve --list-tools` output
- [ ] `internal/serve_test.go` has 3 new tests: `TestPluginToolRegistered`, `TestPluginToolCallSuccess`, `TestPluginToolCallMissingBinary`
- [ ] `testdata/scripts/plugin_mcp.txt` — NEW: live E2E test for plugin tools via MCP

## Files Touched

- `internal/serve.go` — extend tool registration loop (~30 LOC)
- `internal/plugins/registry.go` — implement or extend `MCPTools()` (likely already partially exists)
- `internal/plugins/registry_test.go` — add tests
- `internal/serve_test.go` — add 3 tests
- `testdata/scripts/plugin_mcp.txt` — NEW: live E2E test

## Implementation Sketch

```go
// In serve.go, after registering built-in tools:
for _, plugin := range pluginRegistry.All() {
    for _, tool := range plugin.Tools {
        mcpName := fmt.Sprintf("sin_plugin_%s_%s", plugin.Name, tool.Name)
        server.AddTool(mcp.Tool{
            Name: mcpName,
            Description: tool.Description,
            InputSchema: tool.Schema,
        }, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
            argsJSON, _ := json.Marshal(req.Params)
            cmd := exec.CommandContext(ctx, tool.Exec)
            cmd.Stdin = bytes.NewReader(argsJSON)
            out, err := cmd.Output()
            if err != nil {
                return nil, fmt.Errorf("plugin tool %s: %w", mcpName, err)
            }
            return &mcp.CallToolResult{
                Content: []mcp.Content{{Type: "text", Text: string(out)}},
            }, nil
        })
    }
}
```

## Definition of Done

1. All acceptance criteria met
2. `go test ./... -count=1` green
3. `testdata/scripts/plugin_mcp.txt` passes
4. Live test: install a dummy plugin, run `sin-code serve`, hit `tools/list` and `tools/call` from a test MCP client
5. v2.5.0 release notes mention "Plugin tools now exposed via MCP"
