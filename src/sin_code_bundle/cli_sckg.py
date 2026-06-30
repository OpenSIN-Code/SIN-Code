# SPDX-License-Identifier: MIT
"""SCKG sub-commands — extracted from cli.py."""
from __future__ import annotations

import shutil
import subprocess

import typer

from sin_code_bundle.cli_app import sckg_app


@sckg_app.command("run")
def sckg_run(
    args: list[str] = typer.Argument(
        default_factory=list, help="Arguments to pass to the sckg CLI"
    ),
):
    """Pass-through to the `sckg` CLI — any subcommand and flags."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo(
            "[SCKG] 'sckg' binary not found. Install: pip install -e ~/SIN-Code-SCKG-Tool", err=True
        )
        raise typer.Exit(code=1)
    result = subprocess.run([binary] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("index")
def sckg_index(
    path: str = typer.Argument(..., help="Repository path to index"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg index"),
):
    """Build a knowledge graph from source code: `sckg index <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "index", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("search")
def sckg_search(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    query: str = typer.Argument(..., help="Search query"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg search"),
):
    """Search the knowledge graph: `sckg search <path> <query>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "search", path, query] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("hot-paths")
def sckg_hot_paths(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(
        default_factory=list, help="Extra arguments for sckg hot-paths"
    ),
):
    """Show most frequently called functions: `sckg hot-paths <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "hot-paths", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("dead-code")
def sckg_dead_code(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(
        default_factory=list, help="Extra arguments for sckg dead-code"
    ),
):
    """Analyze graph for dead code: `sckg dead-code <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "dead-code", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("communities")
def sckg_communities(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(
        default_factory=list, help="Extra arguments for sckg communities"
    ),
):
    """Detect language-aware communities: `sckg communities <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "communities", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("dashboard")
def sckg_dashboard(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(
        default_factory=list, help="Extra arguments for sckg graph (dashboard)"
    ),
):
    """Generate interactive D3.js dashboard: `sckg graph <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "graph", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("adr")
def sckg_adr(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg adr"),
):
    """Generate Architecture Decision Records: `sckg adr <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "adr", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("similar")
def sckg_similar(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    node: str = typer.Argument(..., help="Node to find similar symbols for"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg similar"),
):
    """Find structurally similar symbols: `sckg similar <path> <node>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "similar", path, node] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("serve")
def sckg_serve(
    graph: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg serve"),
):
    """Start the GraphQL API server: `sckg serve <graph>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "serve", graph] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("watch")
def sckg_watch(
    path: str = typer.Argument(..., help="Repository path to watch"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg watch"),
):
    """Watch repo for changes with live GraphQL: `sckg watch <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "watch", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)
