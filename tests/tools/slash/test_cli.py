# SPDX-License-Identifier: MIT
# Purpose: Tests for the CLI interface.
# Docs: test_cli.doc.md
"""Test the CLI interface.

Tests cover CLI commands using click.testing.CliRunner.
"""

import os
import tempfile

import pytest
from click.testing import CliRunner

from sin_code_bundle.tools.slash.cli import cli
from sin_code_bundle.tools.slash.registry import CommandRegistry


class TestCLI:
    """Tests for CLI commands."""

    def setup_method(self) -> None:
        """Set up CLI runner and temp files."""
        self.runner = CliRunner()
        self.db_fd, self.db_path = tempfile.mkstemp(suffix=".db")

    def teardown_method(self) -> None:
        """Clean up temp files."""
        os.close(self.db_fd)
        os.unlink(self.db_path)

    def test_cli_version(self) -> None:
        """CLI shows version."""
        result = self.runner.invoke(cli, ["--version"])
        assert result.exit_code == 0
        assert "sin-slash" in result.output

    def test_cli_help(self) -> None:
        """CLI shows help."""
        result = self.runner.invoke(cli, ["--help"])
        assert result.exit_code == 0
        assert "Slash command dispatch" in result.output

    def test_run_command(self) -> None:
        """Run a slash command via CLI."""
        result = self.runner.invoke(cli, ["run", "/test"])
        assert result.exit_code == 0
        assert "test" in result.output

    def test_run_command_raw(self) -> None:
        """Run a slash command with raw output via CLI."""
        result = self.runner.invoke(cli, ["run", "/test", "--raw"])
        assert result.exit_code == 0
        assert "test" in result.output

    def test_run_command_failure(self) -> None:
        """Run a failing command via CLI."""
        result = self.runner.invoke(cli, ["run", "/nonexistent"])
        assert result.exit_code == 1

    def test_list_commands(self) -> None:
        """List commands via CLI."""
        result = self.runner.invoke(cli, ["list"])
        assert result.exit_code == 0
        assert "/test" in result.output
        assert "/docs" in result.output

    def test_list_built_in(self) -> None:
        """List only built-in commands via CLI."""
        result = self.runner.invoke(cli, ["list", "--built-in"])
        assert result.exit_code == 0
        assert "/test" in result.output

    def test_list_custom(self) -> None:
        """List only custom commands via CLI."""
        result = self.runner.invoke(cli, ["list", "--custom"])
        assert result.exit_code == 0

    def test_register_command(self) -> None:
        """Register a command via CLI."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = os.path.join(tmpdir, "registry.db")
            registry = CommandRegistry(db_path)
            result = self.runner.invoke(
                cli,
                ["register", "deploy", "Deploy app", "git push", "--type", "shell"],
            )
            assert result.exit_code == 0
            assert "Registered" in result.output

    def test_register_duplicate(self) -> None:
        """Register duplicate command fails via CLI."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = os.path.join(tmpdir, "registry.db")
            registry = CommandRegistry(db_path)
            registry.register("deploy", "Deploy app", "git push", "shell")
            result = self.runner.invoke(
                cli,
                ["register", "deploy", "Deploy app", "git push", "--type", "shell"],
            )
            assert result.exit_code == 1

    def test_remove_command(self) -> None:
        """Remove a command via CLI."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = os.path.join(tmpdir, "registry.db")
            registry = CommandRegistry(db_path)
            registry.register("deploy", "Deploy app", "git push", "shell")
            result = self.runner.invoke(cli, ["remove", "deploy"])
            assert result.exit_code == 0
            assert "Removed" in result.output

    def test_remove_missing(self) -> None:
        """Remove missing command fails via CLI."""
        result = self.runner.invoke(cli, ["remove", "nonexistent"])
        assert result.exit_code == 1
        assert "not found" in result.output

    def test_history_command(self) -> None:
        """Show history via CLI."""
        # First run a command
        self.runner.invoke(cli, ["run", "/test"])
        result = self.runner.invoke(cli, ["history", "--limit", "10"])
        assert result.exit_code == 0
        assert "test" in result.output

    def test_help_command(self) -> None:
        """Show help via CLI."""
        result = self.runner.invoke(cli, ["help", "test"])
        assert result.exit_code == 0
        assert "Usage" in result.output

    def test_help_missing(self) -> None:
        """Show help for missing command fails via CLI."""
        result = self.runner.invoke(cli, ["help", "nonexistent"])
        assert result.exit_code == 1
        assert "Unknown" in result.output

    def test_run_with_args(self) -> None:
        """Run command with args via CLI."""
        # Register a simple command to test args
        self.runner.invoke(cli, ["register", "echo", "Echo", "echo", "--type", "shell"])
        result = self.runner.invoke(cli, ["run", "/echo hello world"])
        assert result.exit_code == 0
        assert "hello world" in result.output

    def test_run_with_flags(self) -> None:
        """Run command with flags via CLI."""
        self.runner.invoke(cli, ["register", "echo", "Echo", "echo", "--type", "shell"])
        result = self.runner.invoke(cli, ["run", "/echo hello --verbose"])
        assert result.exit_code == 0

    def test_list_empty_custom(self) -> None:
        """List custom when none registered."""
        result = self.runner.invoke(cli, ["list", "--custom"])
        assert result.exit_code == 0

    def test_history_empty(self) -> None:
        """Show empty history via CLI."""
        result = self.runner.invoke(cli, ["history"])
        assert result.exit_code == 0
