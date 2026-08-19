# Purpose: CLI shim for simone find_symbol
# Docs: simone-symbol-search.doc.md
"""CLI: simone-symbol-search — find definitions of a symbol across the workspace.

Mirrors the MCP tool `find_symbol`. Uses AST parsing (jedi/tree-sitter
where available) without spinning up the FastAPI server.

Usage:
    simone-symbol-search <SYMBOL> [--root PATH]

Example:
    simone-symbol-search get_user_by_id --root /Users/jeremy/dev/api
"""
from __future__ import annotations
import argparse
import json
import sys

from simone_mcp.core import find_symbol


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-symbol-search",
        description="Find definitions of a symbol across the workspace (LSP-style).",
    )
    parser.add_argument("symbol", help="Symbol name to search for (function, class, var).")
    parser.add_argument(
        "--root",
        default=None,
        help="Workspace root to search. Defaults to current working directory.",
    )
    args = parser.parse_args(argv)
    payload = {"symbol": args.symbol}
    if args.root:
        payload["root"] = args.root
    result = find_symbol(payload)
    print(json.dumps(result, indent=2, default=str))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
