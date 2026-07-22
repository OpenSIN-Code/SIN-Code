"""Command-line adapter for the durable Simone control plane."""

from __future__ import annotations

import argparse
import json
from typing import Sequence

from .control_plane_tool import OPERATIONS, execute_control_plane


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="simone-control-plane",
        description=(
            "Execute one durable Simone control-plane operation and emit JSON."
        ),
    )
    parser.add_argument("operation", choices=OPERATIONS)
    parser.add_argument(
        "--payload-json",
        required=True,
        help="JSON object passed to the selected operation.",
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = build_parser().parse_args(argv)

    try:
        payload = json.loads(args.payload_json)
    except json.JSONDecodeError as error:
        print(
            json.dumps(
                {
                    "ok": False,
                    "operation": args.operation,
                    "error": f"invalid payload JSON: {error}",
                }
            )
        )
        return 2

    if not isinstance(payload, dict):
        print(
            json.dumps(
                {
                    "ok": False,
                    "operation": args.operation,
                    "error": "payload JSON must be an object",
                }
            )
        )
        return 2

    result = execute_control_plane(
        {
            "operation": args.operation,
            "payload": payload,
        }
    )
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
