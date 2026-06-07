# SPDX-License-Identifier: MIT
# Purpose: Tests for the command dispatcher.
# Docs: test_dispatcher.doc.md
"""Test the command dispatcher.

Tests cover dispatch, history, and command resolution.
"""

import os
import tempfile

import pytest

from sin_code_bundle.tools.slash.dispatcher import CommandDispatcher, DispatchResult
from sin_code_bundle.tools.slash.registry import CommandRegistry


class TestCommandDispatcher:
    """Tests for CommandDispatcher."""

    def setup_method(self) -> None:
        """Create temporary dispatcher for each test."""
        self.db_fd, self.db_path = tempfile.mkstemp(suffix=".db")
        self.history_fd, self.history_path = tempfile.mkstemp(suffix=".db")
        self.registry = CommandRegistry(self.db_path)
        self.dispatcher = CommandDispatcher(
            registry=self.registry,
            history_db=self.history_path,
        )

    def teardown_method(self) -> None:
        """Clean up temporary files."""
        os.close(self.db_fd)
        os.unlink(self.db_path)
        os.close(self.history_fd)
        os.unlink(self.history_path)

    def test_dispatch_builtin_test(self) -> None:
        """Dispatch a built-in /test command."""
        result = self.dispatcher.dispatch("/test")
        assert result.success is True
        assert result.command == "test"
        assert result.error is None

    def test_dispatch_builtin_docs(self) -> None:
        """Dispatch a built-in /docs command."""
        result = self.dispatcher.dispatch("/docs")
        assert result.success is True
        assert result.command == "docs"

    def test_dispatch_builtin_audit(self) -> None:
        """Dispatch a built-in /audit command."""
        result = self.dispatcher.dispatch("/audit")
        assert result.success is True
        assert result.command == "audit"

    def test_dispatch_builtin_status(self) -> None:
        """Dispatch a built-in /status command."""
        result = self.dispatcher.dispatch("/status")
        assert result.success is True
        assert result.command == "status"

    def test_dispatch_builtin_commit(self) -> None:
        """Dispatch a built-in /commit command."""
        result = self.dispatcher.dispatch("/commit test message")
        assert result.success is True
        assert result.command == "commit"

    def test_dispatch_builtin_search(self) -> None:
        """Dispatch a built-in /search command."""
        result = self.dispatcher.dispatch("/search query")
        assert result.success is True
        assert result.command == "search"

    def test_dispatch_builtin_refactor(self) -> None:
        """Dispatch a built-in /refactor command."""
        result = self.dispatcher.dispatch("/refactor my_symbol")
        assert result.success is True
        assert result.command == "refactor"

    def test_dispatch_custom_command(self) -> None:
        """Dispatch a custom command."""
        self.registry.register("deploy", "Deploy", "echo deployed", "shell")
        result = self.dispatcher.dispatch("/deploy")
        assert result.success is True
        assert result.command == "deploy"
        assert "deployed" in result.output

    def test_dispatch_unknown_command(self) -> None:
        """Dispatch unknown command returns error."""
        result = self.dispatcher.dispatch("/nonexistent")
        assert result.success is False
        assert result.command == "nonexistent"
        assert "Unknown command" in result.error

    def test_dispatch_invalid_syntax(self) -> None:
        """Dispatch invalid syntax returns error."""
        result = self.dispatcher.dispatch("not-a-command")
        assert result.success is False
        assert result.error is not None

    def test_dispatch_empty_command(self) -> None:
        """Dispatch empty command returns error."""
        result = self.dispatcher.dispatch("")
        assert result.success is False

    def test_history_recorded(self) -> None:
        """Dispatch records history."""
        self.dispatcher.dispatch("/test")
        history = self.dispatcher.get_history()
        assert len(history) == 1
        assert history[0]["command"] == "test"
        assert history[0]["success"] is True

    def test_history_limit(self) -> None:
        """History respects limit."""
        for _ in range(5):
            self.dispatcher.dispatch("/test")
        history = self.dispatcher.get_history(limit=3)
        assert len(history) == 3

    def test_history_order(self) -> None:
        """History is ordered newest first."""
        self.dispatcher.dispatch("/test")
        self.dispatcher.dispatch("/docs")
        history = self.dispatcher.get_history()
        assert history[0]["command"] == "docs"
        assert history[1]["command"] == "test"

    def test_clear_history(self) -> None:
        """Clear history removes all records."""
        self.dispatcher.dispatch("/test")
        count = self.dispatcher.clear_history()
        assert count == 1
        assert self.dispatcher.get_history() == []

    def test_list_commands(self) -> None:
        """List all available commands."""
        self.registry.register("custom", "Custom", "echo custom", "shell")
        commands = self.dispatcher.list_commands()
        assert "built_in" in commands
        assert "custom" in commands
        assert "custom" in commands["custom"]
        assert "test" in commands["built_in"]

    def test_get_command_help_builtin(self) -> None:
        """Get help for built-in command."""
        help_text = self.dispatcher.get_command_help("test")
        assert help_text is not None
        assert "/test" in help_text

    def test_get_command_help_custom(self) -> None:
        """Get help for custom command."""
        self.registry.register("custom", "Custom", "echo custom", "shell")
        help_text = self.dispatcher.get_command_help("custom")
        assert help_text is not None
        assert "custom" in help_text

    def test_get_command_help_missing(self) -> None:
        """Get help for missing command returns None."""
        help_text = self.dispatcher.get_command_help("nonexistent")
        assert help_text is None

    def test_dispatch_with_args(self) -> None:
        """Dispatch with arguments."""
        self.registry.register("echo", "Echo", "echo", "shell")
        result = self.dispatcher.dispatch("/echo hello world")
        assert result.success is True
        assert "hello world" in result.output

    def test_dispatch_with_flags(self) -> None:
        """Dispatch with flags."""
        self.registry.register("echo", "Echo", "echo", "shell")
        result = self.dispatcher.dispatch("/echo hello --verbose")
        assert result.success is True

    def test_result_has_timestamp(self) -> None:
        """Dispatch result includes timestamp."""
        result = self.dispatcher.dispatch("/test")
        assert result.timestamp is not None
        assert "T" in result.timestamp

    def test_result_has_duration(self) -> None:
        """Dispatch result includes duration."""
        result = self.dispatcher.dispatch("/test")
        assert result.duration_ms >= 0
