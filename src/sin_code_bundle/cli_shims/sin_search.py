# SPDX-License-Identifier: MIT
"""Purpose: `sin-search` CLI shim — wraps `scout` Go tool (regex/semantic/symbol).

Docs: sin_search.doc.md

Wraps `sin_code_bundle.file_ops.sin_search` as a real binary.

Usage:
    sin-search --query "..." [--path DIR] [--type semantic|regex|symbol|usage]

Example:
    $ sin-search --query "def main" --path ./src --type regex
    {"results": [{"file": "...", "line": 1, "match": "def main", ...}], ...}
"""
from __future__ import annotations

import argparse
import sys

from sin_code_bundle.file_ops import sin_search


def main(argv: list[str] | None = None) -> int:
    """Entry point for the `sin-search` console script."""
    parser = argparse.ArgumentParser(
        prog="sin-search",
        description="SIN-Code search — wraps `scout` Go tool, falls back to Python regex.",
    )
    parser.add_argument("--query", required=True, help="Search pattern (regex or semantic).")
    parser.add_argument(
        "--path",
        default=".",
        help="File or directory to search in (default: current dir).",
    )
    # search_type: semantic | regex | symbol | usage — passed straight through
    # to `scout`. The Python-regex fallback ignores this flag.
    parser.add_argument(
        "--type",
        dest="search_type",
        default="semantic",
        choices=["semantic", "regex", "symbol", "usage"],
        help="Search mode (default: semantic).",
    )
    args = parser.parse_args(argv)
    print(sin_search(args.query, args.path, args.search_type))
    return 0


if __name__ == "__main__":
    sys.exit(main())
