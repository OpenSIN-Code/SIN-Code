from __future__ import annotations

import asyncio
import json
import sys
from typing import Any

from .core import build_agent_card, execute_simone_action
from .mcp_stdio import serve_stdio


def _print(payload: Any) -> None:
    sys.stdout.write(f"{json.dumps(payload, indent=2)}\n")


def _read_action_argument() -> dict[str, Any]:
    if len(sys.argv) > 2:
        raw = sys.argv[2]
    else:
        raw = sys.stdin.read().strip()
    if not raw:
        raise SystemExit("missing_action_json")
    return json.loads(raw)


def main() -> None:
    command = sys.argv[1] if len(sys.argv) > 1 else "help"
    if command in {"serve", "serve-a2a"}:
        import uvicorn

        from .http_app import create_app

        uvicorn.run(
            create_app(),
            host="0.0.0.0",
            port=int(sys.argv[2])
            if len(sys.argv) > 2 and sys.argv[2].isdigit()
            else 8234,
        )
        return
    if command == "serve-mcp":
        asyncio.run(serve_stdio())
        return
    if command == "print-card":
        _print(build_agent_card("http://localhost:8234"))
        return
    if command == "run-action":
        _print(asyncio.run(execute_simone_action(_read_action_argument())))
        return
    sys.stderr.write(
        "Usage:\n"
        "  python src/cli.py serve\n"
        "  python src/cli.py serve-mcp\n"
        "  python src/cli.py print-card\n"
        '  python src/cli.py run-action \'{"action":"simone.mcp.health"}\'\n'
    )


if __name__ == "__main__":
    main()
