# Purpose: Typer subcommand for `sin marketplace <sub>` (issue #29)
# Docs: marketplace.doc.md
"""Typer subcommand exposing ``sin marketplace ...``.

Originally the standalone ``sin-marketplace-skill`` MCP skill; merged into
sin-code-bundle v0.9.3 (issue #29). Exposes the catalog/installer surface as::

    sin marketplace search <query>    # search the catalog
    sin marketplace install <slug>    # install a skill
    sin marketplace list              # list installed skills
    sin marketplace remove <slug>     # uninstall a skill
    sin marketplace update [slug]     # update one or all skills
    sin marketplace sync              # sync catalog with Infra-SIN-OpenCode-Stack
    sin marketplace info <slug>       # show details
"""

from __future__ import annotations

from pathlib import Path
from typing import Optional

import typer

# `app` becomes `sin marketplace <sub>` after `app.add_typer(app, name="marketplace")`.
app = typer.Typer(
    name="marketplace",
    help="Manage OpenSIN-Code skills via the marketplace catalog.",
    no_args_is_help=True,
)


def _typer_echo(msg: str) -> None:
    """Lightweight indirection so we can swap in rich.console later if desired."""
    typer.echo(msg)


def _run(coro):
    """Run an async coroutine from a sync Typer command."""
    import asyncio

    return asyncio.run(coro)


# Sub-apps
search_app = typer.Typer(help="Search the catalog.", invoke_without_command=False)
install_app = typer.Typer(help="Install / remove skills.")
update_app = typer.Typer(help="Update installed skills.")


# ── search ───────────────────────────────────────────────────────────────
@app.command("search")
def marketplace_search(
    query: str = typer.Argument(..., help="Keyword to search for."),
    remote: bool = typer.Option(False, "--remote", "-r", help="Fetch remote catalog first."),
    json_out: bool = typer.Option(False, "--json", "-j", help="Output JSON."),
) -> None:
    """Search skills by name/category/keyword."""
    from .catalog import Catalog, CatalogError

    if remote:
        catalog = Catalog()
        try:
            _run(catalog.load_remote())
        except CatalogError as exc:
            _typer_echo(f"[FAIL] {exc}")
            raise typer.Exit(code=1)
    else:
        cache = Path.home() / ".config" / "opencode" / "skills_catalog.json"
        if not cache.exists():
            _typer_echo("[FAIL] No local catalog. Use --remote or run sync first.")
            raise typer.Exit(code=1)
        catalog = Catalog()
        catalog.load_file(cache)
    results = catalog.search(query)
    if json_out:
        import json as _json
        _typer_echo(_json.dumps(results, indent=2))
        return
    if not results:
        _typer_echo(f"(no results for '{query}')")
        return
    for entry in results:
        desc = (entry.get("description", "") or "")[:60]
        _typer_echo(f"  {entry.get('slug', '?'):30s} {entry.get('name', '?'):20s} {desc}")


# ── install / list / remove / info ──────────────────────────────────────
@app.command("install")
def marketplace_install(
    slug: str = typer.Argument(..., help="Skill slug to install."),
    remote: bool = typer.Option(True, "--remote/--local", help="Fetch remote catalog."),
) -> None:
    """Install a skill from the catalog."""
    from .catalog import Catalog, CatalogError
    from .installer import InstallError, Installer
    from .registry import Registry

    if remote:
        catalog = Catalog()
        try:
            _run(catalog.load_remote())
        except CatalogError as exc:
            _typer_echo(f"[FAIL] {exc}")
            raise typer.Exit(code=1)
    else:
        cache = Path.home() / ".config" / "opencode" / "skills_catalog.json"
        catalog = Catalog()
        catalog.load_file(cache)
    entry = catalog.get_by_slug(slug)
    if not entry:
        _typer_echo(f"[FAIL] Skill '{slug}' not found in catalog.")
        raise typer.Exit(code=1)
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
        _typer_echo(f"[FAIL] {exc}")
        raise typer.Exit(code=1)
    Registry().install(record)
    _typer_echo(f"[OK] Installed {slug} -> {record['destination']}")


@app.command("list")
def marketplace_list(
    json_out: bool = typer.Option(False, "--json", "-j", help="Output JSON."),
) -> None:
    """List installed skills."""
    from .registry import Registry

    skills = Registry().list_all()
    if json_out:
        import json as _json
        _typer_echo(_json.dumps(skills, indent=2))
        return
    if not skills:
        _typer_echo("(no skills installed)")
        return
    for entry in skills:
        _typer_echo(
            f"  {entry['slug']:30s} {entry.get('name', '?'):20s} "
            f"v{entry.get('version', '?')}  {entry.get('installed_at', '?')}"
        )


@app.command("remove")
def marketplace_remove(
    slug: str = typer.Argument(..., help="Skill slug to remove."),
    force: bool = typer.Option(False, "--force", "-f", help="Skip confirmation."),
) -> None:
    """Remove a skill."""
    from .installer import Installer
    from .registry import Registry

    if not force:
        if not typer.confirm(f"Remove skill '{slug}'?"):
            _typer_echo("Aborted.")
            raise typer.Exit(code=0)
    removed = Installer().remove(slug)
    Registry().remove(slug)
    if removed:
        _typer_echo(f"[OK] Removed {slug}")
    else:
        _typer_echo(f"(skill '{slug}' was not installed)")


@app.command("update")
def marketplace_update(
    slug: Optional[str] = typer.Argument(None, help="Skill slug (omit for all)."),
    check: bool = typer.Option(False, "--check", "-c", help="Check only, do not pull."),
    json_out: bool = typer.Option(False, "--json", "-j", help="Output JSON."),
) -> None:
    """Update installed skills (one or all)."""
    import json as _json
    from .updater import Updater

    updater = Updater()
    if slug:
        if check:
            result = updater.check_status(slug)
        else:
            result = updater.update(slug)
        _typer_echo(_json.dumps(result, indent=2))
        return
    if check:
        results = updater.check_all()
    else:
        results = updater.update_all()
    if json_out:
        _typer_echo(_json.dumps(results, indent=2))
        return
    if not results:
        _typer_echo("(no installed skills)")
        return
    for result in results:
        status = "OK" if result.get("success") or not result.get("behind") else "FAIL"
        _typer_echo(f"  {result['slug']:30s} {status}  {result.get('message', '')}")


@app.command("sync")
def marketplace_sync() -> None:
    """Sync catalog with Infra-SIN-OpenCode-Stack."""
    import json as _json
    from .catalog import Catalog, CatalogError
    from .registry import Registry

    catalog = Catalog()
    try:
        _run(catalog.load_remote())
    except CatalogError as exc:
        _typer_echo(f"[FAIL] {exc}")
        raise typer.Exit(code=1)
    cache = Path.home() / ".config" / "opencode" / "skills_catalog.json"
    cache.parent.mkdir(parents=True, exist_ok=True)
    cache.write_text(_json.dumps(catalog.list_skills(), indent=2), encoding="utf-8")
    _typer_echo(f"[OK] Synced {len(catalog)} skills -> {cache}")
    registry = Registry()
    skills = catalog.list_skills()
    if skills:
        registry.set_meta("last_sync", skills[0].get("updated_at", "unknown"))


@app.command("info")
def marketplace_info(
    slug: str = typer.Argument(..., help="Skill slug to inspect."),
    remote: bool = typer.Option(False, "--remote", "-r", help="Fetch remote catalog."),
) -> None:
    """Show detailed info about a skill."""
    from .catalog import Catalog, CatalogError
    from .registry import Registry

    if remote:
        catalog = Catalog()
        try:
            _run(catalog.load_remote())
        except CatalogError as exc:
            _typer_echo(f"[FAIL] {exc}")
            raise typer.Exit(code=1)
    else:
        cache = Path.home() / ".config" / "opencode" / "skills_catalog.json"
        if not cache.exists():
            _typer_echo("[FAIL] No local catalog. Use --remote or run sync first.")
            raise typer.Exit(code=1)
        catalog = Catalog()
        catalog.load_file(cache)
    entry = catalog.get_by_slug(slug)
    if not entry:
        _typer_echo(f"[FAIL] Skill '{slug}' not found in catalog.")
        raise typer.Exit(code=1)
    installed = Registry().get(slug)
    _typer_echo(f"  Name:        {entry.get('name', slug)}")
    _typer_echo(f"  Slug:        {entry.get('slug')}")
    _typer_echo(f"  Title:       {entry.get('title')}")
    _typer_echo(f"  Description: {entry.get('description')}")
    _typer_echo(f"  Source:      {entry.get('source')}")
    _typer_echo(f"  Destination: {entry.get('destination')}")
    if installed:
        _typer_echo(f"  Installed:   {installed['installed_at']}")
    else:
        _typer_echo("  Installed:   no")
