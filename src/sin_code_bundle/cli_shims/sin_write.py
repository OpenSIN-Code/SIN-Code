"""Purpose: `sin-write` CLI shim — atomic write with backup + syntax verify.

Docs: cli/sin_write.doc.md

Wraps `sin_code_bundle.file_ops.sin_write` as a real binary.

Usage:
    sin-write <path> --content "..." [--no-verify]

Example:
    $ sin-write /tmp/hello.py --content 'print("hi")'
    {"success": true, "path": "/tmp/hello.py", "chars": 12, "verified": true, "backup": null}
"""
from __future__ import annotations

import argparse
import sys

from sin_code_bundle.file_ops import sin_write


def main(argv: list[str] | None = None) -> int:
    """Entry point for the `sin-write` console script."""
    parser = argparse.ArgumentParser(
        prog="sin-write",
        description="SIN-Code write — atomic write with auto-backup and Python syntax verify.",
    )
    parser.add_argument("path", help="Target file path")
    # --content vs --content-from-file: shell-passed content is fine for small
    # strings; the file variant handles multi-MB / binary-ish payloads without
    # blowing up argv limits (~256KB on most kernels).
    g = parser.add_mutually_exclusive_group(required=True)
    g.add_argument("--content", help="Content to write (string, max ~256KB).")
    g.add_argument(
        "--content-from-file",
        help="Read content from this file (use '-' for stdin).",
    )
    # --no-verify flag = verify=False. verify=True runs `compile()` on .py files
    # to catch SyntaxError before disk-write. We default to True (the safe path).
    parser.add_argument(
        "--no-verify",
        action="store_true",
        help="Skip Python syntax pre-validation (verify=False).",
    )
    args = parser.parse_args(argv)

    # Read content from --content or --content-from-file
    if args.content is not None:
        content = args.content
    else:
        if args.content_from_file == "-":
            content = sys.stdin.read()
        else:
            with open(args.content_from_file, encoding="utf-8") as fp:
                content = fp.read()

    # verify defaults to True; only flip it off when --no-verify is passed.
    print(sin_write(args.path, content, verify=not args.no_verify))
    return 0


if __name__ == "__main__":
    sys.exit(main())
