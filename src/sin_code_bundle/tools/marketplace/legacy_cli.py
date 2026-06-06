# Purpose: CLI entry point for sin-marketplace commands
# Docs: cli.py.doc.md
"""
Typer-based CLI for the SIN Marketplace skill.

Provides human-friendly commands for searching, installing, listing,
removing, and updating skills.
"""

import asyncio
import json
import logging
from pathlib import Path
from typing import Any

import typer
from rich.console import Console
from rich.table import Table

# ── Local imports ─────────────────────────────────────────────────────────────
from .catalog import Catalog, CatalogError
from .installer import InstallError, Installer
from .registry import Registry
from .updater import Updater

# ── App ───────────────────────────────────────────────────────────────────────
app = typer.Typer(help="SIN Marketplace — manage OpenSIN-Code skills")
console = Console()

# ── Helpers ───────────────────────────────────────────────────────────────────
def _get_catalog(cache_path: Path | None = None) -> Catalog:
    """Load or create a catalog instance.
    
    Args:
        cache_path: Optional path to local catalog cache. Defaults to
                    ~/.config/opencode/skills_catalog.json.
    """
    catalog = Catalog()
    if cache_path is None:
        cache_path = Path.home() / ".config" / "opencode" / "skills_catalog.json"
    if cache_path.exists():
        catalog.load_file(cache_path)
    return catalog


# ── Commands ──────────────────────────────────────────────────────────────────
@app.command("search")
def search(
    query: str = typer.Argument(..., help="Keyword to search for"),
    remote: bool = typer.Option(False, "--remote", "-r", help="Fetch remote catalog first"),
    json_out: bool = typer.Option(False, "--json", "-j", help="Output JSON"),
) -> None:
    """Search skills by name/category/keyword."""
    if remote:
        catalog = Catalog()
        try:
            asyncio.run(catalog.load_remote())
        except CatalogError as exc:
            console.print(f"[red]Catalog error: {exc}[/red]")
            raise typer.Exit(1)
    else:
        console.print("[yellow]Loading local catalog...[/yellow]")
        catalog = _get_catalog()
        if not catalog:
            console.print("[red]No local catalog. Use --remote or run sync first.[/red]")
            raise typer.Exit(1)

    results = catalog.search(query)
    if json_out:
        console.print(json.dumps(results, indent=2))
    else:
        table = Table(title=f"Search results for '{query}'")
        table.add_column("Slug", style="cyan")
        table.add_column("Name", style="green")
        table.add_column("Title", style="white")
        table.add_column("Description", style="dim")
        for entry in results:
            table.add_row(
                entry.get("slug", "?"),
                entry.get("name", "?"),
                entry.get("title", "?"),
                entry.get("description", "")[:60] + "...",
            )
        console.print(table)


@app.command("install")
def install(
    slug: str = typer.Argument(..., help="Skill slug to install"),
    remote: bool = typer.Option(True, "--remote/--local", "-r/-l", help="Fetch remote catalog"),
) -> None:
    """Install a skill from the catalog."""
    if remote:
        catalog = Catalog()
        try:
            asyncio.run(catalog.load_remote())
        except CatalogError as exc:
            console.print(f"[red]Catalog error: {exc}[/red]")
            raise typer.Exit(1)
    else:
        catalog = _get_catalog()
        if not catalog:
            console.print("[red]No local catalog. Use --remote or run sync first.[/red]")
            raise typer.Exit(1)

    entry = catalog.get_by_slug(slug)
    if not entry:
        console.print(f"[red]Skill '{slug}' not found in catalog.[/red]")
        raise typer.Exit(1)

    installer = Installer()
    try:
        record = installer.install(
            slug=slug,
            source=entry["source"],
            destination=entry.get("destination", slug),
            name=entry.get("name", slug),
            title=entry.get("title"),
            description=entry.get("description"),
        )
    except InstallError as exc:
        console.print(f"[red]Install failed: {exc}[/red]")
        raise typer.Exit(1)

    registry = Registry()
    registry.install(record)
    console.print(f"[green]Installed {slug} → {record['destination']}[/green]")


@app.command("list")
def list_(
    json_out: bool = typer.Option(False, "--json", "-j", help="Output JSON"),
) -> None:
    """List installed skills."""
    registry = Registry()
    skills = registry.list_all()
    if json_out:
        console.print(json.dumps(skills, indent=2))
    else:
        table = Table(title="Installed Skills")
        table.add_column("Slug", style="cyan")
        table.add_column("Name", style="green")
        table.add_column("Version", style="yellow")
        table.add_column("Installed", style="dim")
        for entry in skills:
            table.add_row(
                entry["slug"],
                entry.get("name", "?"),
                entry.get("version", "?"),
                entry.get("installed_at", "?"),
            )
        console.print(table)


@app.command("remove")
def remove(
    slug: str = typer.Argument(..., help="Skill slug to remove"),
    force: bool = typer.Option(False, "--force", "-f", help="Skip confirmation"),
) -> None:
    """Remove a skill."""
    if not force:
        confirm = typer.confirm(f"Remove skill '{slug}'?")
        if not confirm:
            console.print("[yellow]Aborted.[/yellow]")
            raise typer.Exit(0)

    installer = Installer()
    removed = installer.remove(slug)
    registry = Registry()
    registry.remove(slug)
    if removed:
        console.print(f"[green]Removed {slug}[/green]")
    else:
        console.print(f"[yellow]Skill '{slug}' was not installed.[/yellow]")


@app.command("update")
def update(
    slug: str | None = typer.Argument(None, help="Skill slug (omit for all)"),
    check: bool = typer.Option(False, "--check", "-c", help="Check only, do not pull"),
) -> None:
    """Update installed skills."""
    updater = Updater()
    if slug:
        if check:
            result = updater.check_status(slug)
        else:
            result = updater.update(slug)
        console.print(json.dumps(result, indent=2))
    else:
        if check:
            results = updater.check_all()
        else:
            results = updater.update_all()
        table = Table(title="Update Results")
        table.add_column("Slug", style="cyan")
        table.add_column("Status", style="green")
        table.add_column("Message", style="white")
        for result in results:
            status = "✓" if result.get("success") or not result.get("behind") else "✗"
            table.add_row(result["slug"], status, result["message"])
        console.print(table)


@app.command("sync")
def sync() -> None:
    """Sync catalog with Infra-SIN-OpenCode-Stack."""
    catalog = Catalog()
    try:
        asyncio.run(catalog.load_remote())
    except CatalogError as exc:
        console.print(f"[red]Sync failed: {exc}[/red]")
        raise typer.Exit(1)

    cache_path = Path.home() / ".config" / "opencode" / "skills_catalog.json"
    cache_path.parent.mkdir(parents=True, exist_ok=True)
    import json
    with cache_path.open("w", encoding="utf-8") as fh:
        json.dump(catalog.list_skills(), fh, indent=2)
    console.print(f"[green]Synced {len(catalog)} skills → {cache_path}[/green]")

    registry = Registry()
    registry.set_meta("last_sync", catalog.list_skills()[0].get("updated_at", "unknown"))


@app.command("info")
def info(
    slug: str = typer.Argument(..., help="Skill slug to inspect"),
    remote: bool = typer.Option(False, "--remote", "-r", help="Fetch remote catalog"),
) -> None:
    """Show detailed info about a skill."""
    if remote:
        catalog = Catalog()
        try:
            asyncio.run(catalog.load_remote())
        except CatalogError as exc:
            console.print(f"[red]Catalog error: {exc}[/red]")
            raise typer.Exit(1)
    else:
        catalog = _get_catalog()
        if not catalog:
            console.print("[red]No local catalog. Use --remote or run sync first.[/red]")
            raise typer.Exit(1)

    entry = catalog.get_by_slug(slug)
    if not entry:
        console.print(f"[red]Skill '{slug}' not found in catalog.[/red]")
        raise typer.Exit(1)

    registry = Registry()
    installed = registry.get(slug)
    console.print(f"[bold cyan]{entry.get('name', slug)}[/bold cyan]")
    console.print(f"Slug: {entry.get('slug')}")
    console.print(f"Title: {entry.get('title')}")
    console.print(f"Description: {entry.get('description')}")
    console.print(f"Source: {entry.get('source')}")
    console.print(f"Destination: {entry.get('destination')}")
    if installed:
        console.print(f"[green]Installed: {installed['installed_at']}[/green]")
    else:
        console.print("[yellow]Not installed[/yellow]")


def main() -> None:
    """Entry point for the CLI."""
    app()
