# `src/mcp_server.py` — MCP Server Re-exports

What this file does: re-exports the core tool functions from `simone_mcp.core` so external MCP clients (OpenCode, Claude Desktop) can import them as a flat module.

## Dependency map

- Imports: `simone_mcp.core` (10 functions).
- Imported by: client configurations (e.g. `clients/opencode-mcp.json`).

## Re-exported symbols

| Symbol                  | Source       | Purpose                                          |
|-------------------------|--------------|--------------------------------------------------|
| `_build_realtime_url`   | `core.py`    | Supabase realtime URL builder                    |
| `build_agent_card`      | `core.py`    | A2A agent discovery card                         |
| `dashboard`             | `core.py`    | HTML dashboard generator                         |
| `execute_simone_action` | `core.py`    | Main action dispatcher                          |
| `find_references`       | `core.py`    | Symbol reference search                          |
| `find_symbol`           | `core.py`    | Symbol definition search                         |
| `get_project_overview`  | `core.py`    | Workspace footprint summary                      |
| `insert_after_symbol`   | `core.py`    | Insert code after a symbol                       |
| `process_lsp_task`      | `core.py`    | Backward-compat shim around `execute_simone_action` |
| `replace_symbol_body`   | `core.py`    | Replace symbol body safely                       |

## Usage

```python
from mcp_server import find_symbol, find_references
```

## Caveats / footguns

- The re-exports are a stable surface — clients pin against this module. Renaming an export is a breaking change.
