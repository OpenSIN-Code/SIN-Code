from __future__ import annotations

import json
import logging
import sys
from typing import Any

from .protocol import handle_mcp_request

logger = logging.getLogger(__name__)


async def _send_stdio_notification(notification: dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(notification) + "\n")
    sys.stdout.flush()


async def serve_stdio() -> None:
    session_id: str | None = None
    client_protocol_version: str | None = None
    for raw_line in sys.stdin:
        line = raw_line.strip()
        if not line:
            continue
        try:
            payload = json.loads(line)
        except json.JSONDecodeError as error:
            sys.stdout.write(
                json.dumps(
                    {
                        "jsonrpc": "2.0",
                        "id": None,
                        "error": {"code": -32700, "message": str(error)},
                    }
                )
                + "\n"
            )
            sys.stdout.flush()
            continue
        if isinstance(payload, list):
            responses = []
            for item in payload:
                if isinstance(item, dict) and item.get("method") == "initialize":
                    client_protocol_version = item.get("params", {}).get("protocolVersion") if isinstance(item.get("params"), dict) else None
                response, session_id, notifications = await handle_mcp_request(
                    item, session_id, send_notification=_send_stdio_notification, client_protocol_version=client_protocol_version
                )
                for n in notifications:
                    sys.stdout.write(json.dumps(n) + "\n")
                    sys.stdout.flush()
                if response is not None:
                    responses.append(response)
            if responses:
                sys.stdout.write(json.dumps(responses) + "\n")
                sys.stdout.flush()
            continue
        if isinstance(payload, dict) and payload.get("method") == "initialize":
            client_protocol_version = payload.get("params", {}).get("protocolVersion") if isinstance(payload.get("params"), dict) else None
        response, session_id, notifications = await handle_mcp_request(
            payload, session_id, send_notification=_send_stdio_notification, client_protocol_version=client_protocol_version
        )
        for n in notifications:
            sys.stdout.write(json.dumps(n) + "\n")
            sys.stdout.flush()
        if response is None:
            continue
        sys.stdout.write(json.dumps(response) + "\n")
        sys.stdout.flush()
