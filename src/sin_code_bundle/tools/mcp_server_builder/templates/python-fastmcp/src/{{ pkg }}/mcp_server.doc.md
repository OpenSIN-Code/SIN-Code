# `{{ pkg }}/mcp_server.py`

## What it does
FastMCP server exposing the tools defined in this package to MCP clients
(e.g. OpenCode). Each tool is a `@mcp.tool()`-decorated function that
returns a JSON string.

## Dependencies
- `fastmcp` for the MCP framework

## Tools exposed
{% for tool in tools -%}
- `{{ tool }}`
{% endfor %}

## Usage
```bash
python -m {{ pkg }}.mcp_server
# or via the entry point: sin-{{ slug }}-mcp
```

## Notes
- All tools return JSON-serialized strings for MCP wire compatibility.
- Add new tools with the `sin-mcp-server-builder`'s `mcp_tool_add` tool to
  preserve CoDocs and the test conventions.
