# `server.py` (simone_mcp) — MCP Server Entry Point

What this file does: the `python -m simone_mcp.server` entry point. Wraps `serve_stdio()` in `asyncio.run`.

## Dependency map

- Imports: `asyncio`, `mcp_stdio.serve_stdio`.
- Imported by: nothing in this repo. Use as a script target.

## Usage

```bash
python -m simone_mcp.server
```

Or via the CLI: `simone serve-mcp`.

## Caveats / footguns

- This is a stdio server. The process owns stdin/stdout — don't write logging to stdout (use stderr).
- For HTTP transport, use `python -m simone_mcp.cli serve` or `uvicorn src.main:app`.
