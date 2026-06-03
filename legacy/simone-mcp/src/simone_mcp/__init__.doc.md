# `simone_mcp/__init__.py` — Simone MCP Package API

What this file does: re-exports the public API (`main`, the tool functions) and silences the "no handlers" warning.

## Dependency map

- Imports: `.cli` (main), `.core` (10 functions).
- Imported by: external user code, the `src/cli.py` and `src/mcp_server.py` shims.

## Public API

```python
from simone_mcp import (
    main,                      # CLI entry point
    _build_realtime_url,       # utility
    build_agent_card,          # A2A discovery
    dashboard,                 # HTML dashboard
    execute_simone_action,     # main dispatcher
    find_references,
    find_symbol,
    get_project_overview,
    insert_after_symbol,
    replace_symbol_body,
)
```

## Caveats / footguns

- `logging.getLogger(__name__).addHandler(NullHandler())` keeps imports quiet for consumers that haven't configured logging. Don't remove it without coordinating with downstream packages.
- `process_lsp_task` was removed from the public re-exports (the underlying function still exists in `core.py` for backward compat). New code should use `execute_simone_action`.
