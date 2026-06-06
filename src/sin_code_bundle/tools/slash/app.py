# Purpose: Typer subcommand exposing `sin slash <subcommand>` (issue #29)
# Docs: slash.doc.md
"""Typer subcommand for the consolidated sin-slash skill.

Exposes the same surface as the legacy Click CLI as ``sin slash ...``::

    sin slash run <command>     # dispatch a slash command
    sin slash list              # list available commands
    sin slash register <name>   # register a custom command
    sin slash unregister <name> # remove a custom command
    sin slash history           # show command history
    sin slash help <command>    # show help for a command
"""

from __future__ import annotations

import json
import sys
from typing import Optional

import typer

from .dispatcher import CommandDispatcher
from .registry import CommandRegistry

# `app` becomes `sin slash <sub>` after `app.add_typer(slash_app, name="slash")`.
app = typer.Typer(
    name="slash",
    help="Manage and dispatch slash commands (built-in + custom).",
    no_args_is_help=True,
)


def _dispatcher() -> CommandDispatcher:
    """Construct a fresh dispatcher (no shared state for CLI invocations)."""
    return CommandDispatcher()


def _registry() -> CommandRegistry:
    """Construct a fresh registry."""
    return CommandRegistry()


@app.command("run")
def slash_run(
    command: str = typer.Argument(..., help="Slash command to dispatch, e.g. /test"),
    raw: bool = typer.Option(False, "--raw", help="Output raw JSON."),
) -> None:
    """Dispatch a slash command end-to-end."""
    dispatcher = _dispatcher()
    result = dispatcher.dispatch(command)
    if raw:
        typer.echo(json.dumps(result.__dict__, indent=2, default=str))
        return
    if result.success:
        typer.echo(f"[OK] /{result.command} ({result.duration_ms:.0f}ms)")
        if result.output:
            typer.echo(result.output)
    else:
        typer.echo(f"[FAIL] /{result.command}", err=True)
        if result.error:
            typer.echo(result.error, err=True)
        raise typer.Exit(code=1)


@app.command("list")
def slash_list(
    built_in: bool = typer.Option(True, "--built-in/--no-built-in", help="Show built-in commands."),
    custom: bool = typer.Option(True, "--custom/--no-custom", help="Show custom commands."),
) -> None:
    """List available slash commands (built-in + custom)."""
    dispatcher = _dispatcher()
    commands = dispatcher.list_commands()
    typer.echo("Built-in:")
    if built_in:
        for name, info in commands.get("built_in", {}).items():
            typer.echo(f"  /{name:10s} {info['description']}")
    typer.echo("Custom:")
    if custom:
        for name, info in commands.get("custom", {}).items():
            typer.echo(f"  /{name:10s} {info['description']}")


@app.command("register")
def slash_register(
    name: str = typer.Argument(..., help="Command name (no leading slash)."),
    description: str = typer.Argument(..., help="Human-readable description."),
    action: str = typer.Argument(..., help="Action to execute."),
    action_type: str = typer.Option(
        "shell", "--type", help="Action type: shell | sin | script."
    ),
) -> None:
    """Register a new custom slash command."""
    registry = _registry()
    try:
        cmd = registry.register(name, description, action, action_type)
    except ValueError as exc:
        typer.echo(f"[FAIL] {exc}", err=True)
        raise typer.Exit(code=1)
    typer.echo(f"[OK] Registered /{cmd.name}")


@app.command("unregister")
def slash_unregister(
    name: str = typer.Argument(..., help="Command name to remove."),
) -> None:
    """Remove a custom slash command."""
    registry = _registry()
    removed = registry.unregister(name)
    if removed:
        typer.echo(f"[OK] Removed /{name}")
    else:
        typer.echo(f"[FAIL] Command /{name} not found", err=True)
        raise typer.Exit(code=1)


@app.command("history")
def slash_history(
    limit: int = typer.Option(20, "--limit", "-n", help="Number of records."),
) -> None:
    """Show recent slash command history."""
    dispatcher = _dispatcher()
    records = dispatcher.get_history(limit=limit)
    if not records:
        typer.echo("(no history)")
        return
    for record in records:
        ok = "OK" if record["success"] else "FAIL"
        typer.echo(
            f"  {record['timestamp']}  /{record['command']:<20s} {ok}  "
            f"{record['duration_ms']:.0f}ms"
        )


@app.command("help")
def slash_help(
    command: str = typer.Argument(..., help="Command name to show help for."),
) -> None:
    """Show help text for a slash command."""
    dispatcher = _dispatcher()
    help_text = dispatcher.get_command_help(command)
    if help_text:
        typer.echo(help_text)
    else:
        typer.echo(f"Unknown command: /{command}", err=True)
        raise typer.Exit(code=1)
