#!/usr/bin/env python3

import argparse
import asyncio
import json
import sys
from pathlib import Path

from mcp_server import build_agent_card, execute_simone_action, run_server


def main() -> int:
    parser = argparse.ArgumentParser(prog="activate_simone")
    sub = parser.add_subparsers(dest="command")

    sub.add_parser("activate")
    sub.add_parser("serve-mcp")
    sub.add_parser("serve-a2a")
    sub.add_parser("print-card")

    run_action = sub.add_parser("run-action")
    run_action.add_argument("payload", nargs="?", default="")

    args = parser.parse_args()

    if args.command in {None, "activate"}:
        print("Activating Simone MCP...")
        try:
            import subprocess

            subprocess.Popen([sys.executable, str(Path(__file__).with_name("mcp_server.py"))])
        except Exception as exc:
            print(f"Could not auto-start server: {exc}")
        print("http://localhost:8234/dashboard")
        return 0

    if args.command in {"serve-mcp", "serve-a2a"}:
        run_server()
        return 0

    if args.command == "print-card":
        print(json.dumps(build_agent_card("http://localhost:8234"), indent=2))
        return 0

    if args.command == "run-action":
        payload = args.payload.strip() or sys.stdin.read().strip()
        if not payload:
            raise SystemExit("missing_action_json")
        action = json.loads(payload)
        result = asyncio.run(execute_simone_action(action))
        print(json.dumps(result, indent=2))
        return 0

    parser.print_help()
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
