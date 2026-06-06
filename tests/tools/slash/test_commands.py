# Purpose: Tests for built-in commands.
# Docs: test_commands.doc.md
"""Test built-in command definitions.

Tests cover command lookup, help text, and built-in listing.
"""

from sin_code_bundle.tools.slash.commands import (
    BUILTIN_COMMANDS,
    get_builtin_command,
    list_builtin_commands,
    get_command_help,
)


class TestBuiltinCommands:
    """Tests for built-in commands."""

    def test_all_commands_exist(self) -> None:
        """All expected built-in commands exist."""
        expected = {"refactor", "test", "docs", "commit", "audit", "status", "search", "help", "list", "history"}
        assert set(BUILTIN_COMMANDS.keys()) == expected

    def test_refactor_command(self) -> None:
        """Refactor command is correctly defined."""
        cmd = get_builtin_command("refactor")
        assert cmd is not None
        assert cmd["description"] == "Refactor code using symbol resolution and checkpointing"
        assert cmd["action"] == "sin_symbol_resolve"
        assert cmd["type"] == "sin"

    def test_test_command(self) -> None:
        """Test command is correctly defined."""
        cmd = get_builtin_command("test")
        assert cmd is not None
        assert cmd["description"] == "Run tests using pytest"
        assert cmd["action"] == "pytest"
        assert cmd["type"] == "shell"

    def test_docs_command(self) -> None:
        """Docs command is correctly defined."""
        cmd = get_builtin_command("docs")
        assert cmd is not None
        assert cmd["description"] == "Check code documentation with CoDocs"
        assert cmd["action"] == "sin codocs check"
        assert cmd["type"] == "shell"

    def test_commit_command(self) -> None:
        """Commit command is correctly defined."""
        cmd = get_builtin_command("commit")
        assert cmd is not None
        assert cmd["description"] == "Create an immortal commit with conventional format"
        assert cmd["action"] == "sin_immortal_commit"
        assert cmd["type"] == "sin"

    def test_audit_command(self) -> None:
        """Audit command is correctly defined."""
        cmd = get_builtin_command("audit")
        assert cmd is not None
        assert cmd["description"] == "Run CEO audit on the repository"
        assert cmd["action"] == "sin ceo-audit"
        assert cmd["type"] == "shell"

    def test_status_command(self) -> None:
        """Status command is correctly defined."""
        cmd = get_builtin_command("status")
        assert cmd is not None
        assert cmd["description"] == "Show project status and health"
        assert cmd["action"] == "sin status"
        assert cmd["type"] == "shell"

    def test_search_command(self) -> None:
        """Search command is correctly defined."""
        cmd = get_builtin_command("search")
        assert cmd is not None
        assert cmd["description"] == "Search the web using sin websearch"
        assert cmd["action"] == "sin_websearch"
        assert cmd["type"] == "sin"

    def test_help_command(self) -> None:
        """Help command is correctly defined."""
        cmd = get_builtin_command("help")
        assert cmd is not None
        assert cmd["type"] == "python"

    def test_list_command(self) -> None:
        """List command is correctly defined."""
        cmd = get_builtin_command("list")
        assert cmd is not None
        assert cmd["type"] == "python"

    def test_history_command(self) -> None:
        """History command is correctly defined."""
        cmd = get_builtin_command("history")
        assert cmd is not None
        assert cmd["type"] == "python"

    def test_get_builtin_command_missing(self) -> None:
        """Get missing command returns None."""
        cmd = get_builtin_command("nonexistent")
        assert cmd is None

    def test_list_builtin_commands(self) -> None:
        """List returns sorted command names."""
        commands = list_builtin_commands()
        assert "audit" in commands
        assert "commit" in commands
        assert "docs" in commands
        assert "refactor" in commands
        assert "search" in commands
        assert "status" in commands
        assert "test" in commands
        assert "help" in commands
        assert "history" in commands
        assert "list" in commands
        assert commands == sorted(commands)

    def test_get_command_help(self) -> None:
        """Get help text for a command."""
        help_text = get_command_help("test")
        assert "Usage" in help_text
        assert "/test" in help_text

    def test_get_command_help_missing(self) -> None:
        """Get help for missing command returns unknown message."""
        help_text = get_command_help("nonexistent")
        assert "Unknown command" in help_text

    def test_all_commands_have_help(self) -> None:
        """All commands have help text."""
        for name in list_builtin_commands():
            help_text = get_command_help(name)
            assert "Usage" in help_text

    def test_all_commands_have_description(self) -> None:
        """All commands have descriptions."""
        for name, cmd in BUILTIN_COMMANDS.items():
            assert cmd["description"] is not None
            assert len(cmd["description"]) > 0

    def test_all_commands_have_action(self) -> None:
        """All commands have actions."""
        for name, cmd in BUILTIN_COMMANDS.items():
            assert cmd["action"] is not None
            assert len(cmd["action"]) > 0

    def test_all_commands_have_type(self) -> None:
        """All commands have types."""
        for name, cmd in BUILTIN_COMMANDS.items():
            assert cmd["type"] in ("shell", "sin", "python")
