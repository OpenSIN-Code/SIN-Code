# Purpose: CLI entry point for sin-slash.
# Docs: cli.doc.md
"""CLI entry point for sin-slash.

Provides a command-line interface for running slash commands directly.
"""

import json
import sys
from typing import Optional

import click
from rich.console import Console
from rich.table import Table
from rich.panel import Panel

from sin_code_bundle.tools.slash.dispatcher import CommandDispatcher
from sin_code_bundle.tools.slash.registry import CommandRegistry

console = Console()


@click.group()
@click.version_option(version="0.1.0", prog_name="sin-slash")
def cli() -> None:
    """SIN-Code Slash Skill - Slash command dispatch."""
    pass


@cli.command()
@click.argument("command", required=True)
@click.option("--raw", is_flag=True, help="Output raw JSON")
def run(command: str, raw: bool) -> None:
    """Run a slash command.

    Args:
        command: The slash command to run (e.g., "/test").
        raw: Output raw JSON instead of formatted.
    """
    dispatcher = CommandDispatcher()
    result = dispatcher.dispatch(command)

    if raw:
        console.print(json.dumps(result.__dict__, indent=2))
    else:
        if result.success:
            console.print(f"[green]✓[/green] /{result.command} ({result.duration_ms:.0f}ms)")
            if result.output:
                console.print(result.output)
        else:
            console.print(f"[red]✗[/red] /{result.command}")
            if result.error:
                console.print(f"[red]{result.error}[/red]")
            sys.exit(1)


@cli.command()
@click.option("--built-in", is_flag=True, help="Show only built-in commands")
@click.option("--custom", is_flag=True, help="Show only custom commands")
def list(built_in: bool, custom: bool) -> None:
    """List available commands.

    Args:
        built_in: Show only built-in commands.
        custom: Show only custom commands.
    """
    dispatcher = CommandDispatcher()
    commands = dispatcher.list_commands()

    table = Table(title="Slash Commands")
    table.add_column("Command", style="cyan")
    table.add_column("Type", style="magenta")
    table.add_column("Description", style="green")
    table.add_column("Action", style="yellow")

    if not custom:
        for name, info in commands.get("built_in", {}).items():
            table.add_row(f"/{name}", "built-in", info["description"], info["action"])

    if not built_in:
        for name, info in commands.get("custom", {}).items():
            table.add_row(f"/{name}", "custom", info["description"], info["action"])

    console.print(table)


@cli.command()
@click.argument("name", required=True)
@click.argument("description", required=True)
@click.argument("action", required=True)
@click.option("--type", "action_type", default="shell", type=click.Choice(["shell", "sin", "script"]))
def register(name: str, description: str, action: str, action_type: str) -> None:
    """Register a custom command.

    Args:
        name: Command name (without slash).
        description: Command description.
        action: Command action.
        action_type: Type of action.
    """
    registry = CommandRegistry()
    try:
        cmd = registry.register(name, description, action, action_type)
        console.print(f"[green]✓[/green] Registered /{cmd.name}")
    except ValueError as e:
        console.print(f"[red]✗[/red] {e}")
        sys.exit(1)


@cli.command()
@click.argument("name", required=True)
def remove(name: str) -> None:
    """Remove a custom command.

    Args:
        name: Command name to remove.
    """
    registry = CommandRegistry()
    removed = registry.unregister(name)
    if removed:
        console.print(f"[green]✓[/green] Removed /{name}")
    else:
        console.print(f"[red]✗[/red] Command /{name} not found")
        sys.exit(1)


@cli.command()
@click.option("--limit", default=20, help="Number of records")
def history(limit: int) -> None:
    """Show command history.

    Args:
        limit: Number of records to show.
    """
    dispatcher = CommandDispatcher()
    records = dispatcher.get_history(limit=limit)

    table = Table(title="Command History")
    table.add_column("Time", style="cyan")
    table.add_column("Command", style="magenta")
    table.add_column("Success", style="green")
    table.add_column("Duration", style="yellow")

    for record in records:
        table.add_row(
            record["timestamp"],
            f"/{record['command']}",
            "✓" if record["success"] else "✗",
            f"{record['duration_ms']:.0f}ms",
        )

    console.print(table)


@cli.command()
@click.argument("command", required=True)
def help(command: str) -> None:
    """Show help for a command.

    Args:
        command: Command name to show help for.
    """
    dispatcher = CommandDispatcher()
    help_text = dispatcher.get_command_help(command)
    if help_text:
        console.print(Panel(help_text, title=f"/{command}"))
    else:
        console.print(f"[red]Unknown command: /{command}[/red]")
        sys.exit(1)


def main() -> None:
    """Main entry point."""
    cli()


if __name__ == "__main__":
    main()
