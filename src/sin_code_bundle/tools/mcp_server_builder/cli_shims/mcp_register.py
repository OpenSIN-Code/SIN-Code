# SPDX-License-Identifier: MIT
# Purpose: CLI shim for mcp_register
# Docs: mcp-register.doc.md
"""CLI: mcp-register — register a new MCP server in opencode.json.

Usage: mcp-register --name NAME --command CMD [--enabled] [--env JSON] [--config-path PATH]
"""

from __future__ import annotations

import argparse
import sys

from ..mcp_server import mcp_register


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="mcp-register", description="Register a new MCP server in opencode.json."
    )
    parser.add_argument("--name", required=True, help="Server name (mcp section key).")
    parser.add_argument(
        "--command", required=True, help='Space-separated cmd + args, e.g. "uvx my-tool-mcp".'
    )
    parser.add_argument("--enabled", type=bool, default=True)
    parser.add_argument("--env", default="{}", help="JSON object of env vars to set.")
    parser.add_argument(
        "--config-path", default="", help="Optional path to opencode.json (default: auto-discover)."
    )
    args = parser.parse_args(argv)
    try:
        print(
            mcp_register(
                name=args.name,
                command=args.command,
                enabled=args.enabled,
                env=args.env,
                config_path=args.config_path,
            )
        )
    except Exception as e:
        print(f"[mcp-register] error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
