# Purpose: CLI shim for simone process_lsp_task
# Docs: simone-process-lsp-task.doc.md
"""CLI: simone-process-lsp-task — backward-compat shim, dispatches to execute_simone_action.

Mirrors the async MCP tool `process_lsp_task`. Kept for callers that
still use the older "LSP task" framing.

Usage:
    simone-process-lsp-task --action NAME [--payload JSON] [--payload-file PATH]
"""
from __future__ import annotations
import argparse
import asyncio
import json
import sys
from pathlib import Path

from simone_mcp.core import process_lsp_task


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="simone-process-lsp-task",
        description="Process an LSP-style task (dispatches to execute_simone_action).",
    )
    parser.add_argument("--action", default=None)
    parser.add_argument("--payload", default=None)
    parser.add_argument("--payload-file", default=None)
    args = parser.parse_args(argv)
    payload: dict = {}
    if args.payload_file:
        payload = json.loads(Path(args.payload_file).read_text(encoding="utf-8"))
    elif args.payload:
        payload = json.loads(args.payload)
    if args.action:
        payload["action"] = args.action
    result = asyncio.run(process_lsp_task(payload))
    print(json.dumps(result, indent=2, default=str))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
