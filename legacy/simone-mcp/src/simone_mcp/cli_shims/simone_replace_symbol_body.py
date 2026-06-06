# Purpose: CLI shim for simone replace_symbol_body
# Docs: simone-replace-symbol-body.doc.md
"""CLI: simone-replace-symbol-body — replace a function/method's body.

Mirrors the MCP tool `replace_symbol_body`. Uses libcst if available,
else falls back to a line-level AST splice. Reads the new body from
--body-file to keep quoting easy.

Usage:
    simone-replace-symbol-body --symbol NAME --file PATH [--root PATH] --body-file PATH
"""
from __future__ import annotations
import argparse
import json
import sys
from pathlib import Path

from simone_mcp.core import replace_symbol_body


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-replace-symbol-body",
        description="Replace a function/method's body with text from a file.",
    )
    parser.add_argument("--symbol", required=True)
    parser.add_argument("--file", required=True)
    parser.add_argument("--body-file", required=True)
    parser.add_argument("--root", default=None)
    args = parser.parse_args(argv)
    body = Path(args.body_file).read_text(encoding="utf-8")
    payload: dict = {"symbol": args.symbol, "file": args.file, "body": body}
    if args.root:
        payload["root"] = args.root
    result = replace_symbol_body(payload)
    print(json.dumps(result, indent=2, default=str))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
