from __future__ import annotations

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
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": build_agent_card(base_url),
            }

        if method == "agent.ping":
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {"status": "alive", "timestamp": datetime.now().isoformat()},
            }

        if method == "tool.list":
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {"tools": TOOL_DEFINITIONS},
            }

        if method == "tool.call":
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
                return {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "result": {"correlation_id": correlation_id, "data": result},
                }
            except Exception as e:
                correlation_manager.complete_call(correlation_id, None, str(e))
                return _error_response(request_id, -32603, str(e))

        return _error_response(request_id, -32601, f"Unknown method: {method}")

    except Exception as e:
        return _error_response(request_id, -32603, str(e))


def _error_response(request_id: Any, code: int, message: str) -> dict[str, Any]:
    return {
        "jsonrpc": "2.0",
        "id": request_id,
        "error": {"code": code, "message": message},
    }
