"""Purpose: `sin-bash` CLI shim — safe shell exec via `execute` Go binary.

Docs: sin_bash.doc.md

Wraps `sin_code_bundle.file_ops.sin_bash` as a real binary.

Usage:
    sin-bash --command "..." [--timeout N]

Example:
    $ sin-bash --command "echo hello"
    {"stdout": "hello\\n", "stderr": "", "returncode": 0, "redacted": true}
"""
from __future__ import annotations

import argparse
import sys

from sin_code_bundle.file_ops import sin_bash


def main(argv: list[str] | None = None) -> int:
    """Entry point for the `sin-bash` console script."""
    parser = argparse.ArgumentParser(
        prog="sin-bash",
        description="SIN-Code bash — safe shell exec with secret redaction + timeout.",
    )
    # --command vs --command-from-file: file variant avoids argv-length issues
    # and is the only safe way to pass multi-line scripts.
    g = parser.add_mutually_exclusive_group(required=True)
    g.add_argument("--command", help="Shell command to execute (single line).")
    g.add_argument(
        "--command-from-file",
        help="Read command from this file (use '-' for stdin).",
    )
    # 60s default = matches MCP tool; 600s max matches the `execute` Go binary's
    # own upper bound. Going past 600s risks orphaned subprocesses.
    parser.add_argument(
        "--timeout",
        type=int,
        default=60,
        help="Timeout in seconds (default: 60, max 600).",
    )
    args = parser.parse_args(argv)

    if args.command is not None:
        cmd = args.command
    else:
        if args.command_from_file == "-":
            cmd = sys.stdin.read()
        else:
            with open(args.command_from_file, encoding="utf-8") as fp:
                cmd = fp.read()

    print(sin_bash(cmd, args.timeout))
    return 0


if __name__ == "__main__":
    sys.exit(main())
