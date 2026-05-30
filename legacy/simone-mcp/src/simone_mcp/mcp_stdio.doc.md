# `src/simone_mcp/mcp_stdio.py` — MCP Stdio Transport

Partner file: `src/simone_mcp/mcp_stdio.py`

## Purpose
Implements the MCP stdio transport protocol. Reads JSON-RPC lines from stdin, dispatches via `handle_mcp_request()`, and writes responses/notifications to stdout. Supports batch requests.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `serve_stdio()` | async function | Main stdio server loop |
| `_send_stdio_notification()` | async function | Write notification to stdout |

## Protocol Flow
1. Read JSON-RPC line from stdin
2. Parse payload (single or batch)
3. Call `handle_mcp_request()` for each payload
4. Write notifications to stdout immediately
5. Write response(s) to stdout

## Session Management
- `session_id` is persisted across requests in the same stdio connection
- `client_protocol_version` is captured from `initialize` requests

## Relationship
- `src/simone_mcp/protocol.py` — `handle_mcp_request()` is the core dispatcher
- `src/simone_mcp/cli.py` — `serve-mcp` command calls `serve_stdio()`

## Dependencies
- `protocol`: `handle_mcp_request`
- Standard lib: `json`, `sys`

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
