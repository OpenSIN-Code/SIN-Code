# SPDX-License-Identifier: MIT
"""Entry point for `python -m sin_delegate serve` — runs the MCP server
in standalone mode (without the full sin-serve bundle).

Useful for development and isolated testing of the delegate tools.
"""

import asyncio
import sys


async def main() -> None:
    try:
        from mcp.server import Server
        from mcp.server.stdio import stdio_server
    except ImportError:
        print("mcp package not installed; run: "
              "pip install 'sin-code-delegate[mcp]'", file=sys.stderr)
        return

    server = Server("sin-delegate-mcp")
    tools: list = []
    handlers: dict = {}

    def add_tool(name, description, schema, handler):
        tools.append({"name": name, "description": description,
                      "inputSchema": schema})
        handlers[name] = handler

    from .mcp_tools import register
    register(add_tool)

    from mcp import types

    @server.list_tools()
    async def list_tools() -> list:
        return [types.Tool(**t) for t in tools]

    @server.call_tool()
    async def call_tool(name: str, arguments: dict | None
                        ) -> list:
        handler = handlers.get(name)
        if handler is None:
            payload = {"error": f"unknown tool {name!r}"}
        else:
            try:
                payload = await handler(arguments or {})
            except Exception as e:
                payload = {"error": f"{type(e).__name__}: {e}"}
        return [types.TextContent(type="text",
                                  text=__import__("json").dumps(
                                      payload, default=str))]

    async with stdio_server() as (read, write):
        await server.run(read, write,
                         server.create_initialization_options())


if __name__ == "__main__":
    asyncio.run(main())
