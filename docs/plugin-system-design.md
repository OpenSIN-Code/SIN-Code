# SIN-Code Plugin System Design

## Overview

The SIN-Code Plugin System extends the MCP server with dynamically discoverable, sandboxed tools that run as isolated processes and communicate via JSON-RPC. This design enables third-party and community-contributed tools without compromising the core server stability or security.

## Goals

- **Extensibility**: Allow community and private plugins to extend `sin-code` without modifying the core codebase.
- **Isolation**: Each plugin runs as a separate OS process with restricted filesystem and network access.
- **Dynamic Discovery**: Plugins are automatically discovered from well-known directories at server startup and optionally reloaded at runtime.
- **Standards Compliance**: Plugins expose tools using the same MCP JSON-RPC interface as the server itself, minimizing integration friction.
- **Security by Default**: Plugins are sandboxed and must explicitly opt in to any privileged access.

---

## Plugin Architecture

### 1. Plugin Discovery

`sin-code` discovers plugins from the following sources, in order of precedence:

1. **User plugins directory** (default): `~/.config/sin/plugins/`
2. **System plugins directory** (admin/root): `/etc/sin/plugins/` (Linux), `/usr/local/share/sin/plugins/` (macOS), `%ProgramData%\sin\plugins\` (Windows)
3. **Project-local plugins directory**: `.sin/plugins/` inside the working directory
4. **Configured directories**: Additional paths from `sin-code config`:
   ```bash
   sin-code config set plugin_dirs.extra /path/to/more/plugins
   ```

Each plugin resides in its own subdirectory (e.g., `~/.config/sin/plugins/csv-processor/`). The directory name does not need to match the plugin `name`, but must be unique.

**Discovery Rules**:
- A valid plugin directory contains a `plugin.json` manifest file.
- Directories without `plugin.json` are silently skipped.
- Subdirectories are scanned recursively **one level deep** (to allow grouping or versioning), but `plugin.json` must be in the leaf directory.
- Symlinks are followed for convenience, but cycle detection is applied.
- The discovery order is deterministic: user → system → project-local → configured extras. In case of duplicate plugin names, the first discovered wins, and a warning is logged.

### 2. Plugin Manifest (`plugin.json`)

Each plugin MUST declare a manifest file with the following schema:

```json
{
  "$schema": "https://sin-code.dev/schemas/plugin-manifest/v1",
  "name": "csv-processor",
  "version": "1.0.0",
  "description": "Process CSV files with filtering and aggregation",
  "entry_point": "python3 ~/.config/sin/plugins/csv-processor/main.py",
  "tools": [
    {
      "name": "csv_filter",
      "description": "Filter CSV rows by column value",
      "input_schema": {
        "type": "object",
        "properties": {
          "file": {"type": "string", "description": "Path to CSV file"},
          "column": {"type": "string", "description": "Column name to filter"},
          "value": {"type": "string", "description": "Value to match"}
        },
        "required": ["file", "column", "value"]
      }
    },
    {
      "name": "csv_aggregate",
      "description": "Aggregate CSV rows by column",
      "input_schema": {
        "type": "object",
        "properties": {
          "file": {"type": "string", "description": "Path to CSV file"},
          "group_by": {"type": "string", "description": "Column to group by"},
          "operation": {"type": "string", "enum": ["sum", "count", "avg"], "description": "Aggregation operation"}
        },
        "required": ["file", "group_by", "operation"]
      }
    }
  ],
  "hooks": {
    "init": "init",
    "shutdown": "shutdown",
    "health_check": "health"
  },
  "sandbox": {
    "filesystem_access": "read_only",
    "allowed_paths": ["/tmp", "~/.cache/sin"],
    "network": false,
    "max_memory_mb": 256,
    "max_cpu_percent": 50,
    "timeout_seconds": 30
  }
}
```

**Field Descriptions**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Unique plugin identifier (kebab-case, max 64 chars). Must be unique across all loaded plugins. |
| `version` | `string` | Yes | SemVer version string. |
| `description` | `string` | Yes | Human-readable summary (max 256 chars). |
| `entry_point` | `string` | Yes | Command to launch the plugin process. May use absolute paths, `~` expansion, or a command resolvable via `$PATH`. |
| `tools` | `array` | Yes | List of tool definitions exposed by this plugin. |
| `tools[].name` | `string` | Yes | Tool name. Must be unique within the plugin. The fully-qualified tool name registered in the MCP server is `{plugin_name}/{tool_name}`. |
| `tools[].description` | `string` | Yes | Tool description for LLM/Agent consumption. |
| `tools[].input_schema` | `object` | Yes | JSON Schema for the tool's input parameters (MCP-compatible). |
| `hooks` | `object` | No | Lifecycle hook mapping. Keys: `init`, `shutdown`, `health_check`. Values are method names sent via JSON-RPC. |
| `sandbox` | `object` | No | Security and resource constraints. Defaults are applied if omitted. |

**Sandbox Defaults**:
- `filesystem_access`: `"none"` (options: `"none"`, `"read_only"`, `"read_write"`)
- `allowed_paths`: `[]` (only meaningful if `filesystem_access` is not `"none"`)
- `network`: `false`
- `max_memory_mb`: `128`
- `max_cpu_percent`: `25`
- `timeout_seconds`: `30`

### 3. Plugin Isolation & Process Model

Each plugin runs as a **separate OS process**. The core MCP server (`sin-code serve`) spawns the process defined by `entry_point` and communicates with it over **stdin/stdout** using the same JSON-RPC message format as the MCP protocol itself.

**Process Lifecycle**:
1. **Discovery Phase**: Server scans directories and collects manifest files. No processes are started yet.
2. **Registration Phase**: For each valid manifest, the server registers the plugin's tools in the MCP tool namespace. The tool names are prefixed: `sin_csv_filter` (or `csv-processor/csv_filter` depending on naming strategy — see Phase 4).
3. **Initialization Phase**: On first tool invocation, the server lazily starts the plugin process. The `init` hook is called if defined. If `init` fails, the plugin is marked `unhealthy` and subsequent calls return an error.
4. **Runtime Phase**: The server maintains a pool of plugin processes. For each active plugin, a single long-running process is kept. Concurrent tool calls from the same plugin are multiplexed over the same JSON-RPC connection using `id` correlation.
5. **Health Monitoring**: A background goroutine periodically calls the `health_check` hook (default interval: 30s). If the plugin process crashes or the health check fails, the server attempts to restart it up to `max_restarts` (default: 3) within a cooldown window, after which the plugin is marked `failed`.
6. **Shutdown Phase**: On server shutdown or explicit plugin unload, the server sends the `shutdown` hook, waits for graceful termination (up to 5s), then sends `SIGTERM` and finally `SIGKILL` if needed.

**JSON-RPC Message Format**:

The server and plugin communicate with standard JSON-RPC 2.0 over stdin/stdout:

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "method": "csv_filter",
  "params": {
    "file": "/tmp/data.csv",
    "column": "status",
    "value": "active"
  }
}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "result": {
    "matched_rows": 150,
    "output_file": "/tmp/filtered.csv"
  }
}
```

Error:

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "error": {
    "code": -32000,
    "message": "File not found: /tmp/data.csv"
  }
}
```

**Plugin SDK** (Reference Implementation):

A Python SDK simplifies plugin development:

```python
from sin_plugin_sdk import Plugin, tool

plugin = Plugin(name="csv-processor")

@tool(name="csv_filter", description="Filter CSV rows by column value")
def csv_filter(file: str, column: str, value: str) -> dict:
    """Implementation here."""
    ...

if __name__ == "__main__":
    plugin.run()
```

The SDK handles stdin/stdout JSON-RPC framing, schema validation, and lifecycle hooks automatically.

### 4. Plugin Registration & Tool Namespacing

When the MCP server loads a plugin, its tools are registered dynamically into the server's tool registry. The fully-qualified tool name follows this pattern:

```
{plugin_name}_{tool_name}
```

For example, the `csv_filter` tool from the `csv-processor` plugin becomes `sin_csv_filter` or `csv_processor_csv_filter`. The exact prefix strategy is configurable:

- **Option A (default)**: Prefix with `sin_` and flatten: `sin_csv_filter` (if no collision with core tools). If a collision exists, use fully qualified: `sin_csv_processor_csv_filter`.
- **Option B (namespace)**: Always use `plugin_name/tool_name`: `csv-processor/csv_filter`. This is unambiguous but requires MCP clients to support namespaced tool names.
- **Option C (alias)**: Plugin manifest may define `alias` for each tool to provide a short, user-friendly name.

The server exposes plugin tools alongside native tools in `tools/list` and `tools/call` responses. The `meta` field of each tool indicates its origin: `{"plugin": "csv-processor", "version": "1.0.0"}`.

### 5. Security & Sandboxing

**Security is the highest priority for the plugin system.** The following mechanisms enforce isolation:

#### 5.1 Filesystem Sandboxing

- **No access (default)**: The plugin process cannot access any host filesystem paths. All data must be passed in the JSON-RPC `params` or returned in `result`.
- **Read-only**: Plugin may read files from `allowed_paths` only. All other paths return `EPERM`.
- **Read-write**: Plugin may read and write to `allowed_paths`. Writes are restricted to those paths.

Implementation: On Linux, this is enforced via `landlock` or `seccomp-bpf` + `mount namespaces`. On macOS, `sandbox-exec` profiles are used. On Windows, AppContainer / Windows Sandbox APIs are used. If OS-level sandboxing is unavailable, the server falls back to a restrictive `chroot` + `seccomp` (Linux) or a warning + process-level monitoring.

#### 5.2 Network Sandboxing

- **Default**: Network access is disabled. Any `socket` syscall is blocked by the sandbox.
- **Opt-in**: If `sandbox.network: true`, the plugin may access the network. This is logged as a security event. The server may apply further restrictions (e.g., allowlist of domains via `allowed_hosts`).

#### 5.3 Resource Limits

- **Memory**: `max_memory_mb` is enforced via `setrlimit(RLIMIT_AS)` or cgroup limits (Linux), `memorylimit` (macOS), or job objects (Windows).
- **CPU**: `max_cpu_percent` is a soft limit enforced via `cpulimit` (Linux) or `task_policy` (macOS). Exceeding it causes throttling, not termination.
- **Timeout**: Each tool call has a `timeout_seconds` limit. If exceeded, the plugin process is killed and the server returns a `timeout` error.
- **Max Restarts**: If a plugin crashes more than 3 times in 60 seconds, it is marked `failed` and disabled until manual re-enable or server restart.

#### 5.4 Code Signing & Trust (Future)

- **Phase 5+**: Plugins may be signed with a private key. The server verifies the signature against a trust store before loading. Unsigned plugins require explicit user confirmation (`--allow-unsigned-plugins`).
- **Hash verification**: The server may store SHA-256 hashes of known-good plugin manifests/binaries and warn if they change.

#### 5.5 Audit Logging

Every plugin lifecycle event and every tool invocation is logged to the server audit log:

```json
{
  "timestamp": "2025-01-01T12:00:00Z",
  "event": "plugin.tool_invoked",
  "plugin": "csv-processor",
  "tool": "csv_filter",
  "client_id": "pid:12345",
  "duration_ms": 45,
  "status": "success"
}
```

---

## Implementation Phases

### Phase 1: Plugin Discovery & Manifest Loading (MVP)

- [ ] Implement directory scanning in `internal/plugins/discover.go`.
- [ ] Define `PluginManifest` struct matching the JSON schema above.
- [ ] Validate manifest JSON (schema check, required fields, name uniqueness).
- [ ] Add `sin-code plugins list` CLI command to list discovered plugins without executing them.
- [ ] Add `sin-code plugins validate <path>` CLI command to validate a manifest.
- [ ] Unit tests: discovery edge cases, invalid manifests, duplicate names, symlink cycles.

**Goal**: Users can drop plugin directories and see them listed.

### Phase 2: Plugin Process Management

- [ ] Implement `PluginProcess` struct in `internal/plugins/process.go` that wraps `exec.Cmd`.
- [ ] Lazy start on first tool invocation.
- [ ] Health check loop (goroutine per plugin).
- [ ] Crash detection + restart with backoff (exponential, max 3 attempts).
- [ ] Graceful shutdown on server exit.
- [ ] Add `sin-code plugins start <name>` and `sin-code plugins stop <name>` commands.
- [ ] Unit tests: process lifecycle, crash restart, zombie cleanup.

**Goal**: Plugin processes are managed reliably.

### Phase 3: JSON-RPC Bridge

- [ ] Implement JSON-RPC codec over stdin/stdout (`internal/plugins/rpc.go`).
- [ ] Message framing: length-prefixed or newline-delimited JSON (TBD).
- [ ] Correlation ID mapping between server request IDs and plugin request IDs.
- [ ] Support for `notifications` (e.g., `progress` from plugin to server).
- [ ] Timeout handling for individual requests.
- [ ] Unit tests: round-trip request/response, error propagation, timeout.

**Goal**: Server can send tool calls to a plugin and receive results.

### Phase 4: Tool Schema Registration & Call Forwarding

- [ ] Integrate plugin tools into the MCP server tool registry (`internal/mcp/tools.go`).
- [ ] Prefix naming strategy (Option A default).
- [ ] Implement `tools/list` to include plugin tools with `meta` annotations.
- [ ] Implement `tools/call` to route to the correct plugin process based on tool name.
- [ ] Schema validation: validate incoming params against the plugin's `input_schema` before forwarding (or let plugin handle it — configurable).
- [ ] Add `sin-code plugins reload` to hot-reload plugin manifests (restart processes).
- [ ] Unit tests: tool registration, routing, schema validation, error handling.

**Goal**: Plugin tools are fully functional alongside native tools.

### Phase 5: Security Sandboxing

- [ ] Implement OS-specific sandboxing backends:
  - Linux: `landlock` (filesystem) + `seccomp` (network + syscalls) + cgroups (resources).
  - macOS: `sandbox-exec` profile + `memorylimit` + `task_policy`.
  - Windows: AppContainer + Job Objects.
- [ ] Fallback: `chroot` + `seccomp` (Linux) or warning-only mode.
- [ ] Implement `allowed_paths` resolution (expand `~`, resolve symlinks, validate within allowed set).
- [ ] Add `--allow-unsigned-plugins` flag and trust store configuration.
- [ ] Security audit: fuzz plugin JSON-RPC, test sandbox escape attempts.
- [ ] Integration tests: filesystem access denied, network blocked, resource limits enforced.

**Goal**: Plugins are isolated and secure by default.

---

## Example: End-to-End Plugin Development

### Step 1: Create Plugin Directory

```bash
mkdir -p ~/.config/sin/plugins/csv-processor
```

### Step 2: Write `plugin.json`

```json
{
  "name": "csv-processor",
  "version": "1.0.0",
  "description": "Process CSV files with filtering and aggregation",
  "entry_point": "python3 ~/.config/sin/plugins/csv-processor/main.py",
  "tools": [
    {
      "name": "csv_filter",
      "description": "Filter CSV rows by column value",
      "input_schema": {
        "type": "object",
        "properties": {
          "file": {"type": "string", "description": "Path to CSV file"},
          "column": {"type": "string", "description": "Column name to filter"},
          "value": {"type": "string", "description": "Value to match"}
        },
        "required": ["file", "column", "value"]
      }
    }
  ],
  "sandbox": {
    "filesystem_access": "read_only",
    "allowed_paths": ["/tmp"],
    "network": false
  }
}
```

### Step 3: Write Plugin Code (`main.py`)

```python
import json
import sys
import csv

def read_request():
    line = sys.stdin.readline()
    if not line:
        return None
    return json.loads(line)

def write_response(response):
    sys.stdout.write(json.dumps(response) + "\n")
    sys.stdout.flush()

while True:
    req = read_request()
    if req is None:
        break
    method = req.get("method")
    params = req.get("params", {})
    req_id = req.get("id")

    if method == "csv_filter":
        file_path = params["file"]
        column = params["column"]
        value = params["value"]
        matched = 0
        with open(file_path, "r") as f:
            reader = csv.DictReader(f)
            for row in reader:
                if row.get(column) == value:
                    matched += 1
        write_response({
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {"matched_rows": matched}
        })
    else:
        write_response({
            "jsonrpc": "2.0",
            "id": req_id,
            "error": {"code": -32601, "message": f"Method not found: {method}"}
        })
```

### Step 4: Verify Discovery

```bash
sin-code plugins list
# Output: csv-processor v1.0.0 — Process CSV files with filtering and aggregation
```

### Step 5: Use via MCP

```bash
sin-code serve
# Client calls: sin_csv_filter(file="/tmp/data.csv", column="status", value="active")
```

---

## Open Questions & Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| JSON-RPC framing: newline-delimited vs. length-prefixed? | Newline-delimited JSON (NDJSON) | Simpler, human-readable, works with `sys.stdin.readline()` in most languages. |
| Tool naming: flat vs. namespaced? | Flat with collision fallback | Flat names are easier for LLM consumption. Namespaces are verbose. |
| Sandbox: `landlock` vs `seccomp-bpf`? | `landlock` for filesystem, `seccomp` for network | `landlock` is newer and purpose-built for path-based access control. |
| Plugin reload: hot or restart? | Hot-reload for manifests, restart for code | Manifest changes are common; code changes require process restart. |
| Plugin configuration: manifest-only or external config? | Manifest-only for MVP, `.sin/plugins/{name}.json` for user overrides | Keep it simple. Overrides can be added later. |

---

## Dependencies

- **Go 1.22+** (for `landlock` bindings via `golang.org/x/sys/unix`).
- **Linux 5.13+** (for `landlock` support). macOS/Windows use platform-specific alternatives.
- No external services or databases required.

---

## Appendix: JSON Schema for `plugin.json`

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["name", "version", "description", "entry_point", "tools"],
  "properties": {
    "name": {"type": "string", "pattern": "^[a-z0-9-]+$", "maxLength": 64},
    "version": {"type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+.*$"},
    "description": {"type": "string", "maxLength": 256},
    "entry_point": {"type": "string", "minLength": 1},
    "tools": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["name", "description", "input_schema"],
        "properties": {
          "name": {"type": "string", "pattern": "^[a-z0-9_]+$", "maxLength": 64},
          "description": {"type": "string", "maxLength": 256},
          "input_schema": {"type": "object"}
        }
      }
    },
    "hooks": {
      "type": "object",
      "properties": {
        "init": {"type": "string"},
        "shutdown": {"type": "string"},
        "health_check": {"type": "string"}
      }
    },
    "sandbox": {
      "type": "object",
      "properties": {
        "filesystem_access": {"type": "string", "enum": ["none", "read_only", "read_write"]},
        "allowed_paths": {"type": "array", "items": {"type": "string"}},
        "network": {"type": "boolean"},
        "max_memory_mb": {"type": "integer", "minimum": 1},
        "max_cpu_percent": {"type": "integer", "minimum": 1, "maximum": 100},
        "timeout_seconds": {"type": "integer", "minimum": 1}
      }
    }
  }
}
```
