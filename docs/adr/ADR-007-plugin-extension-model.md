# ADR-007: Plugin Extension Model — TOML Manifest + Subprocess Execution

| Field       | Value                                    |
|-------------|------------------------------------------|
| ADR         | ADR-007                                  |
| Status      | Accepted                                 |
| Date        | 2026-06-08                               |
| Deciders    | jeremy                                   |
| Supersedes  | (none)                                   |
| Related     | docs/plans/plugin-system-completion.md   |
| Plan        | [v2.5.0 Plugin System Completion](../plans/plugin-system-completion.md) |

## Context

The sin-code binary needs to be **user-extensible** without requiring a rebuild from source. Two competing models were considered:

**Model A: In-process Go plugin (`go plugin` package)**
- Plugins compiled as `.so` files, loaded with `plugin.Open()`
- Tight integration: plugins can call internal APIs directly
- **Drawback 1:** Go plugins are notoriously fragile — version mismatch between plugin and host causes silent failures
- **Drawback 2:** `go plugin` is officially "mostly works, but no promises" per the Go team
- **Drawback 3:** Cross-compilation is nightmarish (must build for exact target)
- **Drawback 4:** Hard to ship plugins via brew/apt — need a Go toolchain on the user's machine

**Model B: Out-of-process subprocess (TOML manifest + exec)**
- Plugins defined by a `plugin.toml` manifest in `~/.local/share/sin-code/plugins/<name>/`
- Manifest declares subcommands, agents, tools, hooks — each pointing to an `exec` binary
- sin-code spawns the binary as a subprocess and communicates via JSON over stdin/stdout
- **Drawback:** Higher per-call latency (~5-20ms for fork+exec)
- **Drawback:** Plugins can't share in-memory state with sin-code
- **Advantage:** Robust, debuggable, language-agnostic (plugins can be any executable)
- **Advantage:** No version coupling — if a plugin's binary is broken, the host doesn't crash
- **Advantage:** Easy to distribute (just copy a directory to the plugins folder)

## Decision

We adopt **Model B: out-of-process subprocess** with TOML manifest.

### Manifest Format

```toml
[plugin]
name = "my-plugin"
version = "0.1.0"
description = "What this plugin does"
author = "..."

[[subcommand]]
name = "my-cmd"
short = "Short description"
exec = "my-plugin-cmd"

[[agents]]
name = "my-agent"
provider = "openai"
model = "gpt-4o"
system_file = "system.txt"  # relative to plugin dir
tools_allow = ["sin_*"]      # pattern, allows all sin_* tools
tools_deny = ["sin_todo_*"]  # explicitly denied

[[tools]]
name = "my-tool"
description = "Callable via MCP"
schema = '{"type": "object", "properties": {"input": {"type": "string"}}}'
exec = "my-plugin-tool"

[[hooks]]
event = "post_complete"
exec = "my-plugin-notify"
timeout_seconds = 30
```

### Plugin Discovery

- Scan `~/.local/share/sin-code/plugins/*/plugin.toml` at startup
- Parse each manifest into a `Plugin` struct
- Validate at load time (bad TOML → skip with warning, not fatal)
- Plugins with conflicting names → first-wins, log warning

### Execution Model

- **Subcommands:** `os/exec` the `exec` binary with the same args the user typed
- **Agents:** Spawn the `exec` binary per-task, pipe JSON via stdin, read JSON via stdout
- **Tools (MCP):** Spawn per-call with JSON args via stdin, return stdout as MCP result
- **Hooks:** Fire-and-forget by default, but capture stdout/stderr to audit log

### Resource Limits

- All `exec` calls have a configurable timeout (default: 30s)
- Plugin processes are NOT sandboxed (out of scope for this ADR — see Security roadmap)
- Plugin binaries are expected to be at `$PATH` or in the plugin's own directory

## Consequences

### Positive

1. **Robustness** — a plugin crash doesn't take down sin-code
2. **Distribution** — plugins are directories, easy to share via git/curl
3. **Language-agnostic** — users can write plugins in Python, Rust, bash, anything
4. **Debuggability** — `strace` works on plugin processes
5. **No version coupling** — host and plugins evolve independently
6. **Fast iteration** — write a plugin without rebuilding sin-code

### Negative

1. **Latency** — ~5-20ms per plugin invocation (fork+exec)
   - Mitigation: accept this; for high-throughput use cases, plugins are the wrong tool
2. **No shared memory** — plugins can't read sin-code's bbolt stores directly
   - Mitigation: plugins can call `sin-code` itself as a subprocess to read state
3. **No type safety** — TOML parsing + JSON over stdin is untyped
   - Mitigation: schema field in `[[tools]]` provides runtime validation
4. **No hot-reload** — plugin changes require sin-code restart
   - Mitigation: acceptable for v2.5.0; defer to v3.0+
5. **Security** — plugins run with user's full permissions
   - Mitigation: documented in AGENTS.md; plugin signing is a separate ADR (future)

## Alternatives Considered

### Model A: In-process Go plugin
**Rejected** — fragility outweighs the perf benefit. 5ms latency is not a problem in practice.

### Model C: WebAssembly plugins (e.g. wazero, wasmtime)
**Considered, deferred** — Wasm would give us sandboxing + portability, but adds significant complexity:
- Need to design a C ABI for plugin ↔ host
- Limited stdlib support in most wasm runtimes
- Harder to write plugins (Rust/C, not bash/Python)
**Defer to v3.0+** if security becomes a real issue.

### Model D: Lua scripts (e.g. gopher-lua)
**Considered, rejected** — Lua is simple but the ecosystem is tiny. TOML + subprocess is more idiomatic and lets users reuse their existing skills (Python, bash, Go).

## References

- Initial design: `docs/plugin-system-design.md` (v2.3.0)
- Implementation: `internal/plugins/registry.go` (v2.3.0)
- Plan: [docs/plans/plugin-system-completion.md](../plans/plugin-system-completion.md) (v2.5.0)
- Issues: [st-phw1](../issues/st-phw1-plugin-hook-wiring.md), [st-ptm2](../issues/st-ptm2-plugin-tools-mcp.md)
