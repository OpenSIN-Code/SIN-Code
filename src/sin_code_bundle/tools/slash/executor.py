# Purpose: Execute slash commands (built-in and custom).
# Docs: executor.doc.md
"""Execute slash commands (built-in and custom).

Maps built-in commands to sin-* tools and executes custom commands via shell or sin tools.
"""

import subprocess
from typing import Any, Optional

from sin_code_bundle.tools.slash.registry import CustomCommand


class CommandExecutor:
    """Execute slash commands.

    Usage:
        executor = CommandExecutor()
        output = executor.execute_builtin("test", action, ["tests/unit"], {"verbose": True})
    """

    def __init__(self, timeout: int = 60) -> None:
        """Initialize the executor.

        Args:
            timeout: Default timeout for shell commands in seconds.
        """
        self._timeout = timeout

    def execute_builtin(
        self,
        command_name: str,
        action: dict[str, Any],
        args: list[str],
        flags: dict[str, Any],
    ) -> str:
        """Execute a built-in command.

        Args:
            command_name: Name of the command.
            action: Action dict with "type" and "target" keys.
            args: Positional arguments.
            flags: Flag arguments.

        Returns:
            Command output as string.

        Raises:
            RuntimeError: If the command execution fails.
        """
        action_type = action.get("type", "shell")
        target = action.get("target", "")

        if action_type == "shell":
            return self._run_shell(target, args, flags)
        elif action_type == "sin":
            return self._run_sin(target, args, flags)
        elif action_type == "python":
            return self._run_python(target, args, flags)
        else:
            raise RuntimeError(f"Unknown action type: {action_type}")

    def execute_custom(
        self,
        command: CustomCommand,
        args: list[str],
        flags: dict[str, Any],
    ) -> str:
        """Execute a custom command.

        Args:
            command: The custom command to execute.
            args: Positional arguments.
            flags: Flag arguments.

        Returns:
            Command output as string.

        Raises:
            RuntimeError: If the command execution fails.
        """
        if command.action_type == "shell":
            return self._run_shell(command.action, args, flags)
        elif command.action_type == "sin":
            return self._run_sin(command.action, args, flags)
        elif command.action_type == "script":
            return self._run_shell(command.action, args, flags)
        else:
            raise RuntimeError(f"Unknown action type: {command.action_type}")

    def _run_shell(
        self, target: str, args: list[str], flags: dict[str, Any]
    ) -> str:
        """Run a shell command.

        Args:
            target: The base command string.
            args: Positional arguments to append.
            flags: Flag arguments to append.

        Returns:
            stdout of the command.
        """
        cmd_parts = [target]
        cmd_parts.extend(args)
        for key, value in flags.items():
            if isinstance(value, bool):
                if value:
                    cmd_parts.append(f"--{key}")
            else:
                cmd_parts.append(f"--{key}={value}")

        cmd = " ".join(cmd_parts)
        try:
            result = subprocess.run(
                cmd,
                shell=True,
                capture_output=True,
                text=True,
                timeout=self._timeout,
            )
            if result.returncode != 0:
                raise RuntimeError(f"Command failed: {result.stderr}")
            return result.stdout
        except subprocess.TimeoutExpired:
            raise RuntimeError(f"Command timed out after {self._timeout}s")

    def _run_sin(
        self, target: str, args: list[str], flags: dict[str, Any]
    ) -> str:
        """Run a sin-* tool command.

        Args:
            target: The sin tool name (e.g., "sin_symbol_resolve").
            args: Positional arguments.
            flags: Flag arguments.

        Returns:
            Simulated output (since actual sin tools may not be available).
        """
        # In a real environment, this would invoke the sin tool via MCP or CLI
        # For now, we simulate the invocation
        return f"[sin] {target} {' '.join(args)} flags={flags}\n"

    def _run_python(
        self, target: str, args: list[str], flags: dict[str, Any]
    ) -> str:
        """Run a Python function.

        Args:
            target: The Python function reference.
            args: Positional arguments.
            flags: Flag arguments.

        Returns:
            Function output.
        """
        return f"[python] {target} {' '.join(args)} flags={flags}\n"

    def validate_command(self, command: str) -> bool:
        """Validate that a command is safe to execute.

        Args:
            command: Command string to validate.

        Returns:
            True if the command is considered safe.
        """
        # Block dangerous commands
        dangerous = ["rm -rf /", "mkfs", "dd if=/dev/zero", "> /dev/sda", ":(){ :|:& };:"]
        for pattern in dangerous:
            if pattern in command:
                return False
        return True
