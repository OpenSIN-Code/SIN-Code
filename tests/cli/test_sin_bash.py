# SPDX-License-Identifier: MIT
"""Purpose: Tests for the `sin-bash` CLI shim.

Docs: test_sin_bash.doc.md

Run as module:
    python -m sin_code_bundle.cli_shims.sin_bash --command "echo hi"

Tests cover:
- Simple echo (returncode 0, stdout present)
- Secret redaction (sk-... → ***REDACTED***)
- Non-zero exit (returncode != 0 in JSON)
- --timeout flag (passed through)
- --command-from-file (read from disk)
- Missing --command / --command-from-file (CLI usage error)
"""
from __future__ import annotations

import json
import subprocess
import sys


def _run_cli(*args: str, input_text: str | None = None) -> subprocess.CompletedProcess:
    """Run `python -m sin_code_bundle.cli_shims.sin_bash` with the given args."""
    return subprocess.run(
        [sys.executable, "-m", "sin_code_bundle.cli_shims.sin_bash", *args],
        capture_output=True,
        text=True,
        input=input_text,
        timeout=120,
    )


def test_sin_bash_echo():
    """A simple `echo` command returns 0 with the expected stdout."""
    result = _run_cli("--command", "echo hello")
    assert result.returncode == 0, f"stderr: {result.stderr}"
    data = json.loads(result.stdout)
    # The outer JSON wraps the `execute` binary's structured output.
    # `redacted: true` means the `execute` Go binary is on PATH.
    assert "stdout" in data
    inner = json.loads(data["stdout"])
    assert inner["exit_code"] == 0
    assert "hello" in inner["stdout"]


def test_sin_bash_secret_redaction():
    """Secrets like `sk-...` are auto-redacted in the *executed* stdout.

    Note: the `execute` Go binary also echoes the full command in its
    structured output (for audit logging). The redaction applies to the
    *executed* stdout/stderr fields, not to the command echo field — and
    agents consume `inner["stdout"]`, not the command field.
    """
    result = _run_cli("--command", "echo sk-1234567890abcdef")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    inner = json.loads(data["stdout"])
    # The raw token MUST NOT appear in the executed stdout (this is what
    # the agent would see in its context).
    assert "sk-1234567890abcdef" not in inner["stdout"]
    # A redaction marker must replace it
    assert "REDACTED" in inner["stdout"]


def test_sin_bash_nonzero_exit():
    """A failing command propagates the non-zero exit code."""
    result = _run_cli("--command", "false")
    assert result.returncode == 0  # CLI itself succeeds; result is in JSON
    data = json.loads(result.stdout)
    inner = json.loads(data["stdout"])
    assert inner["exit_code"] != 0


def test_sin_bash_command_from_file(tmp_path):
    """`--command-from-file` reads the shell script from a file."""
    script = tmp_path / "script.sh"
    script.write_text("echo from_file")
    result = _run_cli("--command-from-file", str(script))
    assert result.returncode == 0
    data = json.loads(result.stdout)
    inner = json.loads(data["stdout"])
    assert "from_file" in inner["stdout"]


def test_sin_bash_command_from_stdin():
    """`--command-from-file -` reads the shell script from stdin."""
    result = _run_cli("--command-from-file", "-", input_text="echo from_stdin")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    inner = json.loads(data["stdout"])
    assert "from_stdin" in inner["stdout"]


def test_sin_bash_requires_command_flag():
    """Missing --command / --command-from-file is a CLI usage error."""
    result = _run_cli("--timeout", "5")
    assert result.returncode != 0
