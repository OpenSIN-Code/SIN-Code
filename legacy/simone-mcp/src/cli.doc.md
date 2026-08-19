# `src/cli.py` — Top-level CLI Entry Point

What this file does: thin wrapper that delegates to `simone_mcp.cli.main()`. This is the script target for `python -m src.cli` and the installed `simone` console script (declared in `pyproject.toml`).

## Dependency map

- Imports: `simone_mcp.cli.main`.
- Imported by: nothing in this repo. Entry point only.

## Usage

```bash
python src/cli.py serve           # HTTP/A2A server
python src/cli.py serve-mcp       # stdio MCP server
python src/cli.py print-card      # print the agent card

# Or via the installed entry point:
simone serve
```

## Caveats / footguns

- Don't add logic here. Anything substantive belongs in `simone_mcp/cli.py`.
