# Purpose: Track installed skills in a local SQLite database
# Docs: registry.py.doc.md
"""
Local registry for installed skills.

Uses SQLite to persist metadata about which skills are installed,
where they came from, and when they were last updated.
"""

import logging
import sqlite3
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

# ── Logger ──────────────────────────────────────────────────────────────────
logger = logging.getLogger(__name__)

# ── Schema ────────────────────────────────────────────────────────────────────
_SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS skills (
    slug TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    title TEXT,
    description TEXT,
    source TEXT NOT NULL,
    destination TEXT NOT NULL,
    installed_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version TEXT
);

CREATE TABLE IF NOT EXISTS catalog_meta (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
);
"""

DEFAULT_DB_PATH = Path.home() / ".config" / "opencode" / "sin_marketplace.db"


# ── Types ─────────────────────────────────────────────────────────────────────
SkillRecord = dict[str, Any]


class RegistryError(Exception):
    """Raised when registry operations fail."""

    pass


# ── Registry ──────────────────────────────────────────────────────────────────
class Registry:
    """SQLite-backed registry of installed skills.

    Every installed skill gets a row with metadata about source repo,
    installation path, and timestamps.
    """

    def __init__(self, db_path: Path | str | None = None) -> None:
        """Open (or create) the SQLite registry.

        Args:
            db_path: Path to SQLite file. Defaults to
                     ``~/.config/opencode/sin_marketplace.db``.
        """
        self.db_path = Path(db_path) if db_path else DEFAULT_DB_PATH
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self._init_db()

    def _init_db(self) -> None:
        """Initialize schema if tables do not exist."""
        with sqlite3.connect(self.db_path) as conn:
            conn.executescript(_SCHEMA_SQL)
            logger.debug("Registry schema ensured at %s", self.db_path)

    # ── CRUD ──────────────────────────────────────────────────────────────────
    def install(self, record: SkillRecord) -> None:
        """Register a skill as installed.

        Args:
            record: Skill metadata. Must contain at least ``slug`` and
                    ``source``. ``destination`` is also required.

        Raises:
            RegistryError: If required fields are missing.
        """
        required = {"slug", "source", "destination"}
        missing = required - set(record.keys())
        if missing:
            raise RegistryError(f"Missing required fields: {missing}")

        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                """
                INSERT INTO skills (slug, name, title, description, source,
                                    destination, installed_at, updated_at, version)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT(slug) DO UPDATE SET
                    name = excluded.name,
                    title = excluded.title,
                    description = excluded.description,
                    source = excluded.source,
                    destination = excluded.destination,
                    updated_at = excluded.updated_at,
                    version = excluded.version
                """,
                (
                    record["slug"],
                    record.get("name", record["slug"]),
                    record.get("title"),
                    record.get("description"),
                    record["source"],
                    record["destination"],
                    record.get("installed_at", now),
                    now,
                    record.get("version"),
                ),
            )
        logger.info("Registered skill %s", record["slug"])

    def remove(self, slug: str) -> bool:
        """Remove a skill from the registry.

        Args:
            slug: Skill identifier to delete.

        Returns:
            True if a row was deleted, False if not found.
        """
        with sqlite3.connect(self.db_path) as conn:
            cur = conn.execute("DELETE FROM skills WHERE slug = ?", (slug,))
            deleted = cur.rowcount > 0
        if deleted:
            logger.info("Removed skill %s", slug)
        else:
            logger.warning("Skill %s not found in registry", slug)
        return deleted

    def get(self, slug: str) -> SkillRecord | None:
        """Retrieve a single installed skill by slug.

        Args:
            slug: Skill identifier.

        Returns:
            Record dict, or ``None`` if not installed.
        """
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            row = conn.execute(
                "SELECT * FROM skills WHERE slug = ?", (slug,)
            ).fetchone()
        return dict(row) if row else None

    def list_all(self) -> list[SkillRecord]:
        """Return all installed skills.

        Returns:
            List of record dicts ordered by installation time.
        """
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute(
                "SELECT * FROM skills ORDER BY installed_at"
            ).fetchall()
        return [dict(row) for row in rows]

    def exists(self, slug: str) -> bool:
        """Check if a skill is registered.

        Args:
            slug: Skill identifier.

        Returns:
            True if the skill is in the registry.
        """
        with sqlite3.connect(self.db_path) as conn:
            row = conn.execute(
                "SELECT 1 FROM skills WHERE slug = ?", (slug,)
            ).fetchone()
        return row is not None

    def update_timestamp(self, slug: str) -> bool:
        """Bump the ``updated_at`` field for a skill.

        Args:
            slug: Skill identifier.

        Returns:
            True if the skill was found and updated.
        """
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        with sqlite3.connect(self.db_path) as conn:
            cur = conn.execute(
                "UPDATE skills SET updated_at = ? WHERE slug = ?",
                (now, slug),
            )
            updated = cur.rowcount > 0
        if updated:
            logger.info("Updated timestamp for %s", slug)
        return updated

    def set_meta(self, key: str, value: str) -> None:
        """Store a catalog metadata key/value.

        Args:
            key: Metadata key (e.g., ``last_sync``).
            value: Metadata value.
        """
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                """
                INSERT INTO catalog_meta (key, value) VALUES (?, ?)
                ON CONFLICT(key) DO UPDATE SET value = excluded.value
                """,
                (key, value),
            )

    def get_meta(self, key: str) -> str | None:
        """Retrieve a catalog metadata value.

        Args:
            key: Metadata key.

        Returns:
            Stored value, or ``None``.
        """
        with sqlite3.connect(self.db_path) as conn:
            row = conn.execute(
                "SELECT value FROM catalog_meta WHERE key = ?", (key,)
            ).fetchone()
        return row[0] if row else None

    def clear(self) -> None:
        """Delete all skills from the registry."""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM skills")
        logger.warning("Registry cleared")

    def __len__(self) -> int:
        """Return number of installed skills."""
        with sqlite3.connect(self.db_path) as conn:
            row = conn.execute("SELECT COUNT(*) FROM skills").fetchone()
        return row[0] if row else 0
