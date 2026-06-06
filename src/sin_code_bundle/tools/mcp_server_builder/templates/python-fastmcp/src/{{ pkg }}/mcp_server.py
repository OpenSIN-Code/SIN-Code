"""
Purpose: FastMCP server exposing {{ tools | length }} tools.
Docs: {{ pkg }}/mcp_server.doc.md
"""
from __future__ import annotations

import json
from typing import Any

from fastmcp import FastMCP

mcp = FastMCP("sin_{{ slug | replace('-', '_') }}")


def _json(payload: Any) -> str:
    """Serialize `payload` as JSON for the MCP wire protocol."""
    return json.dumps(payload, indent=2, default=str)


{% for tool in tools %}
@mcp.tool()
def {{ tool }}() -> str:
    """
    {{ tool }} — generated tool.

    Returns:
        JSON string with the result.
    """
    result = {"tool": "{{ tool }}", "ok": True}
    return _json(result)
{% endfor %}


def main() -> None:
    """Entry point for the MCP server."""
    mcp.run()


if __name__ == "__main__":
    main()
