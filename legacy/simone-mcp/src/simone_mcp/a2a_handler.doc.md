# `src/simone_mcp/a2a_handler.py` — A2A Protocol Handler

Partner file: `src/simone_mcp/a2a_handler.py`

## Purpose
Implements the Agent-to-Agent (A2A) JSON-RPC protocol handler. Supports agent discovery, tool listing, tool calling, message sending, and task retrieval. All responses are JSON-RPC 2.0 compliant.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `handle_a2a_request()` | async function | Main entry point for A2A requests |
| `_handle_tool_call()` | async function | Execute a tool via `execute_simone_action()` |
| `_handle_message_send()` | async function | Parse natural language or JSON actions from messages |
| `_ok_response()` | function | Build JSON-RPC success response |
| `_error_response()` | function | Build JSON-RPC error response |

## Supported Methods
- `agent.discover` — Return agent card
- `agent.ping` — Health check
- `tool.list` — List all tools
- `tool.call` — Execute a tool with correlation tracking
- `message/send` — Send natural language or JSON actions
- `tasks/get` — Get task status (always returns completed)

## Relationship
- `src/simone_mcp/core.py` — `execute_simone_action()`, `TOOL_DEFINITIONS`, `build_agent_card()`
- `src/simone_mcp/correlation.py` — `correlation_manager` for tracking calls
- `src/simone_mcp/schemas.py` — `JsonRpcRequest`, `ToolCallParams`, `MessageSendParams`
- `src/simone_mcp/http_app.py` — mounts this at `POST /a2a/v1`

## Dependencies
- `core`: `TOOL_DEFINITIONS`, `build_agent_card`, `execute_simone_action`
- `correlation`: `correlation_manager`
- `schemas`: `JsonRpcRequest`, `ToolCallParams`, `MessageSendParams`
- Standard lib: `json`, `uuid`, `datetime`

## Security Notes
- Correlation IDs are generated from SHA-256 hash of tool name + arguments + timestamp
- Tool calls are validated via Pydantic schemas before execution
- Path traversal is checked in core functions, not here

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
