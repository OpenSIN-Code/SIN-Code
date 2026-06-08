# Issue: st-phw1 — Plugin hooks → todo event wiring

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-phw1                                                     |
| Title       | Plugin hooks → todo event wiring                            |
| Status      | done                                                        |
| Priority    | P0 (blocks v2.5.0 release)                                  |
| Created     | 2026-06-08T13:50:00Z                                        |
| Reporter    | jeremy (via sin-code v2.4.0 audit)                          |
| Plan        | [docs/plans/plugin-system-completion.md](../plans/plugin-system-completion.md) |
| Component   | plugins, todo                                               |
| Effort      | 2-3 hours                                                   |
| Blocks      | st-ptm2 (plugin tools → MCP)                                |

## Summary

The plugin manifest format defines a `[[hooks]]` section in `docs/plugin-system-design.md` and the plugin registry has a `HookConfig` data type, but the runtime wiring from todo events (post_add, post_complete, pre_complete) to plugin-defined hook executables is incomplete.

**Subagent 2** started the wiring in `internal/todo/plugin_hooks.go` (function `firePluginHooks`) but the code path is only half-connected — the hooks fire but their return values are not fed back into the todo state, and there's no graceful skip when a plugin binary is missing.

## Symptoms

- `sin-code todo add ...` does NOT invoke `[[hooks]]` from any loaded plugin
- `sin-code todo complete ...` does NOT invoke `post_complete` hooks
- No error message when a plugin's `[[hooks]]` binary is missing on PATH
- Audit log (`sin-code todo audit`) does not record hook invocations

## Expected Behavior (per design)

1. User installs a plugin at `~/.local/share/sin-code/plugins/my-plugin/plugin.toml`
2. Plugin manifest contains:
   ```toml
   [[hooks]]
   event = "post_complete"
   exec = "my-plugin-notify"
   timeout_seconds = 30
   ```
3. User runs `sin-code todo complete st-xyz`
4. `firePluginHooks` finds the `post_complete` hook, spawns `my-plugin-notify <todo-id>`, waits up to 30s
5. Hook stdout/stderr is captured in the audit log
6. If the binary is missing, the error is logged but the `complete` action still succeeds

## Acceptance Criteria

- [ ] `firePluginHooks` is called from `todo.complete()` and `todo.add()`
- [ ] Hooks are discovered by globbing `~/.local/share/sin-code/plugins/*/plugin.toml` and parsing `[[hooks]]`
- [ ] Missing binary → `fmt.Fprintf(os.Stderr, "plugin hook %s/%s: binary not found: %s\n", ...)` and continue
- [ ] Hook stdout (one JSON line) is parsed and merged into audit log entry
- [ ] `internal/plugins/registry_test.go` has a new test: `TestPluginHookFiredOnTodoEvent`
- [ ] `internal/todo/plugin_hooks_test.go` has 3 unit tests (success, missing binary, timeout)

## Files Touched

- `internal/todo/plugin_hooks.go` — extend `firePluginHooks` (already exists)
- `internal/todo/todo.go` — call `firePluginHooks` from `complete()` and `add()`
- `internal/plugins/registry.go` — add `HooksForEvent(event string) []HookConfig` method
- `internal/plugins/registry_test.go` — add hook test
- `testdata/scripts/plugin_hooks.txt` — NEW: live E2E test for plugin hooks

## Definition of Done

1. All acceptance criteria met
2. `go test ./... -count=1` green (no regressions)
3. `testdata/scripts/plugin_hooks.txt` passes
4. v2.5.0 release notes mention the fix
