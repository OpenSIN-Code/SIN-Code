# `src/simone_mcp/__init__.py` — Package Public API

Partner file: `src/simone_mcp/__init__.py`

## Purpose
Defines the public API surface of the `simone_mcp` package. All external consumers import from this module. Attaches a `NullHandler` to suppress log spam when the package is imported without logging configuration.

## Key Symbols
| Symbol | Source | Purpose |
|--------|--------|---------|
| `_build_realtime_url` | `core.py` | Supabase realtime URL builder |
| `build_agent_card` | `core.py` | A2A agent discovery card |
| `dashboard` | `core.py` | HTML dashboard generator |
| `execute_simone_action` | `core.py` | Main action dispatcher |
| `find_references` | `core.py` | Symbol reference search |
| `find_symbol` | `core.py` | Symbol definition search |
| `get_project_overview` | `core.py` | Workspace footprint summary |
| `insert_after_symbol` | `core.py` | Insert code after symbol |
| `process_lsp_task` | `core.py` | LSP task processor |
| `replace_symbol_body` | `core.py` | Replace symbol body safely |

## Relationship
- `src/simone_mcp/core.py` — all functions re-exported from here
- `src/cli.py` — uses these symbols via direct imports

## Dependencies
- `simone_mcp.core`
- `logging.NullHandler`

## Notes
`__all__` is explicitly defined to control the public API. Do not add symbols here without updating the `__all__` list.
