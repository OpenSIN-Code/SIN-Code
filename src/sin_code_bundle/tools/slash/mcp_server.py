# SPDX-License-Identifier: MIT
# Purpose: FastMCP server exposing slash command tools.
# Docs: mcp_server.doc.md
"""FastMCP server exposing slash command tools.

Provides MCP tools for dispatch, listing, registration, and management of slash commands.
"""

import json
from typing import Any, Optional

from fastmcp import FastMCP

from sin_code_bundle.tools.slash.dispatcher import CommandDispatcher, DispatchResult
from sin_code_bundle.tools.slash.registry import CommandRegistry
from sin_code_bundle.tools.slash.commands import BUILTIN_COMMANDS, get_command_help

# Initialize MCP server
mcp = FastMCP("sin-slash")

# Global dispatcher instance (shared across tools)
_dispatcher: Optional[CommandDispatcher] = None


def _get_dispatcher() -> CommandDispatcher:
    """Get or create the shared dispatcher instance.

    Returns:
        CommandDispatcher instance.
    """
    global _dispatcher
    if _dispatcher is None:
        _dispatcher = CommandDispatcher()
    return _dispatcher


@mcp.tool()
def slash_dispatch(command: str) -> str:
    """Dispatch a slash command.

    Args:
        command: The full slash command string (e.g., "/test --verbose").

    Returns:
        JSON string with the dispatch result.
    """
    dispatcher = _get_dispatcher()
    result = dispatcher.dispatch(command)
    return json.dumps(
        {
            "success": result.success,
            "command": result.command,
            "output": result.output,
            "error": result.error,
            "duration_ms": result.duration_ms,
            "timestamp": result.timestamp,
        },
        indent=2,
    )


@mcp.tool()
def slash_list(built_in: bool = True, custom: bool = True) -> str:
    """List available slash commands.

    Args:
        built_in: Include built-in commands. Defaults to True.
        custom: Include custom commands. Defaults to True.

    Returns:
        JSON string with available commands.
    """
    dispatcher = _get_dispatcher()
    commands = dispatcher.list_commands()
    result: dict[str, Any] = {}

    if built_in:
        result["built_in"] = commands.get("built_in", {})
    if custom:
        result["custom"] = commands.get("custom", {})

    return json.dumps(result, indent=2)


@mcp.tool()
def slash_register(
    name: str,
    description: str,
    action: str,
    action_type: str = "shell",
) -> str:
    """Register a new custom slash command.

    Args:
        name: Command name (without leading slash).
        description: Human-readable description.
        action: The action to execute.
        action_type: One of "shell", "sin", or "script". Defaults to "shell".

    Returns:
        JSON string with the registered command.
    """
    dispatcher = _get_dispatcher()
    registry = dispatcher._registry
    try:
        cmd = registry.register(name, description, action, action_type)
        return json.dumps(
            {
                "success": True,
                "command": {
                    "name": cmd.name,
                    "description": cmd.description,
                    "action": cmd.action,
                    "action_type": cmd.action_type,
                    "created_at": cmd.created_at,
                    "updated_at": cmd.updated_at,
                },
            },
            indent=2,
        )
    except ValueError as e:
        return json.dumps({"success": False, "error": str(e)}, indent=2)


@mcp.tool()
def slash_unregister(name: str) -> str:
    """Remove a custom slash command.

    Args:
        name: Command name to remove.

    Returns:
        JSON string with the result.
    """
    dispatcher = _get_dispatcher()
    registry = dispatcher._registry
    removed = registry.unregister(name)
    return json.dumps(
        {"success": removed, "message": f"Command /{name} removed" if removed else f"Command /{name} not found"},
        indent=2,
    )


@mcp.tool()
def slash_help(command: str) -> str:
    """Show help for a specific command.

    Args:
        command: Command name to get help for.

    Returns:
        Help text for the command.
    """
    dispatcher = _get_dispatcher()
    help_text = dispatcher.get_command_help(command)
    if help_text:
        return help_text
    return f"Unknown command: /{command}"


@mcp.tool()
def slash_history(limit: int = 50) -> str:
    """Show recent slash command invocations.

    Args:
        limit: Maximum number of records to return. Defaults to 50.

    Returns:
        JSON string with recent history.
    """
    dispatcher = _get_dispatcher()
    history = dispatcher.get_history(limit=limit)
    return json.dumps(history, indent=2)


def main() -> None:
    """Run the MCP server."""
    mcp.run()


if __name__ == "__main__":
    main()
