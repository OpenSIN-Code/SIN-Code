# Purpose: CLI shim for simone insert_after_symbol
# Docs: simone-insert-after-symbol.doc.md
"""CLI: simone-insert-after-symbol — insert text immediately after a symbol.

Mirrors the MCP tool `insert_after_symbol`. Reads the insertion text
from a file (--text-file) for ergonomic reasons.

Usage:
    simone-insert-after-symbol --symbol NAME --file PATH --text-file PATH [--root PATH]
"""
from __future__ import annotations
import argparse
import json
import sys
from pathlib import Path

from simone_mcp.core import insert_after_symbol


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-insert-after-symbol",
        description="Insert text immediately after a symbol's last line.",
    )
    parser.add_argument("--symbol", required=True)
    parser.add_argument("--file", required=True)
    parser.add_argument("--text-file", required=True)
    parser.add_argument("--root", default=None)
    args = parser.parse_args(argv)
    text = Path(args.text_file).read_text(encoding="utf-8")
    payload: dict = {"symbol": args.symbol, "file": args.file, "text": text}
    if args.root:
        payload["root"] = args.root
    result = insert_after_symbol(payload)
    print(json.dumps(result, indent=2, default=str))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
