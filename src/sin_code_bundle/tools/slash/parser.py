# Purpose: Parse slash command syntax with support for arguments, flags, and quoted strings.
# Docs: parser.doc.md
"""Parse slash command syntax.

Handles `/command arg1 arg2 --flag "quoted string"` parsing with support for:
- Single and double quoted strings
- Long and short flags (--flag, -f)
- Flag values (--flag=value)
- Positional arguments
"""

import re
import shlex
from dataclasses import dataclass
from typing import Any


@dataclass
class ParsedCommand:
    """Result of parsing a slash command string.

    Attributes:
        command: The command name (without leading slash).
        args: List of positional arguments.
        flags: Dict of flag names to values (True for boolean flags).
        raw: The original input string.
    """

    command: str
    args: list[str]
    flags: dict[str, Any]
    raw: str


class SlashParser:
    """Parse slash commands into structured components.

    Usage:
        parser = SlashParser()
        result = parser.parse("/test --verbose tests/unit")
        # result.command == "test"
        # result.args == ["tests/unit"]
        # result.flags == {"verbose": True}
    """

    # Regex to detect flag patterns: --flag, --flag=value, -f, -f=value
    _FLAG_RE = re.compile(r"^--([a-zA-Z0-9_-]+)(?:=(.+))?$")
    _SHORT_FLAG_RE = re.compile(r"^-([a-zA-Z0-9])(?:=(.+))?$")

    def __init__(self) -> None:
        """Initialize parser with default settings."""
        self._quote_chars = {'"', "'"}

    def parse(self, raw: str) -> ParsedCommand:
        """Parse a raw slash command string.

        Args:
            raw: The full slash command string, e.g. "/test --verbose tests/unit".

        Returns:
            ParsedCommand with command, args, and flags.

        Raises:
            ValueError: If the string does not start with '/' or is empty.
        """
        stripped = raw.strip()
        if not stripped:
            raise ValueError("Command string cannot be empty")
        if not stripped.startswith("/"):
            raise ValueError(f"Command must start with '/': {raw}")

        stripped = raw.strip()
        # Use shlex to properly handle quoted strings
        try:
            tokens = shlex.split(stripped)
        except ValueError as e:
            raise ValueError(f"Invalid quoting in command: {raw}") from e

        if not tokens:
            raise ValueError("Command string is empty after tokenization")

        command = tokens[0][1:]  # Remove leading slash
        if not command:
            raise ValueError("Command name cannot be empty")

        args: list[str] = []
        flags: dict[str, Any] = {}

        for token in tokens[1:]:
            long_match = self._FLAG_RE.match(token)
            short_match = self._SHORT_FLAG_RE.match(token)

            if long_match:
                flag_name = long_match.group(1)
                flag_value = long_match.group(2)
                flags[flag_name] = self._coerce_value(flag_value) if flag_value is not None else True
            elif short_match:
                flag_name = short_match.group(1)
                flag_value = short_match.group(2)
                flags[flag_name] = self._coerce_value(flag_value) if flag_value is not None else True
            else:
                args.append(token)

        return ParsedCommand(
            command=command,
            args=args,
            flags=flags,
            raw=raw,
        )

    def _coerce_value(self, value: str) -> Any:
        """Coerce a string value to an appropriate type.

        Args:
            value: String value to coerce.

        Returns:
            int, float, bool, or original string.
        """
        # Boolean check
        lowered = value.lower()
        if lowered in ("true", "yes", "on"):
            return True
        if lowered in ("false", "no", "off"):
            return False

        # Integer check
        try:
            return int(value)
        except ValueError:
            pass

        # Float check
        try:
            return float(value)
        except ValueError:
            pass

        return value

    def parse_many(self, commands: list[str]) -> list[ParsedCommand]:
        """Parse multiple slash commands.

        Args:
            commands: List of raw command strings.

        Returns:
            List of ParsedCommand objects.
        """
        return [self.parse(cmd) for cmd in commands]

    def is_valid_command(self, raw: str) -> bool:
        """Check if a string is a valid slash command.

        Args:
            raw: String to validate.

        Returns:
            True if the string appears to be a valid slash command.
        """
        try:
            self.parse(raw)
            return True
        except ValueError:
            return False
