# SPDX-License-Identifier: MIT
# Purpose: Dispatch slash commands to the appropriate executor action.
# Docs: dispatcher.doc.md
"""Dispatch slash commands to the appropriate executor action.

The dispatcher is the central router that:
1. Parses the incoming slash command
2. Resolves the command (built-in or custom)
3. Invokes the executor with the correct action
4. Records invocation history
"""

import json
import sqlite3
from dataclasses import dataclass, asdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Optional

from sin_code_bundle.tools.slash.parser import ParsedCommand, SlashParser
from sin_code_bundle.tools.slash.registry import CommandRegistry
from sin_code_bundle.tools.slash.executor import CommandExecutor
from sin_code_bundle.tools.slash.commands import BUILTIN_COMMANDS, get_command_help as get_builtin_help


@dataclass
class DispatchResult:
    """Result of dispatching a slash command.

    Attributes:
        success: Whether the command executed successfully.
        command: The command name that was executed.
        output: Command output (stdout or result).
        error: Error message if the command failed.
        duration_ms: Execution time in milliseconds.
        timestamp: ISO timestamp of execution.
    """

    success: bool
    command: str
    output: str
    error: Optional[str]
    duration_ms: float
    timestamp: str


class CommandDispatcher:
    """Central dispatcher for slash commands.

    Usage:
        dispatcher = CommandDispatcher()
        result = dispatcher.dispatch("/test --verbose tests/unit")
        # result.success == True
        # result.output == "..."
    """

    def __init__(
        self,
        registry: Optional[CommandRegistry] = None,
        executor: Optional[CommandExecutor] = None,
        history_db: Optional[str] = None,
    ) -> None:
        """Initialize the dispatcher.

        Args:
            registry: Custom command registry. Defaults to new instance.
            executor: Command executor. Defaults to new instance.
            history_db: Path to history SQLite database. Defaults to ~/.config/sin-slash/history.db.
        """
        self._parser = SlashParser()
        self._registry = registry or CommandRegistry()
        self._executor = executor or CommandExecutor()
        self._history_db = history_db or str(Path.home() / ".config" / "sin-slash" / "history.db")
        self._init_history_db()

    def _init_history_db(self) -> None:
        """Create the history table if it does not exist."""
        Path(self._history_db).parent.mkdir(parents=True, exist_ok=True)
        with sqlite3.connect(self._history_db) as conn:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS history (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    command TEXT NOT NULL,
                    args TEXT,
                    flags TEXT,
                    success INTEGER NOT NULL,
                    output TEXT,
                    error TEXT,
                    duration_ms REAL,
                    timestamp TEXT NOT NULL
                )
                """
            )
            conn.commit()

    def dispatch(self, raw_command: str) -> DispatchResult:
        """Dispatch a raw slash command.

        Args:
            raw_command: The full slash command string.

        Returns:
            DispatchResult with execution outcome.
        """
        start = datetime.now(timezone.utc)
        timestamp = start.isoformat()

        try:
            parsed = self._parser.parse(raw_command)
        except ValueError as e:
            return DispatchResult(
                success=False,
                command="",
                output="",
                error=str(e),
                duration_ms=0.0,
                timestamp=timestamp,
            )

        command_name = parsed.command
        args = parsed.args
        flags = parsed.flags

        # Check built-in commands first
        if command_name in BUILTIN_COMMANDS:
            action = BUILTIN_COMMANDS[command_name]
            try:
                output = self._executor.execute_builtin(
                    command_name, action, args, flags
                )
                success = True
                error = None
            except Exception as e:
                output = ""
                success = False
                error = f"Execution failed: {e}"
        else:
            # Check custom registry
            custom = self._registry.get(command_name)
            if custom:
                try:
                    output = self._executor.execute_custom(
                        custom, args, flags
                    )
                    success = True
                    error = None
                except Exception as e:
                    output = ""
                    success = False
                    error = f"Custom command execution failed: {e}"
            else:
                output = ""
                success = False
                error = f"Unknown command: /{command_name}"

        duration = (datetime.now(timezone.utc) - start).total_seconds() * 1000

        result = DispatchResult(
            success=success,
            command=command_name,
            output=output,
            error=error,
            duration_ms=duration,
            timestamp=timestamp,
        )

        self._record_history(parsed, result)
        return result

    def _record_history(self, parsed: ParsedCommand, result: DispatchResult) -> None:
        """Record a command invocation in history.

        Args:
            parsed: The parsed command.
            result: The dispatch result.
        """
        with sqlite3.connect(self._history_db) as conn:
            conn.execute(
                "INSERT INTO history (command, args, flags, success, output, error, duration_ms, timestamp) "
                "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    parsed.command,
                    json.dumps(parsed.args),
                    json.dumps(parsed.flags),
                    1 if result.success else 0,
                    result.output,
                    result.error,
                    result.duration_ms,
                    result.timestamp,
                ),
            )
            conn.commit()

    def get_history(self, limit: int = 50) -> list[dict[str, Any]]:
        """Get recent command history.

        Args:
            limit: Maximum number of records to return.

        Returns:
            List of history records as dictionaries.
        """
        with sqlite3.connect(self._history_db) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute(
                "SELECT * FROM history ORDER BY timestamp DESC LIMIT ?",
                (limit,),
            ).fetchall()

        return [
            {
                "id": row["id"],
                "command": row["command"],
                "args": json.loads(row["args"]) if row["args"] else [],
                "flags": json.loads(row["flags"]) if row["flags"] else {},
                "success": bool(row["success"]),
                "output": row["output"],
                "error": row["error"],
                "duration_ms": row["duration_ms"],
                "timestamp": row["timestamp"],
            }
            for row in rows
        ]

    def clear_history(self) -> int:
        """Clear all command history.

        Returns:
            Number of records removed.
        """
        with sqlite3.connect(self._history_db) as conn:
            cursor = conn.execute("DELETE FROM history")
            conn.commit()
            return cursor.rowcount

    def list_commands(self) -> dict[str, Any]:
        """List all available commands (built-in + custom).

        Returns:
            Dictionary with "built_in" and "custom" keys.
        """
        custom = self._registry.list()
        return {
            "built_in": {
                name: {"description": info["description"], "action": info["action"]}
                for name, info in BUILTIN_COMMANDS.items()
            },
            "custom": {cmd.name: asdict(cmd) for cmd in custom},
        }

    def get_command_help(self, name: str) -> Optional[str]:
        """Get help text for a command.

        Args:
            name: Command name to get help for.

        Returns:
            Help text string, or None if command not found.
        """
        if name in BUILTIN_COMMANDS:
            return get_builtin_help(name)

        custom = self._registry.get(name)
        if custom:
            return f"/{custom.name} - {custom.description}\nAction: {custom.action}\nType: {custom.action_type}"

        return None
