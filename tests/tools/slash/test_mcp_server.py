# Purpose: Tests for the MCP server.
# Docs: test_mcp_server.doc.md
"""Test the MCP server tools.

Tests cover MCP tool registration and invocation via direct calls.
"""

import json
import os
import tempfile

import pytest

from sin_code_bundle.tools.slash.mcp_server import (
    slash_dispatch,
    slash_list,
    slash_register,
    slash_unregister,
    slash_help,
    slash_history,
    _get_dispatcher,
)
from sin_code_bundle.tools.slash.dispatcher import CommandDispatcher


class TestMCPServer:
    """Tests for MCP server tools."""

    def setup_method(self) -> None:
        """Set up temporary dispatcher for each test."""
        self.db_fd, self.db_path = tempfile.mkstemp(suffix=".db")
        self.history_fd, self.history_path = tempfile.mkstemp(suffix=".db")
        # Reset dispatcher singleton
        import sin_code_bundle.tools.slash.mcp_server as mcp_module
        from sin_code_bundle.tools.slash.registry import CommandRegistry
        registry = CommandRegistry(self.db_path)
        mcp_module._dispatcher = CommandDispatcher(
            registry=registry,
            history_db=self.history_path,
        )

    def teardown_method(self) -> None:
        """Clean up temporary files."""
        os.close(self.db_fd)
        os.unlink(self.db_path)
        os.close(self.history_fd)
        os.unlink(self.history_path)

    def test_slash_dispatch_builtin(self) -> None:
        """Dispatch a built-in command via MCP."""
        result = slash_dispatch("/test")
        data = json.loads(result)
        assert data["success"] is True
        assert data["command"] == "test"

    def test_slash_dispatch_unknown(self) -> None:
        """Dispatch unknown command via MCP."""
        result = slash_dispatch("/nonexistent")
        data = json.loads(result)
        assert data["success"] is False
        assert "Unknown command" in data["error"]

    def test_slash_dispatch_invalid(self) -> None:
        """Dispatch invalid command via MCP."""
        result = slash_dispatch("not-a-command")
        data = json.loads(result)
        assert data["success"] is False

    def test_slash_list_all(self) -> None:
        """List all commands via MCP."""
        result = slash_list()
        data = json.loads(result)
        assert "built_in" in data
        assert "custom" in data

    def test_slash_list_built_in_only(self) -> None:
        """List only built-in commands via MCP."""
        result = slash_list(built_in=True, custom=False)
        data = json.loads(result)
        assert "built_in" in data
        assert "custom" not in data

    def test_slash_list_custom_only(self) -> None:
        """List only custom commands via MCP."""
        result = slash_list(built_in=False, custom=True)
        data = json.loads(result)
        assert "built_in" not in data
        assert "custom" in data

    def test_slash_register(self) -> None:
        """Register a command via MCP."""
        result = slash_register("deploy", "Deploy app", "git push", "shell")
        data = json.loads(result)
        assert data["success"] is True
        assert data["command"]["name"] == "deploy"

    def test_slash_register_duplicate(self) -> None:
        """Register duplicate command fails via MCP."""
        slash_register("deploy", "Deploy app", "git push", "shell")
        result = slash_register("deploy", "Deploy app", "git push", "shell")
        data = json.loads(result)
        assert data["success"] is False
        assert "already exists" in data["error"]

    def test_slash_unregister(self) -> None:
        """Unregister a command via MCP."""
        slash_register("deploy", "Deploy app", "git push", "shell")
        result = slash_unregister("deploy")
        data = json.loads(result)
        assert data["success"] is True
        assert "removed" in data["message"]

    def test_slash_unregister_missing(self) -> None:
        """Unregister missing command via MCP."""
        result = slash_unregister("nonexistent")
        data = json.loads(result)
        assert data["success"] is False
        assert "not found" in data["message"]

    def test_slash_help_builtin(self) -> None:
        """Get help for built-in command via MCP."""
        result = slash_help("test")
        assert "Usage" in result
        assert "/test" in result

    def test_slash_help_custom(self) -> None:
        """Get help for custom command via MCP."""
        slash_register("deploy", "Deploy app", "git push", "shell")
        result = slash_help("deploy")
        assert "Deploy app" in result

    def test_slash_help_missing(self) -> None:
        """Get help for missing command via MCP."""
        result = slash_help("nonexistent")
        assert "Unknown command" in result

    def test_slash_history_empty(self) -> None:
        """Get empty history via MCP."""
        result = slash_history()
        data = json.loads(result)
        assert data == []

    def test_slash_history_with_records(self) -> None:
        """Get history with records via MCP."""
        slash_dispatch("/test")
        result = slash_history()
        data = json.loads(result)
        assert len(data) == 1
        assert data[0]["command"] == "test"

    def test_slash_history_limit(self) -> None:
        """Get history with limit via MCP."""
        slash_dispatch("/test")
        slash_dispatch("/docs")
        result = slash_history(limit=1)
        data = json.loads(result)
        assert len(data) == 1

    def test_slash_register_invalid_type(self) -> None:
        """Register with invalid type fails via MCP."""
        result = slash_register("bad", "Bad", "test", "invalid")
        data = json.loads(result)
        assert data["success"] is False

    def test_slash_dispatch_with_args(self) -> None:
        """Dispatch with args via MCP."""
        slash_register("echo", "Echo", "echo", "shell")
        result = slash_dispatch("/echo hello world")
        data = json.loads(result)
        assert data["success"] is True
        assert "hello world" in data["output"]

    def test_slash_dispatch_with_flags(self) -> None:
        """Dispatch with flags via MCP."""
        slash_register("echo", "Echo", "echo", "shell")
        result = slash_dispatch("/echo hello --verbose")
        data = json.loads(result)
        assert data["success"] is True

    def test_slash_dispatch_output(self) -> None:
        """Dispatch result contains output."""
        result = slash_dispatch("/test")
        data = json.loads(result)
        assert "output" in data
        assert data["output"] is not None

    def test_slash_dispatch_duration(self) -> None:
        """Dispatch result contains duration."""
        result = slash_dispatch("/test")
        data = json.loads(result)
        assert "duration_ms" in data
        assert data["duration_ms"] >= 0

    def test_slash_dispatch_timestamp(self) -> None:
        """Dispatch result contains timestamp."""
        result = slash_dispatch("/test")
        data = json.loads(result)
        assert "timestamp" in data
        assert "T" in data["timestamp"]
