"""MCP stdio transport — line-delimited JSON-RPC over stdin/stdout.

The canonical transport for local MCP clients (OpenCode, Claude Desktop,
etc.). Each line on stdin is one JSON-RPC request (or a JSON array
batch); each response is one line on stdout.

Docs: mcp_stdio.doc.md
"""
from __future__ import annotations

import json
import logging
import sys
from typing import Any

from .protocol import handle_mcp_request

logger = logging.getLogger(__name__)


# ── Helpers ────────────────────────────────────────────────────────────
async def _send_stdio_notification(notification: dict[str, Any]) -> None:
    """Write a JSON-RPC notification line to stdout and flush.

    Notifications have no `id`; the protocol layer filters them out
    before they would be a request response.
    """
    sys.stdout.write(json.dumps(notification) + "\n")
    sys.stdout.flush()


# ── Public entry point ─────────────────────────────────────────────────
async def serve_stdio() -> None:
    """Run the MCP server reading JSON-RPC requests from stdin.

    Maintains a per-process `session_id` and `client_protocol_version`
    across calls (set on `initialize`, reused on subsequent calls).
    Supports both single objects and JSON arrays (batches).

    Blocks until stdin closes (EOF).
    """
    session_id: str | None = None
    client_protocol_version: str | None = None
    for raw_line in sys.stdin:
        line = raw_line.strip()
        if not line:
            # Skip blank lines (defensive — they shouldn't appear in
            # well-formed JSON-Lines input).
            continue
        try:
            payload = json.loads(line)
        except json.JSONDecodeError as error:
            # Return a parse error (JSON-RPC code -32700) on the same
            # line — the client knows the line was malformed.
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
            # JSON-RPC batch: process each item, then reply with an array.
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
            # Remember the client's protocol version for the whole
            # session — the protocol layer uses it for version negotiation.
            client_protocol_version = payload.get("params", {}).get("protocolVersion") if isinstance(payload.get("params"), dict) else None
        response, session_id, notifications = await handle_mcp_request(
            payload, session_id, send_notification=_send_stdio_notification, client_protocol_version=client_protocol_version
        )
        for n in notifications:
            sys.stdout.write(json.dumps(n) + "\n")
            sys.stdout.flush()
        if response is None:
            # Pure notification — no response needed.
            continue
        sys.stdout.write(json.dumps(response) + "\n")
        sys.stdout.flush()
