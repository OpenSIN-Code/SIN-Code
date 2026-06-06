# Purpose: Built-in slash commands and their mappings.
# Docs: commands.doc.md
"""Built-in slash commands and their mappings.

Each built-in command maps to a sin-* tool or shell action. The dispatcher uses
this mapping to route commands to the executor.
"""

from typing import Any

# Built-in command definitions
# Each entry: name -> {"description": str, "action": str, "type": str}
BUILTIN_COMMANDS: dict[str, dict[str, Any]] = {
    "refactor": {
        "description": "Refactor code using symbol resolution and checkpointing",
        "action": "sin_symbol_resolve",
        "type": "sin",
        "help": "Usage: /refactor <symbol> [--scope=<scope>]\nResolves a symbol and creates a checkpoint before refactoring.",
    },
    "test": {
        "description": "Run tests using pytest",
        "action": "pytest",
        "type": "shell",
        "help": "Usage: /test [path] [--verbose] [--coverage]\nRun pytest on the specified path or all tests.",
    },
    "docs": {
        "description": "Check code documentation with CoDocs",
        "action": "sin codocs check",
        "type": "shell",
        "help": "Usage: /docs [--fix] [--path=<path>]\nRun CoDocs documentation validation.",
    },
    "commit": {
        "description": "Create an immortal commit with conventional format",
        "action": "sin_immortal_commit",
        "type": "sin",
        "help": "Usage: /commit <message> [--type=<type>] [--scope=<scope>]\nCreate a conventional commit with an annotated tag.",
    },
    "audit": {
        "description": "Run CEO audit on the repository",
        "action": "sin ceo-audit",
        "type": "shell",
        "help": "Usage: /audit [--profile=<profile>] [--grade=<grade>]\nRun the 47-gate CEO audit.",
    },
    "status": {
        "description": "Show project status and health",
        "action": "sin status",
        "type": "shell",
        "help": "Usage: /status [--detailed]\nShow project status, tool versions, and health.",
    },
    "search": {
        "description": "Search the web using sin websearch",
        "action": "sin_websearch",
        "type": "sin",
        "help": "Usage: /search <query> [--limit=<n>]\nPerform a web search using the sin websearch tool.",
    },
    "help": {
        "description": "Show help for a command",
        "action": "help",
        "type": "python",
        "help": "Usage: /help <command>\nShow help for the specified command.",
    },
    "list": {
        "description": "List all available commands",
        "action": "list",
        "type": "python",
        "help": "Usage: /list [--built-in] [--custom]\nList all available commands.",
    },
    "history": {
        "description": "Show recent command history",
        "action": "history",
        "type": "python",
        "help": "Usage: /history [--limit=<n>]\nShow recent slash command invocations.",
    },
}


def get_builtin_command(name: str) -> dict[str, Any] | None:
    """Get a built-in command by name.

    Args:
        name: Command name to look up.

    Returns:
        Command dict if found, None otherwise.
    """
    return BUILTIN_COMMANDS.get(name)


def list_builtin_commands() -> list[str]:
    """List all built-in command names.

    Returns:
        Sorted list of command names.
    """
    return sorted(BUILTIN_COMMANDS.keys())


def get_command_help(name: str) -> str:
    """Get help text for a built-in command.

    Args:
        name: Command name to get help for.

    Returns:
        Help text string, or "Unknown command" if not found.
    """
    cmd = BUILTIN_COMMANDS.get(name)
    if cmd:
        return cmd.get("help", f"/{name} - {cmd['description']}")
    return f"Unknown command: /{name}"
