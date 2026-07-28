# SPDX-License-Identifier: MIT
"""MarkItDown sub-commands — extracted from cli.py."""

from __future__ import annotations

import json
from pathlib import Path

import typer

from sin_code_bundle.cli_app import markitdown_app


@markitdown_app.command("doctor")
def markitdown_doctor():
    """Check MarkItDown MCP/CLI availability."""
    from sin_code_bundle import markitdown

    typer.echo(json.dumps(markitdown.doctor(), indent=2))


@markitdown_app.command("setup")
def markitdown_setup(
    agents: str = typer.Option(
        "opencode,codex,hermes",
        help="Comma-separated agents to wire (opencode,codex,hermes).",
    ),
):
    """Wire the MarkItDown MCP server into each coder agent's config."""
    from sin_code_bundle import markitdown

    chosen = [a.strip() for a in agents.split(",") if a.strip()]
    try:
        written = markitdown.setup_agents(chosen)
    except markitdown.MarkItDownError as exc:
        typer.echo(f"[MARKITDOWN] {exc}", err=True)
        raise typer.Exit(code=1)
    for agent, path in written.items():
        typer.echo(f"[MARKITDOWN] wired {agent} -> {path}")
    typer.echo("[MARKITDOWN] Agents can now convert documents to Markdown via MCP.")


@markitdown_app.command("convert")
def markitdown_convert(
    path: Path = typer.Argument(..., help="Document to convert to Markdown"),
):
    """Convert a document (PDF/Office/image/...) to Markdown via the CLI."""
    from sin_code_bundle import markitdown

    try:
        typer.echo(markitdown.convert(str(path)))
    except markitdown.MarkItDownError as exc:
        typer.echo(f"[MARKITDOWN] {exc}", err=True)
        raise typer.Exit(code=1)
