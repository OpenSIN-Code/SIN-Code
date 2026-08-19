"""Top-level CLI entry point for the `simone` command.

Thin wrapper that delegates to `simone_mcp.cli.main()`. The actual
implementation lives in `simone_mcp/cli.py`; this file is the script
target for `python -m src.cli` and the installed `simone` console
script (declared in `pyproject.toml`).

Docs: cli.doc.md
"""
from simone_mcp.cli import main


if __name__ == "__main__":
    main()
