# Purpose: SIN-Code Slash Skill - merged from sin-slash v0.1.0
# Docs: sin_code_bundle/cli/slash.doc.md
"""SIN-Code Slash Skill - MCP server for slash command dispatch.

Provides built-in slash commands (/refactor, /test, /docs, etc.) and a custom
command registry backed by SQLite. Dispatches commands to the appropriate
sin-* tools or shell execution.

Originally a standalone package (sin-slash), merged into sin-code-bundle v0.9.3
as `sin slash <subcommand>` (issue #29). The legacy Click CLI in
``sin_code_bundle.cli.slash.cli`` is still importable for backwards compatibility.
"""

__version__ = "0.1.0"
__all__ = [
    "parser",
    "registry",
    "dispatcher",
    "executor",
    "commands",
    "mcp_server",
    "cli",
    "app",
]

from .commands import BUILTIN_COMMANDS, get_command_help
from .dispatcher import CommandDispatcher, DispatchResult
from .executor import CommandExecutor
from .parser import ParsedCommand, SlashParser
from .registry import CommandRegistry, CustomCommand

# `app` is the Typer subcommand for `sin slash ...`
from .app import app  # noqa: E402
