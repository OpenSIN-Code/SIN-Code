# SPDX-License-Identifier: MIT
"""GitNexus sub-commands — extracted from cli.py."""

from __future__ import annotations

import json

import typer

from sin_code_bundle.cli_app import gitnexus_app


@gitnexus_app.command("doctor")
def gitnexus_doctor(root: str = typer.Argument(".", help="Repository root")):
    """Check Node/npx + GitNexus index health."""
    from sin_code_bundle import gitnexus

    typer.echo(json.dumps(gitnexus.doctor(root), indent=2))


@gitnexus_app.command("setup")
def gitnexus_setup(
    agents: str = typer.Option(
        "opencode,codex,hermes",
        help="Comma-separated agents to wire (opencode,codex,hermes).",
    ),
):
    """Wire the GitNexus MCP server into each coder agent's config."""
    from sin_code_bundle import gitnexus

    chosen = [a.strip() for a in agents.split(",") if a.strip()]
    try:
        written = gitnexus.setup_agents(chosen)
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)
    for agent, path in written.items():
        typer.echo(f"[GITNEXUS] wired {agent} -> {path}")
    typer.echo("[GITNEXUS] Agents now have mandatory graph context via MCP.")


@gitnexus_app.command("index")
def gitnexus_index(
    root: str = typer.Argument(".", help="Repository root"),
    force: bool = typer.Option(False, "--force", help="Rebuild even if fresh."),
):
    """Build or refresh the GitNexus index for a repository."""
    from sin_code_bundle import gitnexus

    try:
        if force:
            gitnexus.analyze(root)
            state = gitnexus.index_state(root)
        else:
            state = gitnexus.ensure_index(root, auto=True)
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)
    typer.echo(json.dumps(state.to_dict(), indent=2))


@gitnexus_app.command("status")
def gitnexus_status(root: str = typer.Argument(".", help="Repository root")):
    """Show the on-disk index state without invoking GitNexus."""
    from sin_code_bundle import gitnexus

    typer.echo(json.dumps(gitnexus.index_state(root).to_dict(), indent=2))


@gitnexus_app.command("context")
def gitnexus_context(
    symbol: str = typer.Argument(..., help="Symbol / FQID to inspect"),
    root: str = typer.Option(".", help="Repository root"),
):
    """Structural context for a symbol from the graph."""
    from sin_code_bundle import gitnexus

    try:
        gitnexus.ensure_index(root, auto=True)
        typer.echo(gitnexus.context(symbol, root=root))
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)


@gitnexus_app.command("impact")
def gitnexus_impact(
    symbol: str = typer.Argument(..., help="Symbol / FQID to analyze"),
    root: str = typer.Option(".", help="Repository root"),
):
    """Blast-radius impact analysis for a symbol."""
    from sin_code_bundle import gitnexus

    try:
        gitnexus.ensure_index(root, auto=True)
        typer.echo(gitnexus.impact(symbol, root=root))
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)


@gitnexus_app.command("ai-context")
def gitnexus_ai_context(
    task: str = typer.Argument(..., help="Task description to scope context to"),
    root: str = typer.Option(".", help="Repository root"),
):
    """Task-scoped, graph-aware context bundle for an agent."""
    from sin_code_bundle import gitnexus

    try:
        gitnexus.ensure_index(root, auto=True)
        typer.echo(gitnexus.ai_context(task, root=root))
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)
