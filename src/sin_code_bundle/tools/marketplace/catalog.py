# Purpose: Read and query the Infra-SIN-OpenCode-Stack skills catalog
# Docs: catalog.py.doc.md
"""
Catalog reader for the OpenSIN-Code skill marketplace.

Handles fetching, parsing, and querying the canonical skills catalog
from the Infra-SIN-OpenCode-Stack repository.
"""

import json
import logging
from pathlib import Path
from typing import Any

import httpx

# ── Logger ──────────────────────────────────────────────────────────────────
logger = logging.getLogger(__name__)

# ── Constants ───────────────────────────────────────────────────────────────
CATALOG_URL = (
    "https://raw.githubusercontent.com/"
    "OpenSIN-Code/Infra-SIN-OpenCode-Stack/main/skills/catalog.json"
)
DEFAULT_TIMEOUT = 30.0

# ── Types ─────────────────────────────────────────────────────────────────────
SkillEntry = dict[str, Any]
CatalogData = list[SkillEntry]


class CatalogError(Exception):
    """Raised when catalog operations fail."""

    pass


# ── Catalog Reader ────────────────────────────────────────────────────────────
class Catalog:
    """In-memory representation of the skills catalog.

    Provides search, filtering, and metadata access for all published
    skills in the OpenSIN-Code ecosystem.
    """

    def __init__(self, data: CatalogData | None = None) -> None:
        """Initialize with optional pre-loaded data.

        Args:
            data: Pre-loaded catalog entries. If None, the catalog is empty
                  until ``load_remote()`` or ``load_file()`` is called.
        """
        self._entries: CatalogData = list(data) if data else []

    # ── Loading ─────────────────────────────────────────────────────────────
    async def load_remote(self, url: str = CATALOG_URL, timeout: float = DEFAULT_TIMEOUT) -> None:
        """Fetch catalog from remote URL.

        Args:
            url: Raw GitHub URL to ``catalog.json``.
            timeout: HTTP request timeout in seconds.

        Raises:
            CatalogError: On network failure or invalid JSON.
        """
        logger.info("Fetching catalog from %s", url)
        try:
            async with httpx.AsyncClient(timeout=timeout) as client:
                response = await client.get(url)
                response.raise_for_status()
        except httpx.HTTPError as exc:
            raise CatalogError(f"Failed to fetch catalog: {exc}") from exc

        try:
            payload = response.json()
        except json.JSONDecodeError as exc:
            raise CatalogError(f"Invalid JSON in catalog: {exc}") from exc

        if not isinstance(payload, list):
            raise CatalogError(f"Catalog root must be a list, got {type(payload).__name__}")

        self._entries = payload
        logger.info("Loaded %d skills from catalog", len(self._entries))

    def load_file(self, path: Path | str) -> None:
        """Load catalog from local JSON file.

        Args:
            path: Filesystem path to ``catalog.json``.

        Raises:
            CatalogError: On read or parse failure.
        """
        path = Path(path)
        logger.info("Loading catalog from %s", path)
        try:
            with path.open("r", encoding="utf-8") as fh:
                payload = json.load(fh)
        except (OSError, json.JSONDecodeError) as exc:
            raise CatalogError(f"Failed to load catalog file: {exc}") from exc

        if not isinstance(payload, list):
            raise CatalogError(f"Catalog root must be a list, got {type(payload).__name__}")

        self._entries = payload
        logger.info("Loaded %d skills from %s", len(self._entries), path)

    # ── Queries ───────────────────────────────────────────────────────────────
    def list_skills(self) -> CatalogData:
        """Return all catalog entries.

        Returns:
            Copy of the internal catalog list.
        """
        return list(self._entries)

    def search(self, query: str) -> CatalogData:
        """Search skills by keyword across name, title, description, and slug.

        The search is case-insensitive and matches any substring.

        Args:
            query: Keyword or phrase to search for.

        Returns:
            Matching skill entries.
        """
        query_lower = query.lower()
        matches: CatalogData = []
        for entry in self._entries:
            text = " ".join(
                str(entry.get(k, ""))
                for k in ("name", "title", "description", "slug")
            ).lower()
            if query_lower in text:
                matches.append(entry)
        logger.debug("Search '%s' → %d matches", query, len(matches))
        return matches

    def get_by_slug(self, slug: str) -> SkillEntry | None:
        """Retrieve a single skill by its slug.

        Args:
            slug: Unique skill identifier.

        Returns:
            The skill entry, or ``None`` if not found.
        """
        for entry in self._entries:
            if entry.get("slug") == slug:
                return entry
        return None

    def get_by_name(self, name: str) -> SkillEntry | None:
        """Retrieve a single skill by its name field.

        Args:
            name: Skill name (not necessarily unique).

        Returns:
            The first matching skill entry, or ``None``.
        """
        for entry in self._entries:
            if entry.get("name") == name:
                return entry
        return None

    def filter_by_category(self, category: str) -> CatalogData:
        """Filter skills by category tag.

        Args:
            category: Category string to match.

        Returns:
            Skills whose ``category`` field equals the query.
        """
        cat_lower = category.lower()
        return [
            entry
            for entry in self._entries
            if entry.get("category", "").lower() == cat_lower
        ]

    def __len__(self) -> int:
        """Return number of loaded skills."""
        return len(self._entries)

    def __bool__(self) -> bool:
        """Return True if catalog has entries."""
        return bool(self._entries)
