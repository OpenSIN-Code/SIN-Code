# SPDX-License-Identifier: MIT
"""Git sub-commands — extracted from cli.py."""

from __future__ import annotations

import subprocess

import typer

from sin_code_bundle.cli_app import git_app


@git_app.command("status")
def git_status(
    path: str = typer.Argument(".", help="Repository path."),
):
    """Show git status with a clean summary."""
    result = subprocess.run(
        ["git", "-C", path, "status", "--short"], capture_output=True, text=True
    )
    if result.returncode != 0:
        typer.echo(f"[SIN-BUNDLE] Not a git repository: {path}", err=True)
        raise typer.Exit(code=1)

    lines = result.stdout.strip().splitlines()
    if not lines:
        typer.echo("[SIN-BUNDLE] Working tree clean ✨")
        return

    typer.echo(f"[SIN-BUNDLE] Git status ({len(lines)} changed file(s)):")
    for line in lines:
        typer.echo(f"  {line}")


@git_app.command("commit")
def git_commit(
    message: str = typer.Argument(..., help="Commit message.", metavar="MESSAGE"),
    path: str = typer.Option(".", help="Repository path."),
    all: bool = typer.Option(
        False, "--all", "-a", help="Stage all modified files before committing."
    ),
    push: bool = typer.Option(False, "--push", help="Push after committing."),
):
    """Create a git commit with the given message."""
    if all:
        stage = subprocess.run(["git", "-C", path, "add", "-A"], capture_output=True, text=True)
        if stage.returncode != 0:
            typer.echo(f"[SIN-BUNDLE] Failed to stage files: {stage.stderr}", err=True)
            raise typer.Exit(code=1)

    result = subprocess.run(
        ["git", "-C", path, "commit", "-m", message], capture_output=True, text=True
    )
    if result.returncode != 0:
        typer.echo(f"[SIN-BUNDLE] Commit failed: {result.stderr}", err=True)
        raise typer.Exit(code=1)

    typer.echo(f"[SIN-BUNDLE] Committed: {message}")

    if push:
        push_result = subprocess.run(["git", "-C", path, "push"], capture_output=True, text=True)
        if push_result.returncode != 0:
            typer.echo(f"[SIN-BUNDLE] Push failed: {push_result.stderr}", err=True)
            raise typer.Exit(code=1)
        typer.echo("[SIN-BUNDLE] Pushed to remote ✨")


@git_app.command("clean")
def git_clean(
    path: str = typer.Argument(".", help="Repository path."),
    dry_run: bool = typer.Option(
        True, "--dry-run/--no-dry-run", help="Show what would be deleted without deleting."
    ),
    force: bool = typer.Option(False, "--force", "-f", help="Force delete merged branches."),
):
    """Clean up merged branches and stale references."""
    # Fetch and prune
    subprocess.run(["git", "-C", path, "fetch", "--prune"], capture_output=True, text=True)

    # List merged branches (excluding current, main, master)
    result = subprocess.run(
        ["git", "-C", path, "branch", "--merged"], capture_output=True, text=True
    )
    if result.returncode != 0:
        typer.echo(f"[SIN-BUNDLE] Failed to list branches: {result.stderr}", err=True)
        raise typer.Exit(code=1)

    branches = [
        b.strip() for b in result.stdout.splitlines() if b.strip() and not b.startswith("*")
    ]
    protected = {"main", "master", "develop", "dev"}
    to_delete = [b for b in branches if b not in protected]

    if not to_delete:
        typer.echo("[SIN-BUNDLE] No merged branches to clean up ✨")
        return

    typer.echo(f"[SIN-BUNDLE] Branches to clean up ({len(to_delete)}):")
    for b in to_delete:
        typer.echo(f"  - {b}")

    if dry_run:
        typer.echo("\n[SIN-BUNDLE] Dry-run mode — no branches deleted. Use --no-dry-run to delete.")
        return

    if not force:
        typer.echo("\n[SIN-BUNDLE] Use --force to confirm deletion.")
        return

    for b in to_delete:
        del_result = subprocess.run(
            ["git", "-C", path, "branch", "-d", b], capture_output=True, text=True
        )
        if del_result.returncode == 0:
            typer.echo(f"  ✅ Deleted {b}")
        else:
            typer.echo(f"  ⚠️  Could not delete {b}: {del_result.stderr}", err=True)


@git_app.command("log")
def git_log(
    path: str = typer.Argument(".", help="Repository path."),
    count: int = typer.Option(10, "-n", help="Number of commits to show."),
    oneline: bool = typer.Option(True, "--oneline/--no-oneline", help="Show one-line summary."),
):
    """Show recent commit history."""
    args = ["git", "-C", path, "log", f"-{count}"]
    if oneline:
        args.append("--oneline")
    args.append("--graph")
    args.append("--decorate")

    result = subprocess.run(args, capture_output=True, text=True)
    if result.returncode != 0:
        typer.echo(f"[SIN-BUNDLE] Failed to get log: {result.stderr}", err=True)
        raise typer.Exit(code=1)

    typer.echo(result.stdout)
