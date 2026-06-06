"""Purpose: `sin-read` CLI shim — URI-aware, size-safe file read.

Docs: cli/sin_read.doc.md

Wraps `sin_code_bundle.file_ops.sin_read` as a real binary so sub-agents
and shell users can call it without spinning up the MCP server.

Usage:
    sin-read <path> [--summarize] [--max-chars N]

Example:
    $ sin-read /etc/hostname
    {"path": "/etc/hostname", "chars": ..., "truncated": false, "content": "..."}

    $ sin-read sckg://module/foo/callers
    {"module": "foo", "callers": [...]}
"""
from __future__ import annotations

import argparse
import sys

from sin_code_bundle.file_ops import sin_read


def main(argv: list[str] | None = None) -> int:
    """Entry point for the `sin-read` console script."""
    parser = argparse.ArgumentParser(
        prog="sin-read",
        description="SIN-Code read — URI-aware, size-safe file read.",
    )
    parser.add_argument("path", help="File path or SIN URI (sckg://, poc://, ...)")
    # 50000 = matches the MCP-tool default; see file_ops.sin_read.
    parser.add_argument(
        "--summarize",
        action="store_true",
        help="Return structural overview (line count, first_5, last_5) instead of content.",
    )
    parser.add_argument(
        "--max-chars",
        type=int,
        default=50000,
        help="Truncate to head+tail if larger than this (default: 50000).",
    )
    args = parser.parse_args(argv)
    # sin_read returns JSON string — print raw to stdout for easy piping into jq.
    print(sin_read(args.path, args.summarize, args.max_chars))
    return 0


if __name__ == "__main__":
    sys.exit(main())
