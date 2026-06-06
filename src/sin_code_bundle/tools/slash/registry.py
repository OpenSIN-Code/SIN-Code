# Purpose: SQLite-backed registry for custom slash commands.
# Docs: registry.doc.md
"""SQLite-backed registry for custom slash commands.

Manages user-defined slash commands with CRUD operations. Each command has:
- name: Unique command identifier
- description: Human-readable description
- action: The action to execute (shell command, sin tool reference, or script)
- action_type: "shell", "sin", or "script"
- created_at: Timestamp
- updated_at: Timestamp
"""

import json
import sqlite3
from dataclasses import dataclass, asdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional


@dataclass
class CustomCommand:
    """A user-defined slash command.

    Attributes:
        name: Unique command name (without leading slash).
        description: Human-readable description.
        action: The action to execute.
        action_type: One of "shell", "sin", or "script".
        created_at: ISO timestamp when created.
        updated_at: ISO timestamp when last modified.
    """

    name: str
    description: str
    action: str
    action_type: str
    created_at: str
    updated_at: str


class CommandRegistry:
    """SQLite-backed registry for custom slash commands.

    Usage:
        reg = CommandRegistry("~/.config/sin-slash/registry.db")
        reg.register("deploy", "Deploy to production", "git push origin main", "shell")
        cmd = reg.get("deploy")
        reg.unregister("deploy")
    """

    def __init__(self, db_path: Optional[str] = None) -> None:
        """Initialize registry with a SQLite database.

        Args:
            db_path: Path to the SQLite database. Defaults to ~/.config/sin-slash/registry.db.
        """
        if db_path is None:
            db_path = str(Path.home() / ".config" / "sin-slash" / "registry.db")

        self._db_path = db_path
        Path(db_path).parent.mkdir(parents=True, exist_ok=True)
        self._init_db()

    def _init_db(self) -> None:
        """Create the commands table if it does not exist."""
        with sqlite3.connect(self._db_path) as conn:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS commands (
                    name TEXT PRIMARY KEY,
                    description TEXT NOT NULL,
                    action TEXT NOT NULL,
                    action_type TEXT NOT NULL CHECK(action_type IN ('shell', 'sin', 'script')),
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                )
                """
            )
            conn.commit()

    def register(
        self,
        name: str,
        description: str,
        action: str,
        action_type: str = "shell",
    ) -> CustomCommand:
        """Register a new custom command.

        Args:
            name: Command name (without leading slash).
            description: Human-readable description.
            action: The action to execute.
            action_type: One of "shell", "sin", or "script".

        Returns:
            The registered CustomCommand.

        Raises:
            ValueError: If the command already exists or action_type is invalid.
        """
        if action_type not in ("shell", "sin", "script"):
            raise ValueError(f"Invalid action_type: {action_type}")

        now = datetime.now(timezone.utc).isoformat()
        cmd = CustomCommand(
            name=name,
            description=description,
            action=action,
            action_type=action_type,
            created_at=now,
            updated_at=now,
        )

        with sqlite3.connect(self._db_path) as conn:
            try:
                conn.execute(
                    "INSERT INTO commands (name, description, action, action_type, created_at, updated_at) "
                    "VALUES (?, ?, ?, ?, ?, ?)",
                    (cmd.name, cmd.description, cmd.action, cmd.action_type, cmd.created_at, cmd.updated_at),
                )
                conn.commit()
            except sqlite3.IntegrityError as e:
                raise ValueError(f"Command '{name}' already exists") from e

        return cmd

    def get(self, name: str) -> Optional[CustomCommand]:
        """Get a command by name.

        Args:
            name: Command name to look up.

        Returns:
            CustomCommand if found, None otherwise.
        """
        with sqlite3.connect(self._db_path) as conn:
            conn.row_factory = sqlite3.Row
            row = conn.execute(
                "SELECT * FROM commands WHERE name = ?", (name,)
            ).fetchone()

        if row is None:
            return None

        return CustomCommand(
            name=row["name"],
            description=row["description"],
            action=row["action"],
            action_type=row["action_type"],
            created_at=row["created_at"],
            updated_at=row["updated_at"],
        )

    def unregister(self, name: str) -> bool:
        """Remove a command from the registry.

        Args:
            name: Command name to remove.

        Returns:
            True if the command was removed, False if it did not exist.
        """
        with sqlite3.connect(self._db_path) as conn:
            cursor = conn.execute("DELETE FROM commands WHERE name = ?", (name,))
            conn.commit()
            return cursor.rowcount > 0

    def list(self) -> list[CustomCommand]:
        """List all registered custom commands.

        Returns:
            List of all CustomCommand objects.
        """
        with sqlite3.connect(self._db_path) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute(
                "SELECT * FROM commands ORDER BY name"
            ).fetchall()

        return [
            CustomCommand(
                name=row["name"],
                description=row["description"],
                action=row["action"],
                action_type=row["action_type"],
                created_at=row["created_at"],
                updated_at=row["updated_at"],
            )
            for row in rows
        ]

    def update(
        self,
        name: str,
        description: Optional[str] = None,
        action: Optional[str] = None,
        action_type: Optional[str] = None,
    ) -> Optional[CustomCommand]:
        """Update an existing command.

        Args:
            name: Command name to update.
            description: New description (optional).
            action: New action (optional).
            action_type: New action_type (optional).

        Returns:
            Updated CustomCommand if found, None otherwise.

        Raises:
            ValueError: If action_type is invalid.
        """
        existing = self.get(name)
        if existing is None:
            return None

        if action_type is not None and action_type not in ("shell", "sin", "script"):
            raise ValueError(f"Invalid action_type: {action_type}")

        now = datetime.now(timezone.utc).isoformat()
        updates = {
            "description": description or existing.description,
            "action": action or existing.action,
            "action_type": action_type or existing.action_type,
            "updated_at": now,
        }

        with sqlite3.connect(self._db_path) as conn:
            conn.execute(
                "UPDATE commands SET description=?, action=?, action_type=?, updated_at=? "
                "WHERE name=?",
                (updates["description"], updates["action"], updates["action_type"], updates["updated_at"], name),
            )
            conn.commit()

        return self.get(name)

    def exists(self, name: str) -> bool:
        """Check if a command exists.

        Args:
            name: Command name to check.

        Returns:
            True if the command exists.
        """
        return self.get(name) is not None

    def clear(self) -> int:
        """Remove all commands from the registry.

        Returns:
            Number of commands removed.
        """
        with sqlite3.connect(self._db_path) as conn:
            cursor = conn.execute("DELETE FROM commands")
            conn.commit()
            return cursor.rowcount

    def to_dict(self) -> list[dict]:
        """Export all commands as a list of dictionaries.

        Returns:
            List of dict representations.
        """
        return [asdict(cmd) for cmd in self.list()]

    def to_json(self) -> str:
        """Export all commands as JSON.

        Returns:
            JSON string of all commands.
        """
        return json.dumps(self.to_dict(), indent=2)
