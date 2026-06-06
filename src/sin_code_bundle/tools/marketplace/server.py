# Purpose: MCP server exposing 7 marketplace tools via FastMCP
# Docs: server.py.doc.md
"""
FastMCP server for the SIN Marketplace skill.

Exposes 7 tools that allow AI agents to search, install, list, remove,
update, sync, and inspect skills programmatically.
"""

import json
import logging
from pathlib import Path
from typing import Any

from fastmcp import FastMCP

# ── Local imports ─────────────────────────────────────────────────────────────
from .catalog import Catalog, CatalogError
from .installer import InstallError, Installer
from .registry import Registry
from .updater import Updater

# ── Logger ────────────────────────────────────────────────────────────────────
logger = logging.getLogger(__name__)

# ── MCP Server ───────────────────────────────────────────────────────────────
mcp = FastMCP("sin-marketplace")

# ── Helpers ───────────────────────────────────────────────────────────────────
async def _get_catalog() -> Catalog:
    """Load catalog from remote or local cache."""
    catalog = Catalog()
    cache_path = Path.home() / ".config" / "opencode" / "skills_catalog.json"
    if cache_path.exists():
        catalog.load_file(cache_path)
    else:
        try:
            await catalog.load_remote()
            cache_path.parent.mkdir(parents=True, exist_ok=True)
            with cache_path.open("w", encoding="utf-8") as fh:
                json.dump(catalog.list_skills(), fh, indent=2)
        except CatalogError:
            pass
    return catalog


# ── Tools ─────────────────────────────────────────────────────────────────────
@mcp.tool()
async def marketplace_search(query: str) -> str:
    """Search skills by name, category, or keyword.

    Args:
        query: Search term (case-insensitive substring match).

    Returns:
        JSON string with matching skill entries.
    """
    catalog = await _get_catalog()
    results = catalog.search(query)
    return json.dumps({"query": query, "count": len(results), "results": results}, indent=2)


@mcp.tool()
async def marketplace_install(slug: str) -> str:
    """Install a skill from the catalog.

    Args:
        slug: Unique skill identifier from the catalog.

    Returns:
        JSON string with installation status and path.
    """
    catalog = await _get_catalog()
    entry = catalog.get_by_slug(slug)
    if not entry:
        return json.dumps({"error": f"Skill '{slug}' not found in catalog"}, indent=2)

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
        return json.dumps({"error": str(exc), "slug": slug}, indent=2)

    registry = Registry()
    registry.install(record)
    return json.dumps({"success": True, "slug": slug, "destination": record["destination"]}, indent=2)


@mcp.tool()
async def marketplace_list() -> str:
    """List installed skills.

    Returns:
        JSON string with all installed skills.
    """
    registry = Registry()
    skills = registry.list_all()
    return json.dumps({"count": len(skills), "skills": skills}, indent=2)


@mcp.tool()
async def marketplace_remove(slug: str) -> str:
    """Remove a skill.

    Args:
        slug: Skill identifier to remove.

    Returns:
        JSON string with removal status.
    """
    installer = Installer()
    installer.remove(slug)
    registry = Registry()
    removed = registry.remove(slug)
    return json.dumps({"success": removed, "slug": slug}, indent=2)


@mcp.tool()
async def marketplace_info(slug: str) -> str:
    """Show detailed info about a skill.

    Args:
        slug: Skill identifier to inspect.

    Returns:
        JSON string with catalog and installation metadata.
    """
    catalog = await _get_catalog()
    entry = catalog.get_by_slug(slug)
    registry = Registry()
    installed = registry.get(slug)
    return json.dumps(
        {
            "slug": slug,
            "catalog": entry,
            "installed": installed,
        },
        indent=2,
        default=str,
    )


@mcp.tool()
async def marketplace_update(slug: str | None = None) -> str:
    """Update all skills or a specific skill.

    Args:
        slug: Optional skill identifier. If omitted, updates all skills.

    Returns:
        JSON string with update results.
    """
    updater = Updater()
    if slug:
        result = updater.update(slug)
        return json.dumps({"results": [result]}, indent=2)
    results = updater.update_all()
    return json.dumps({"results": results}, indent=2)


@mcp.tool()
async def marketplace_sync() -> str:
    """Sync catalog with Infra-SIN-OpenCode-Stack.

    Returns:
        JSON string with sync status and skill count.
    """
    catalog = Catalog()
    try:
        await catalog.load_remote()
    except CatalogError as exc:
        return json.dumps({"error": str(exc)}, indent=2)

    cache_path = Path.home() / ".config" / "opencode" / "skills_catalog.json"
    cache_path.parent.mkdir(parents=True, exist_ok=True)
    with cache_path.open("w", encoding="utf-8") as fh:
        json.dump(catalog.list_skills(), fh, indent=2)

    registry = Registry()
    registry.set_meta("last_sync", catalog.list_skills()[0].get("updated_at", "unknown"))
    return json.dumps({"success": True, "count": len(catalog)}, indent=2)


def main() -> None:
    """Run the MCP server."""
    mcp.run()


if __name__ == "__main__":
    main()
