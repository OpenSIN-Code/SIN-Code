from __future__ import annotations

from pathlib import Path

import pytest

from simone_mcp.core import TOOL_DEFINITIONS, execute_simone_action
from simone_mcp.protocol import handle_mcp_request


LEGACY_GRAPH_TOOLS = {
    "sin_simone_mcp_graphify_query",
    "sin_simone_mcp_graphify_update",
    "sin_simone_mcp_graphify_explain",
    "sin_simone_mcp_graphify_path",
}


def test_legacy_graph_tools_are_not_advertised_by_default() -> None:
    advertised = {tool["name"] for tool in TOOL_DEFINITIONS}
    assert advertised.isdisjoint(LEGACY_GRAPH_TOOLS)


@pytest.mark.asyncio
async def test_direct_legacy_graph_action_routes_to_sin_context() -> None:
    result = await execute_simone_action(
        {
            "action": "sin_simone_mcp_graphify_query",
            "query": "architecture",
            "root": ".",
        }
    )
    assert result == {
        "ok": False,
        "error": "graph_tool_routed_via_sin_context",
        "action": "sin_simone_mcp_graphify_query",
    }


@pytest.mark.asyncio
async def test_protocol_rejects_disabled_graph_tool() -> None:
    response, _, _ = await handle_mcp_request(
        {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "tools/call",
            "params": {
                "name": "sin_simone_mcp_graphify_query",
                "arguments": {"query": "architecture", "root": "."},
            },
        },
        session_id="test-session",
    )

    assert response is not None
    assert response["error"]["code"] == -32602
    assert "disabled tool" in response["error"]["message"]


@pytest.mark.asyncio
async def test_structural_edit_contract_matches_handler(tmp_path: Path) -> None:
    source = tmp_path / "module.py"
    source.write_text(
        "def greeting():\n    return 'old'\n",
        encoding="utf-8",
    )

    response, _, _ = await handle_mcp_request(
        {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/call",
            "params": {
                "name": "sin_simone_mcp_structural_edit",
                "arguments": {
                    "symbol": "greeting",
                    "file": str(source),
                    "body": "return 'new'",
                    "root": str(tmp_path),
                },
            },
        },
        session_id="test-session",
    )

    assert response is not None
    assert response["result"]["isError"] is False
    assert "return 'new'" in source.read_text(encoding="utf-8")
