# `src/cli.py` — CLI Entry Point

Partner file: `src/cli.py`

## Purpose
Thin entry-point script that delegates to `simone_mcp.cli.main()`. This is the script executed when running `python -m src.cli` or calling the `simone` CLI command.

## Key Symbols
- `main()` — imported from `simone_mcp.cli`

## Relationship
- `src/simone_mcp/cli.py` — contains the actual CLI implementation

## Dependencies
- `simone_mcp.cli.main`

## Usage
```bash
python src/cli.py serve
# or via installed entry point:
simone serve
```
