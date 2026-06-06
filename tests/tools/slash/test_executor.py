# Purpose: Tests for the command executor.
# Docs: test_executor.doc.md
"""Test the command executor.

Tests cover built-in execution, custom execution, shell commands, and validation.
"""

import pytest

from sin_code_bundle.tools.slash.executor import CommandExecutor
from sin_code_bundle.tools.slash.registry import CustomCommand


class TestCommandExecutor:
    """Tests for CommandExecutor."""

    def setup_method(self) -> None:
        """Set up executor instance."""
        self.executor = CommandExecutor()

    def test_execute_builtin_shell(self) -> None:
        """Execute a shell-type built-in command."""
        action = {"type": "shell", "target": "echo hello"}
        result = self.executor.execute_builtin("test", action, [], {})
        assert "hello" in result

    def test_execute_builtin_shell_with_args(self) -> None:
        """Execute a shell command with arguments."""
        action = {"type": "shell", "target": "echo"}
        result = self.executor.execute_builtin("test", action, ["hello"], {})
        assert "hello" in result

    def test_execute_builtin_sin(self) -> None:
        """Execute a sin-type command."""
        action = {"type": "sin", "target": "sin_test"}
        result = self.executor.execute_builtin("test", action, ["arg1"], {})
        assert "[sin]" in result
        assert "sin_test" in result
        assert "arg1" in result

    def test_execute_builtin_python(self) -> None:
        """Execute a python-type command."""
        action = {"type": "python", "target": "print"}
        result = self.executor.execute_builtin("test", action, ["arg1"], {})
        assert "[python]" in result
        assert "print" in result

    def test_execute_builtin_unknown_type(self) -> None:
        """Unknown action type raises RuntimeError."""
        action = {"type": "unknown", "target": "test"}
        with pytest.raises(RuntimeError, match="Unknown action type"):
            self.executor.execute_builtin("test", action, [], {})

    def test_execute_custom_shell(self) -> None:
        """Execute a custom shell command."""
        cmd = CustomCommand(
            name="deploy",
            description="Deploy",
            action="echo deployed",
            action_type="shell",
            created_at="2024-01-01T00:00:00",
            updated_at="2024-01-01T00:00:00",
        )
        result = self.executor.execute_custom(cmd, [], {})
        assert "deployed" in result

    def test_execute_custom_sin(self) -> None:
        """Execute a custom sin command."""
        cmd = CustomCommand(
            name="check",
            description="Check",
            action="sin_status",
            action_type="sin",
            created_at="2024-01-01T00:00:00",
            updated_at="2024-01-01T00:00:00",
        )
        result = self.executor.execute_custom(cmd, [], {})
        assert "[sin]" in result
        assert "sin_status" in result

    def test_execute_custom_script(self) -> None:
        """Execute a custom script command."""
        cmd = CustomCommand(
            name="run",
            description="Run",
            action="echo run",
            action_type="script",
            created_at="2024-01-01T00:00:00",
            updated_at="2024-01-01T00:00:00",
        )
        result = self.executor.execute_custom(cmd, [], {})
        assert "run" in result

    def test_execute_custom_unknown_type(self) -> None:
        """Unknown custom action type raises RuntimeError."""
        cmd = CustomCommand(
            name="bad",
            description="Bad",
            action="test",
            action_type="unknown",
            created_at="2024-01-01T00:00:00",
            updated_at="2024-01-01T00:00:00",
        )
        with pytest.raises(RuntimeError, match="Unknown action type"):
            self.executor.execute_custom(cmd, [], {})

    def test_validate_command_safe(self) -> None:
        """Safe commands are valid."""
        assert self.executor.validate_command("echo hello") is True
        assert self.executor.validate_command("ls -la") is True

    def test_validate_command_dangerous(self) -> None:
        """Dangerous commands are rejected."""
        assert self.executor.validate_command("rm -rf /") is False
        assert self.executor.validate_command("dd if=/dev/zero of=/dev/sda") is False

    def test_shell_command_failure(self) -> None:
        """Failed shell command raises RuntimeError."""
        action = {"type": "shell", "target": "false"}  # `false` exits with 1
        with pytest.raises(RuntimeError, match="Command failed"):
            self.executor.execute_builtin("test", action, [], {})

    def test_shell_command_timeout(self) -> None:
        """Shell command timeout raises RuntimeError."""
        executor = CommandExecutor(timeout=1)
        action = {"type": "shell", "target": "sleep 10"}
        with pytest.raises(RuntimeError, match="timed out"):
            executor.execute_builtin("test", action, [], {})

    def test_shell_command_with_flags(self) -> None:
        """Shell command with flags."""
        action = {"type": "shell", "target": "echo"}
        result = self.executor.execute_builtin("test", action, [], {"verbose": True})
        assert "--verbose" in result

    def test_shell_command_with_flag_values(self) -> None:
        """Shell command with flag values."""
        action = {"type": "shell", "target": "echo"}
        result = self.executor.execute_builtin("test", action, [], {"count": 5})
        assert "--count=5" in result

    def test_shell_command_with_args_and_flags(self) -> None:
        """Shell command with both args and flags."""
        action = {"type": "shell", "target": "echo"}
        result = self.executor.execute_builtin("test", action, ["hello"], {"verbose": True})
        assert "hello" in result
        assert "--verbose" in result

    def test_empty_shell_command(self) -> None:
        """Empty shell command output."""
        action = {"type": "shell", "target": "true"}
        result = self.executor.execute_builtin("test", action, [], {})
        assert result == ""
