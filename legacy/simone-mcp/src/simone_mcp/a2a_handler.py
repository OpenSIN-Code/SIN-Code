from __future__ import annotations

import json
import uuid
from datetime import datetime
from typing import Any

from .core import TOOL_DEFINITIONS, build_agent_card, execute_simone_action
from .correlation import correlation_manager


async def handle_a2a_request(
    payload: dict[str, Any], base_url: str
) -> dict[str, Any]:
    request_id = payload.get("id")
    method = payload.get("method")
    params = payload.get("params") or {}

    try:
        if method == "agent.discover":
            return _ok_response(request_id, build_agent_card(base_url))

        if method == "agent.ping":
            return _ok_response(
                request_id, {"status": "alive", "timestamp": datetime.now().isoformat()}
            )

        if method == "tool.list":
            return _ok_response(request_id, {"tools": TOOL_DEFINITIONS})

        if method == "tool.call":
            return await _handle_tool_call(request_id, params)

        if method == "message/send":
            return await _handle_message_send(request_id, params)

        if method == "tasks/get":
            task_id = params.get("id")
            if not task_id:
                return _error_response(request_id, -32602, "Missing task id")
            return _ok_response(
                request_id,
                {
                    "id": task_id,
                    "kind": "task",
                    "status": {"state": "completed"},
                },
            )

        return _error_response(request_id, -32601, f"Method not found: {method}")

    except Exception as e:
        return _error_response(request_id, -32603, str(e))


async def _handle_tool_call(request_id: Any, params: dict[str, Any]) -> dict[str, Any]:
    tool_name = params.get("tool_name") or params.get("name")
    arguments = params.get("arguments") or params.get("params") or {}
    if not tool_name:
        return _error_response(request_id, -32602, "Missing tool_name in params")
    action = dict(arguments)
    action["action"] = tool_name
    correlation_id = correlation_manager.generate_correlation_id(
        tool_name, arguments, params.get("correlation_id")
    )
    try:
        result = await execute_simone_action(action)
        correlation_manager.complete_call(correlation_id, result)
        return _ok_response(
            request_id, {"correlation_id": correlation_id, "data": result}
        )
    except Exception as e:
        correlation_manager.complete_call(correlation_id, None, str(e))
        return _error_response(request_id, -32603, str(e))


async def _handle_message_send(request_id: Any, params: dict[str, Any]) -> dict[str, Any]:
    parts = params.get("message", {}).get("parts", [])
    text = " ".join(str(part.get("text", "")) for part in parts).strip()
    action = {"action": text or "agent.help"}
    try:
        parsed = json.loads(text) if text else None
        if isinstance(parsed, dict) and isinstance(parsed.get("action"), str):
            action = parsed
    except json.JSONDecodeError:
        pass
    result = await execute_simone_action(action)
    return _ok_response(
        request_id,
        {
            "id": str(uuid.uuid4()),
            "kind": "task",
            "status": {
                "state": "completed",
                "message": {
                    "role": "agent",
                    "parts": [{"type": "text", "text": "completed"}],
                },
            },
            "artifacts": [
                {
                    "id": str(uuid.uuid4()),
                    "name": action.get("action", "agent.help"),
                    "parts": [{"type": "data", "data": result}],
                }
            ],
        },
    )


def _ok_response(request_id: Any, result: Any) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def _error_response(request_id: Any, code: int, message: str) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}}
