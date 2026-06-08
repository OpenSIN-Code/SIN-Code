# Plan: Plugin System Completion (v2.5.0)

**Status:** proposed
**Owner:** jeremy
**Target release:** v2.5.0 (Q3 2026)
**Related ADRs:** [ADR-007](../adr/ADR-007-plugin-extension-model.md)
**Related issues:** [st-phw1](../issues/st-phw1-plugin-hook-wiring.md), [st-ptm2](../issues/st-ptm2-plugin-tools-mcp.md)

---

## Executive Summary

The plugin system shipped in v2.3.0 (commit `bc524c8`) with **3 of 4 surfaces** functional: subcommands, agents, and the plugin discovery mechanism. The 4th surface — **tools exposed via MCP** — was scoped for a follow-up release, and the hook integration (which was designed but not wired) was also deferred. This plan covers both gaps as a single, cohesive v2.5.0 release.

**Why now:** Without these two pieces, plugins are a half-measure. Users can extend the CLI but not the AI agent surface (MCP), and can't react to system events (hooks).

---

## Goals

1. **Plugin tools exposed as MCP tools** — A plugin's `[[tools]]` entries become callable via `tools/call` from any MCP client
2. **Plugin hooks fire on todo events** — `[[hooks]]` entries are invoked when `todo add`/`complete`/`delete` runs
3. **Backward compatibility** — Existing v2.3.0/v2.4.0 plugins (CLI extension only) continue to work
4. **Test coverage** — 6 new unit tests + 2 new E2E testscripts
5. **Documentation** — Update `docs/plugin-system-design.md` with worked examples

---

## Non-Goals

- **Plugin marketplace** (deferred to v3.0+)
- **Plugin signing/verification** (security roadmap, separate ADR)
- **Plugin hot-reload** (plugins load at startup only, restart required for changes)
- **Plugin-to-plugin dependencies** (`[[depends_on]]` not in scope)

---

## Architecture

### Current State (v2.4.0)

```
Plugin Manifest (TOML) → Registry.LoadManifest() → Registry (in-memory)
                                ↓
                  ┌─────────────┼─────────────┐
                  ↓             ↓             ↓
              Subcommands    Agents        Hooks
              (cobra)     (orchestrator)   (DEFINED
                                           but NOT FIRED)
                                                       
                  Tools (DEFINED but NOT EXPOSED to MCP)
```

### Target State (v2.5.0)

```
Plugin Manifest (TOML) → Registry.LoadManifest() → Registry (in-memory)
                                ↓
                  ┌─────────────┼─────────────┬─────────────┐
                  ↓             ↓             ↓             ↓
              Subcommands    Agents        Hooks        Tools
              (cobra)     (orchestrator)   (fired on     (registered
                                           todo events)  as MCP tools)
```

### Hook Event Flow

```
User runs: sin-code todo complete st-xyz
                              ↓
                    todo.complete() in todo.go
                              ↓
                ┌─────────────┴─────────────┐
                ↓                           ↓
    Mark st-xyz as done          firePluginHooks("post_complete", todo)
    in bbolt store                          ↓
                ↑                  PluginRegistry.HooksForEvent("post_complete")
                │                           ↓
                │                  For each hook: exec plugin-exec <args>
                │                           ↓
                │                  Capture stdout/stderr → audit log
                │                           ↓
                └────────── (regardless of hook success/failure) ──┘
```

### MCP Tool Registration Flow

```
sin-code serve (MCP server starts)
                              ↓
                serve.registerTools()
                              ↓
    ┌─────────────────────────┴─────────────────────────┐
    ↓                                                    ↓
Built-in tools (40)                            Plugin tools (N × M)
    sin_discover, sin_todo, ...                  sin_plugin_<plugin>_<tool>
    ↓                                                    ↓
    mcp.AddTool(name, schema, handler)          mcp.AddTool(name, schema, 
                                                       pluginHandler)
                                                         ↓
                                                  pluginHandler spawns
                                                  plugin-exec, pipes JSON
                                                  args via stdin, returns
                                                  stdout as result
```

---

## Implementation Phases

### Phase 1: Plugin Hook Wiring (Issue st-phw1) — 2-3 hours

**Goal:** Hooks fire on todo events with proper error handling.

**Files:**
- `internal/plugins/registry.go` — Add `HooksForEvent(event string) []HookConfig` method
- `internal/todo/plugin_hooks.go` — Extend `firePluginHooks` (already exists, needs ~30 LOC)
- `internal/todo/todo.go` — Call `firePluginHooks` from `complete()`, `add()`, `delete()`
- `internal/todo/plugin_hooks_test.go` — NEW, 3 unit tests
- `internal/plugins/registry_test.go` — Add 1 test for `HooksForEvent`
- `testdata/scripts/plugin_hooks.txt` — NEW, E2E test

**Test cases:**
1. Hook fires on `post_complete`, audit log records invocation
2. Missing binary → error logged, todo action succeeds
3. Hook timeout → error logged, todo action succeeds
4. `HooksForEvent` returns hooks for the given event type

**Code sketch:**

```go
// internal/todo/plugin_hooks.go
func firePluginHooks(event string, todo *Todo) {
    hooks := pluginRegistry.HooksForEvent(event)
    for _, hook := range hooks {
        ctx, cancel := context.WithTimeout(context.Background(), 
            time.Duration(hook.TimeoutSeconds)*time.Second)
        defer cancel()
        
        argsJSON, _ := json.Marshal(map[string]any{
            "event": event,
            "todo":  todo,
        })
        cmd := exec.CommandContext(ctx, hook.Exec)
        cmd.Stdin = bytes.NewReader(argsJSON)
        out, err := cmd.CombinedOutput()
        
        if err != nil {
            fmt.Fprintf(os.Stderr, "plugin hook %s/%s: %v\n",
                hook.PluginName, hook.Exec, err)
        }
        
        // Record in audit log
        auditLog.Record(audit.Entry{
            TodoID:  todo.ID,
            Actor:   "plugin:" + hook.PluginName,
            Action:  "hook:" + event,
            Note:    string(out),
        })
    }
}
```

---

### Phase 2: Plugin Tools → MCP (Issue st-ptm2) — 3-4 hours

**Goal:** Plugin tools callable via MCP `tools/call`.

**Files:**
- `internal/serve.go` — Extend tool registration loop (~30 LOC)
- `internal/plugins/registry.go` — Implement or extend `MCPTools()` method
- `internal/serve_test.go` — Add 3 tests
- `testdata/scripts/plugin_mcp.txt` — NEW, E2E test

**Test cases:**
1. Plugin tool appears in `tools/list` with `sin_plugin_<name>_<tool>` prefix
2. `tools/call` invokes plugin binary, returns stdout as result
3. Missing binary → MCP error code `-32603`

**Code sketch:**

```go
// internal/serve.go (inside registerTools)
for _, plugin := range pluginRegistry.All() {
    for _, tool := range plugin.Tools {
        mcpName := fmt.Sprintf("sin_plugin_%s_%s", plugin.Name, tool.Name)
        mcpTool := mcp.Tool{
            Name:        mcpName,
            Description: tool.Description,
            InputSchema: json.RawMessage(tool.Schema),
        }
        mcpServer.AddTool(mcpTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
            argsJSON, _ := json.Marshal(req.Params.Arguments)
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

---

### Phase 3: Documentation Update — 1 hour

**Files:**
- `docs/plugin-system-design.md` — Add 2 worked examples:
  - Example 1: Plugin with `[[hooks]]` (post_complete → Slack notification)
  - Example 2: Plugin with `[[tools]]` (slack-notify callable via MCP)
- `CHANGELOG.md` — Add v2.5.0 entry
- `AGENTS.md` — Update plugin manifest section

---

### Phase 4: Live Testing — 1 hour

1. Write a real plugin (`examples/plugins/slack-notify/`):
   - Bash script with `[[hooks]]` and `[[tools]]` in manifest
   - Hook: `post_complete` → echoes "Slack: st-xyz done"
   - Tool: `slack-notify` → echoes "Sent to #channel"
2. Install it: `cp -r examples/plugins/slack-notify/ ~/.local/share/sin-code/plugins/`
3. Test hook: `sin-code todo add "test"; sin-code todo complete st-test1` → verify hook fires
4. Test tool: start `sin-code serve`, hit `tools/call` with `sin_plugin_slack-notify_notify` → verify response

---

## Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Hook blocks todo action (sync vs async) | High | All hook executions have timeout; failures are logged but don't propagate |
| Plugin tool exec is slow, blocks MCP client | Medium | `exec.CommandContext` with 30s default timeout (configurable per-tool) |
| Plugin binary has malware | High | Document risk; add to security roadmap (separate ADR for plugin signing) |
| Plugin manifest parse error breaks startup | Medium | Validate manifest at load time; skip bad plugins with warning, not fatal error |
| `[[hooks]]` event name typo (e.g. "post_complet") | Low | Validate event names against known list at load time |

---

## Testing Strategy

### Unit Tests (8 new)

**`internal/plugins/registry_test.go`:**
- `TestLoadManifestValid` — happy path
- `TestLoadManifestInvalid` — bad TOML
- `TestHooksForEvent` — returns correct hooks (added in v2.5.0)

**`internal/todo/plugin_hooks_test.go` (NEW):**
- `TestFirePluginHooksSuccess` — hook fires, returns expected stdout
- `TestFirePluginHooksMissingBinary` — graceful skip
- `TestFirePluginHooksTimeout` — graceful timeout
- `TestFirePluginHooksNoHooks` — no-op when no hooks registered

**`internal/serve_test.go`:**
- `TestPluginToolRegistered` — tool appears in `tools/list`
- `TestPluginToolCallSuccess` — happy path
- `TestPluginToolCallMissingBinary` — error path

### E2E Testscripts (2 new)

**`testdata/scripts/plugin_hooks.txt`:**
```
# Create a dummy plugin
mkdir $WORK/dummy-plugin
cat > $WORK/dummy-plugin/plugin.toml <<EOF
[plugin]
name = "dummy"
version = "0.1.0"
[[hooks]]
event = "post_complete"
exec = "$WORK/dummy-plugin/hook.sh"
EOF
cat > $WORK/dummy-plugin/hook.sh <<EOF
#!/bin/sh
echo "hook fired for $1"
EOF
chmod +x $WORK/dummy-plugin/hook.sh

# Install plugin
mkdir $WORK/home/.local/share/sin-code/plugins
cp -r $WORK/dummy-plugin $WORK/home/.local/share/sin-code/plugins/

# Trigger
sin-code todo add "test hook"
sin-code todo complete st-test1
stdout 'hook fired'
```

**`testdata/scripts/plugin_mcp.txt`:**
```
# Similar setup, then:
sin-code serve --list-tools
stdout 'sin_plugin_dummy_echo'

# Use mcp client to call tool
mcp_call_tool 'sin_plugin_dummy_echo' '{"message":"hi"}'
stdout 'echo: hi'
```

---

## Definition of Done (v2.5.0)

- [ ] All 8 unit tests pass
- [ ] Both E2E testscripts pass
- [ ] `go test ./...` green (no regressions)
- [ ] `go test -race ./...` green
- [ ] Live test with `examples/plugins/slack-notify/` works
- [ ] `docs/plugin-system-design.md` updated with examples
- [ ] `CHANGELOG.md` v2.5.0 entry
- [ ] Git tag `v2.5.0` created
- [ ] Release notes mention "Plugin system: hooks + MCP tools"

---

## Open Questions

1. **Should hook output be returned to the user?** (e.g. `sin-code todo complete` could print hook stdout)
   - Recommendation: NO — hooks are silent, only audit log records output
   - Rationale: keeps user-facing commands clean; debug via `sin-code todo audit`

2. **Can plugins register custom hook events?** (e.g. `my-plugin.fire`)
   - Recommendation: NO in v2.5.0; defer to v3.0 if there's demand
   - Rationale: limits scope; 14 known events are enough for now

3. **Should plugin tools require permission grant on first use?**
   - Recommendation: NO in v2.5.0; trust the user (they installed the plugin)
   - Rationale: adds friction; security roadmap covers this

---

## Timeline

| Phase | Effort | ETA |
|-------|--------|-----|
| Phase 1: Hook wiring | 2-3h | Day 1 |
| Phase 2: Tools → MCP | 3-4h | Day 1-2 |
| Phase 3: Docs | 1h | Day 2 |
| Phase 4: Live test | 1h | Day 2 |
| Buffer (review, fixes) | 2h | Day 2-3 |
| **Total** | **1-2 days** | **3 working days from start** |

---

## References

- Original design: `docs/plugin-system-design.md`
- Plugin registry: `internal/plugins/registry.go`
- Hook config: `internal/todo/hooks.go`
- MCP server: `internal/serve.go`
- Issue tracker: bbolt store at `/tmp/sinator-issues.db`
