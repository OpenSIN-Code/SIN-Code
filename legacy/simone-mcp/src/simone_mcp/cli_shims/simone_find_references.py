# Purpose: CLI shim for simone find_references
# Docs: simone-find-references.doc.md
"""CLI: simone-find-references — find textual references to a symbol.

Mirrors the MCP tool `find_references`. Uses jedi if available, else
falls back to a regex search.

Usage:
    simone-find-references <SYMBOL> [--root PATH]
"""
from __future__ import annotations
import argparse
import json
import sys

from simone_mcp.core import find_references


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-find-references",
        description="Find textual references to a symbol (jedi / regex).",
    )
    parser.add_argument("symbol")
    parser.add_argument("--root", default=None)
    args = parser.parse_args(argv)
    payload = {"symbol": args.symbol}
    if args.root:
        payload["root"] = args.root
    result = find_references(payload)
    print(json.dumps(result, indent=2, default=str))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
