# SPDX-License-Identifier: MIT
# Purpose: Tests for the command registry.
# Docs: test_registry.doc.md
"""Test the SQLite-backed command registry.

Tests cover CRUD operations, persistence, and edge cases.
"""

import os
import tempfile

import pytest

from sin_code_bundle.tools.slash.registry import CommandRegistry, CustomCommand


class TestCommandRegistry:
    """Tests for CommandRegistry."""

    def setup_method(self) -> None:
        """Create a temporary registry for each test."""
        self.db_fd, self.db_path = tempfile.mkstemp(suffix=".db")
        self.registry = CommandRegistry(self.db_path)

    def teardown_method(self) -> None:
        """Clean up temporary database."""
        os.close(self.db_fd)
        os.unlink(self.db_path)

    def test_register_command(self) -> None:
        """Register a new command."""
        cmd = self.registry.register("deploy", "Deploy app", "git push", "shell")
        assert cmd.name == "deploy"
        assert cmd.description == "Deploy app"
        assert cmd.action == "git push"
        assert cmd.action_type == "shell"
        assert cmd.created_at is not None
        assert cmd.updated_at is not None

    def test_register_duplicate_raises(self) -> None:
        """Registering duplicate command raises ValueError."""
        self.registry.register("deploy", "Deploy app", "git push", "shell")
        with pytest.raises(ValueError, match="already exists"):
            self.registry.register("deploy", "Deploy app", "git push", "shell")

    def test_register_invalid_type_raises(self) -> None:
        """Registering with invalid action_type raises ValueError."""
        with pytest.raises(ValueError, match="Invalid action_type"):
            self.registry.register("deploy", "Deploy app", "git push", "invalid")

    def test_get_command(self) -> None:
        """Get a registered command."""
        self.registry.register("deploy", "Deploy app", "git push", "shell")
        cmd = self.registry.get("deploy")
        assert cmd is not None
        assert cmd.name == "deploy"
        assert cmd.description == "Deploy app"

    def test_get_missing_command(self) -> None:
        """Get returns None for missing command."""
        cmd = self.registry.get("nonexistent")
        assert cmd is None

    def test_unregister_command(self) -> None:
        """Unregister a command."""
        self.registry.register("deploy", "Deploy app", "git push", "shell")
        removed = self.registry.unregister("deploy")
        assert removed is True
        assert self.registry.get("deploy") is None

    def test_unregister_missing(self) -> None:
        """Unregister returns False for missing command."""
        removed = self.registry.unregister("nonexistent")
        assert removed is False

    def test_list_commands(self) -> None:
        """List all registered commands."""
        self.registry.register("a", "Command A", "cmd a", "shell")
        self.registry.register("b", "Command B", "cmd b", "sin")
        commands = self.registry.list()
        assert len(commands) == 2
        assert commands[0].name == "a"
        assert commands[1].name == "b"

    def test_list_empty(self) -> None:
        """List returns empty list when no commands."""
        commands = self.registry.list()
        assert commands == []

    def test_update_command(self) -> None:
        """Update an existing command."""
        self.registry.register("deploy", "Deploy app", "git push", "shell")
        updated = self.registry.update("deploy", description="Deploy to prod")
        assert updated is not None
        assert updated.description == "Deploy to prod"
        assert updated.action == "git push"  # unchanged

    def test_update_missing(self) -> None:
        """Update returns None for missing command."""
        updated = self.registry.update("nonexistent", description="Test")
        assert updated is None

    def test_update_invalid_type_raises(self) -> None:
        """Update with invalid type raises ValueError."""
        self.registry.register("deploy", "Deploy app", "git push", "shell")
        with pytest.raises(ValueError, match="Invalid action_type"):
            self.registry.update("deploy", action_type="invalid")

    def test_exists(self) -> None:
        """Check if command exists."""
        self.registry.register("deploy", "Deploy app", "git push", "shell")
        assert self.registry.exists("deploy") is True
        assert self.registry.exists("nonexistent") is False

    def test_clear(self) -> None:
        """Clear all commands."""
        self.registry.register("a", "A", "cmd a", "shell")
        self.registry.register("b", "B", "cmd b", "shell")
        count = self.registry.clear()
        assert count == 2
        assert self.registry.list() == []

    def test_clear_empty(self) -> None:
        """Clear on empty registry returns 0."""
        count = self.registry.clear()
        assert count == 0

    def test_to_dict(self) -> None:
        """Export to dict."""
        self.registry.register("a", "A", "cmd a", "shell")
        result = self.registry.to_dict()
        assert len(result) == 1
        assert result[0]["name"] == "a"
        assert result[0]["description"] == "A"

    def test_to_json(self) -> None:
        """Export to JSON."""
        self.registry.register("a", "A", "cmd a", "shell")
        json_str = self.registry.to_json()
        assert "a" in json_str
        assert "A" in json_str

    def test_persistence(self) -> None:
        """Registry persists across instances."""
        self.registry.register("deploy", "Deploy app", "git push", "shell")
        # Create new registry instance with same db
        new_registry = CommandRegistry(self.db_path)
        cmd = new_registry.get("deploy")
        assert cmd is not None
        assert cmd.name == "deploy"

    def test_default_db_path(self) -> None:
        """Default database path is in home directory."""
        registry = CommandRegistry()
        assert ".config/sin-slash/registry.db" in registry._db_path
