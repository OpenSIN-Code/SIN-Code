# `src/simone_mcp/core.py` — Core Code Intelligence Engine

Partner file: `src/simone_mcp/core.py`

## Purpose
The heart of Simone MCP. Provides LSP-grade code analysis tools: symbol search, reference finding, structural editing, project overview, and action execution. Supports Python, JavaScript, and TypeScript via AST parsing and tree-sitter.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `TOOL_DEFINITIONS` | list | JSON Schema 2020-12 tool definitions for MCP registration |
| `CAPABILITIES` | list | Server capability strings |
| `build_agent_card()` | function | Build A2A agent discovery card |
| `build_oauth_client_metadata()` | function | OAuth 2.1 client metadata |
| `build_authorization_server_metadata()` | function | OAuth 2.1 authorization server metadata |
| `find_symbol()` | function | Search symbol definitions across workspace |
| `find_references()` | function | Find references with jedi (LSP) or regex fallback |
| `replace_symbol_body()` | function | Safe structural edit via libcst or AST |
| `insert_after_symbol()` | function | Insert text after a symbol |
| `get_project_overview()` | function | Summarize workspace file types and counts |
| `execute_simone_action()` | function | Main action dispatcher |
| `process_lsp_task()` | function | Async LSP task wrapper |
| `dashboard()` | function | Generate HTML dashboard |
| `_build_realtime_url()` | function | Supabase realtime WebSocket URL |

## Internal Helpers
| Symbol | Purpose |
|--------|---------|
| `_candidate_files()` | Collect .py/.js/.ts files excluding blocked dirs |
| `_parse_file()` | Parse Python file to AST |
| `_extract_symbols_treesitter()` | Extract JS/TS symbols via tree-sitter |
| `_extract_symbols_js_regex()` | Fallback regex-based JS symbol extraction |
| `_find_references_jedi()` | Jedi-based reference finding |
| `_find_references_regex()` | Regex-based reference fallback |
| `_replace_symbol_body_libcst()` | libcst-based structural edit |
| `_replace_symbol_body_ast()` | AST-based structural edit fallback |
| `_validate_file_in_workspace()` | Path traversal prevention |
| `_workspace_root()` | Resolve workspace root path |

## Relationship
- `src/simone_mcp/__init__.py` — re-exports 10 public symbols from here
- `src/simone_mcp/mcp_server.py` — re-exports 10 public symbols from here
- `src/simone_mcp/cli.py` — uses `execute_simone_action()`, `get_project_overview()`, `build_agent_card()`
- `src/simone_mcp/a2a_handler.py` — uses `execute_simone_action()`, `build_agent_card()`, `TOOL_DEFINITIONS`
- `src/simone_mcp/protocol.py` — uses `execute_simone_action()`, `TOOL_DEFINITIONS`, `json_dumps()`
- `src/simone_mcp/hybrid_memory.py` — uses `_workspace_root()`
- `src/simone_mcp/graphify_service.py` — called via `execute_simone_action()` for graphify actions
- `tests/test_simone_mcp.py` — extensive tests for all core functions

## Dependencies
| Optional | Purpose |
|----------|---------|
| `libcst` | Safe structural edits (CST preserving) |
| `jedi` | LSP-grade reference finding |
| `tree-sitter` + `tree-sitter-python` | Python AST parsing |
| `tree-sitter-typescript` | JS/TS symbol extraction |
| `qdrant_client` | Vector DB validation |
| `neo4j` | Graph DB validation |

## Security
- `PathTraversalError` — raised for paths outside workspace
- `_PY_BLOCKED` — blocked directories (.git, venv, node_modules, etc.)
- `_validate_file_in_workspace()` — checks `Path.resolve().relative_to()`

## Broken Links Check
- References to `hybrid_memory.py` for `query_hybrid_memory` (import at bottom)
- References to `graphify_service.py` for graphify functions (import at bottom)
- These are late imports (line 909-917), not circular dependencies.
