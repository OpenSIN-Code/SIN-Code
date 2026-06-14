# `serve_extra_handlers.doc.md` — MCP Tool Handlers for v2+ Subcommands

Implements the MCP tool handlers that dispatch to the v2.0+ sin-code
subcommands: todo, memory, notifications, orchestrator, agent, and lsp.

## What it does

- Converts MCP tool arguments into cobra-style CLI arguments.
- Dispatches each request to the corresponding `sin-code` subcommand via
  `runSinCodeCLI`.
- Returns the subcommand stdout as MCP text content (or an error result if the
  subcommand fails).

## Files that import / touch it

- `cmd/sin-code/internal/serve.go` — `registerAllMCPTools` wires these handlers
  into the MCP server.
- `cmd/sin-code/internal/serve_extra_handlers_test.go` — unit tests for each
  handler's argument parsing and dispatch.
- `cmd/sin-code/internal/serve_resolve_test.go` — tests for `runSinCodeCLI` and
  the binary resolver it uses.

## Important config values & limits

- `runSinCodeCLI` uses `resolveBinary()` to find the sin-code binary.
- A 5-minute `CommandContext` timeout guards against runaway subprocesses.
- Most handlers validate required arguments (e.g., todo title, memory insight,
  orchestrator prompt) before dispatching.

## Usage examples

These handlers are not called directly by users; they are invoked by MCP clients
through the `sin-code` server as tools like `sin_todo_add`, `sin_memory_search`,
`sin_notifications_list`, `sin_orchestrator_run`, `sin_agent_doctor`, and
`sin_lsp_servers`.

## Known caveats / footguns

- `runSinCodeCLI` returns subcommand errors as Go errors, which the MCP wrapper
  converts into `IsError=true` tool results. This is different from
  `runSubcommandRaw` in `serve.go`, which embeds errors in the output string.
- The fake-binary test pattern relies on `SIN_CODE_BIN` being set so tests do
  not try to run the Go test runner as `sin-code`.
- `memoryOpenFunc` and `notificationsOpenFunc` are package-level hooks that
  default to `memory.Open` and `notifications.Open`. They are intentionally
  unexported so tests in the same package can inject failures and mock stores
  without changing the public API or the production filesystem layout.
