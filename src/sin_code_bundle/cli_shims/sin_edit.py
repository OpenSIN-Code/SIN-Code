# SPDX-License-Identifier: MIT
"""Purpose: `sin-edit` CLI shim — hashline-anchored semantic patching.

Docs: sin_edit.doc.md

Wraps `sin_code_bundle.file_ops.sin_edit` as a real binary.

Usage:
    sin-edit <file> --old "..." --new "..." [--intent "..."]

Example:
    $ sin-edit /tmp/hello.py --old 'print("hi")' --new 'print("hello")'
    {"success": true, "message": "...", "intent": "", "patch": {...}}
"""
from __future__ import annotations

import argparse
import sys

from sin_code_bundle.file_ops import sin_edit


def main(argv: list[str] | None = None) -> int:
    """Entry point for the `sin-edit` console script."""
    parser = argparse.ArgumentParser(
        prog="sin-edit",
        description="SIN-Code edit — hashline-anchored, line-shift-resilient patch.",
    )
    parser.add_argument("file_path", help="Target file path")
    parser.add_argument(
        "--old",
        dest="old_content",
        required=True,
        help="The exact text to replace (must be unique after hashline normalize).",
    )
    parser.add_argument(
        "--new",
        dest="new_content",
        default="",
        help="The replacement text (default: empty string = delete).",
    )
    parser.add_argument(
        "--intent",
        default="",
        help="Free-text description of why this edit exists (logged in patch metadata).",
    )
    args = parser.parse_args(argv)
    print(
        sin_edit(
            args.file_path,
            args.old_content,
            args.new_content,
            args.intent,
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
