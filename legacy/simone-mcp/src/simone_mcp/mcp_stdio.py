from __future__ import annotations

import json
import sys
import uuid
from typing import Any

from .core import TOOL_DEFINITIONS, execute_simone_action, json_dumps


async def _handle_request(
    payload: dict[str, Any], session_id: str | None
) -> tuple[dict[str, Any] | None, str | None]:
    request_id = payload.get("id")
    method = payload.get("method")
    if method == "initialize":
        session_id = session_id or str(uuid.uuid4())
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {
                "protocolVersion": "2025-03-26",
                "capabilities": {"tools": {}, "logging": {}},
                "serverInfo": {"name": "simone-mcp", "version": "2026.04.12"},
                "sessionId": session_id,
            },
        }, session_id
    if method in {"initialized", "notifications/initialized"}:
        return None, session_id
    if method == "ping":
        return {"jsonrpc": "2.0", "id": request_id, "result": {}}, session_id
    if method == "tools/list":
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {"tools": TOOL_DEFINITIONS},
        }, session_id
    if method == "resources/list":
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {"resources": []},
        }, session_id
    if method == "prompts/list":
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {"prompts": []},
        }, session_id
    if method == "tools/call":
        params = payload.get("params", {})
        action = dict(params.get("arguments", {}))
        action["action"] = params.get("name")
        result = await execute_simone_action(action)
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {
                "content": [{"type": "text", "text": json_dumps(result)}],
                "isError": not result.get("ok", False),
            },
        }, session_id
    return {
        "jsonrpc": "2.0",
        "id": request_id,
        "error": {"code": -32601, "message": "Method not found"},
    }, session_id


async def serve_stdio() -> None:
    session_id: str | None = None
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
                response, session_id = await _handle_request(item, session_id)
                if response is not None:
                    responses.append(response)
            if responses:
                sys.stdout.write(json.dumps(responses) + "\n")
                sys.stdout.flush()
            continue
        response, session_id = await _handle_request(payload, session_id)
        if response is None:
            continue
        sys.stdout.write(json.dumps(response) + "\n")
        sys.stdout.flush()
