# `src/simone_mcp/schemas.py` — Pydantic Data Models

Partner file: `src/simone_mcp/schemas.py`

## Purpose
Pydantic v2 data models for JSON-RPC requests, tool call parameters, and all argument types. Validates MCP protocol payloads and tool arguments with strict type checking.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `JsonRpcRequest` | Pydantic model | JSON-RPC 2.0 request validation |
| `JsonRpcResponse` | Pydantic model | JSON-RPC 2.0 response structure |
| `ToolCallParams` | Pydantic model | Tool call parameters (name + arguments) |
| `MessagePart` | Pydantic model | A2A message part |
| `A2AMessage` | Pydantic model | A2A message container |
| `MessageSendParams` | Pydantic model | message/send parameters |
| `FindSymbolArgs` | Pydantic model | find_symbol arguments |
| `FindReferencesArgs` | Pydantic model | find_references arguments |
| `ReplaceSymbolBodyArgs` | Pydantic model | replace_symbol_body arguments |
| `InsertAfterSymbolArgs` | Pydantic model | insert_after_symbol arguments |
| `ProjectOverviewArgs` | Pydantic model | project_overview arguments |
| `SymbolSearchArgs` | Pydantic model | symbol_search arguments |
| `StructuralEditArgs` | Pydantic model | structural_edit arguments |
| `MemoryQueryArgs` | Pydantic model | memory_query arguments |
| `GraphifyQueryArgs` | Pydantic model | graphify_query arguments |
| `GraphifyUpdateArgs` | Pydantic model | graphify_update arguments |
| `GraphifyExplainArgs` | Pydantic model | graphify_explain arguments |
| `GraphifyPathArgs` | Pydantic model | graphify_path arguments |
| `TaskGetArgs` | Pydantic model | tasks/get arguments |
| `TaskUpdateArgs` | Pydantic model | tasks/update arguments |
| `TaskCancelArgs` | Pydantic model | tasks/cancel arguments |
| `TOOL_ARG_MODELS` | dict | Mapping of tool names to argument models |

## Validation Rules
- All string fields have `min_length=1` (non-empty)
- `jsonrpc` field must be exactly `"2.0"`
- `taskId` accepts `id` alias via `populate_by_name`

## Relationship
- `src/simone_mcp/protocol.py` — `TOOL_ARG_MODELS` used for argument validation in `tools/call`
- `src/simone_mcp/a2a_handler.py` — `JsonRpcRequest`, `ToolCallParams`, `MessageSendParams`
- `tests/test_simone_mcp.py` — tests schema validation

## Dependencies
- `pydantic`: BaseModel, Field, model_validator
- Standard lib: `typing.Literal`

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
