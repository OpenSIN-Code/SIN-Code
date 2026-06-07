# SPDX-License-Identifier: MIT
# Purpose: Tests for the slash command parser.
# Docs: test_parser.doc.md
"""Test the slash command parser.

Tests cover parsing, validation, quoted strings, flags, and edge cases.
"""

import pytest

from sin_code_bundle.tools.slash.parser import SlashParser, ParsedCommand


class TestSlashParser:
    """Tests for SlashParser."""

    def setup_method(self) -> None:
        """Set up parser instance for each test."""
        self.parser = SlashParser()

    def test_parse_simple_command(self) -> None:
        """Parse a simple command without arguments."""
        result = self.parser.parse("/test")
        assert result.command == "test"
        assert result.args == []
        assert result.flags == {}
        assert result.raw == "/test"

    def test_parse_with_args(self) -> None:
        """Parse a command with positional arguments."""
        result = self.parser.parse("/test arg1 arg2")
        assert result.command == "test"
        assert result.args == ["arg1", "arg2"]

    def test_parse_with_long_flag(self) -> None:
        """Parse a command with long flag."""
        result = self.parser.parse("/test --verbose")
        assert result.command == "test"
        assert result.flags == {"verbose": True}

    def test_parse_with_short_flag(self) -> None:
        """Parse a command with short flag."""
        result = self.parser.parse("/test -v")
        assert result.command == "test"
        assert result.flags == {"v": True}

    def test_parse_with_flag_value(self) -> None:
        """Parse a command with flag value."""
        result = self.parser.parse("/test --limit=10")
        assert result.flags == {"limit": 10}

    def test_parse_with_short_flag_value(self) -> None:
        """Parse a command with short flag value."""
        result = self.parser.parse("/test -n=5")
        assert result.flags == {"n": 5}

    def test_parse_with_quoted_string(self) -> None:
        """Parse a command with quoted string argument."""
        result = self.parser.parse('/test "hello world"')
        assert result.args == ["hello world"]

    def test_parse_with_single_quotes(self) -> None:
        """Parse a command with single-quoted string."""
        result = self.parser.parse("/test 'hello world'")
        assert result.args == ["hello world"]

    def test_parse_with_mixed_args_and_flags(self) -> None:
        """Parse a command with both args and flags."""
        result = self.parser.parse("/test arg1 --verbose arg2 --limit=10")
        assert result.args == ["arg1", "arg2"]
        assert result.flags == {"verbose": True, "limit": 10}

    def test_parse_empty_string_raises(self) -> None:
        """Empty string raises ValueError."""
        with pytest.raises(ValueError, match="cannot be empty"):
            self.parser.parse("")

    def test_parse_no_slash_raises(self) -> None:
        """String without leading slash raises ValueError."""
        with pytest.raises(ValueError, match="must start with"):
            self.parser.parse("test")

    def test_parse_only_slash_raises(self) -> None:
        """String with only slash raises ValueError."""
        with pytest.raises(ValueError, match="cannot be empty"):
            self.parser.parse("/")

    def test_parse_invalid_quotes_raises(self) -> None:
        """Invalid quoting raises ValueError."""
        with pytest.raises(ValueError, match="Invalid quoting"):
            self.parser.parse('/test "unclosed')

    def test_coerce_boolean_true(self) -> None:
        """Coerce 'true' to boolean True."""
        result = self.parser.parse("/test --flag=true")
        assert result.flags == {"flag": True}

    def test_coerce_boolean_false(self) -> None:
        """Coerce 'false' to boolean False."""
        result = self.parser.parse("/test --flag=false")
        assert result.flags == {"flag": False}

    def test_coerce_boolean_yes_no(self) -> None:
        """Coerce 'yes' and 'no' to booleans."""
        result = self.parser.parse("/test --a=yes --b=no")
        assert result.flags == {"a": True, "b": False}

    def test_coerce_integer(self) -> None:
        """Coerce numeric string to integer."""
        result = self.parser.parse("/test --count=42")
        assert result.flags == {"count": 42}
        assert isinstance(result.flags["count"], int)

    def test_coerce_float(self) -> None:
        """Coerce decimal string to float."""
        result = self.parser.parse("/test --ratio=3.14")
        assert result.flags == {"ratio": 3.14}
        assert isinstance(result.flags["ratio"], float)

    def test_coerce_string(self) -> None:
        """Keep string as string when not numeric."""
        result = self.parser.parse("/test --name=hello")
        assert result.flags == {"name": "hello"}
        assert isinstance(result.flags["name"], str)

    def test_parse_many(self) -> None:
        """Parse multiple commands at once."""
        commands = ["/test", "/docs --verbose", "/audit --profile=FULL"]
        results = self.parser.parse_many(commands)
        assert len(results) == 3
        assert results[0].command == "test"
        assert results[1].command == "docs"
        assert results[2].command == "audit"

    def test_is_valid_command_true(self) -> None:
        """is_valid_command returns True for valid commands."""
        assert self.parser.is_valid_command("/test") is True
        assert self.parser.is_valid_command("/test arg1") is True

    def test_is_valid_command_false(self) -> None:
        """is_valid_command returns False for invalid commands."""
        assert self.parser.is_valid_command("test") is False
        assert self.parser.is_valid_command("") is False

    def test_parse_whitespace_stripped(self) -> None:
        """Leading/trailing whitespace is stripped."""
        result = self.parser.parse("  /test arg1  ")
        assert result.command == "test"
        assert result.args == ["arg1"]

    def test_parse_underscore_in_flag_name(self) -> None:
        """Flag names can contain underscores."""
        result = self.parser.parse("/test --my_flag=value")
        assert result.flags == {"my_flag": "value"}

    def test_parse_dash_in_flag_name(self) -> None:
        """Flag names can contain dashes."""
        result = self.parser.parse("/test --my-flag=value")
        assert result.flags == {"my-flag": "value"}

    def test_parse_hyphenated_args(self) -> None:
        """Args with hyphens are treated as args, not flags."""
        result = self.parser.parse("/test -some-arg")
        # -some-arg is treated as an argument because it doesn't match short flag pattern
        assert result.args == ["-some-arg"]
        assert result.flags == {}

    def test_parse_multiple_same_flags(self) -> None:
        """Last flag wins when duplicate flags provided."""
        result = self.parser.parse("/test --limit=10 --limit=20")
        assert result.flags == {"limit": 20}
