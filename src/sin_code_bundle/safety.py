# SPDX-License-Identifier: MIT
"""Hardened subprocess + input-sanitization helpers shared by all subsystems."""

from __future__ import annotations

import subprocess
from pathlib import Path
from typing import Optional, Sequence

DEFAULT_TIMEOUT = 600  # seconds — never run unbounded


class SafetyError(RuntimeError):
    """Raised when a safety invariant is violated (timeout, unsafe cmd shape, …)."""


def run_checked(
    cmd: Sequence[str],
    cwd: Optional[Path] = None,
    timeout: int = DEFAULT_TIMEOUT,
    allow_shell: bool = False,
) -> subprocess.CompletedProcess:
    """Run a subprocess with a mandatory timeout and no shell by default."""
    if not allow_shell and not isinstance(cmd, (list, tuple)):
        raise SafetyError("cmd must be a list/tuple unless allow_shell=True")
    try:
        return subprocess.run(
            cmd,
            cwd=str(cwd) if cwd else None,
            shell=allow_shell,
            timeout=timeout,
            check=False,
            capture_output=True,
            text=True,
        )
    except subprocess.TimeoutExpired as exc:
        raise SafetyError(f"command timed out after {timeout}s: {cmd}") from exc


def sanitize_prompt(
    text: str, max_len: int = 8000
) -> str:  # 8000 chars ≈ 2K tokens; fits LLM context without flooding
    """Neutralize obvious prompt-injection markers in untrusted task text."""
    if len(text) > max_len:
        text = text[:max_len] + "\n...[truncated]"
    safe_lines = []
    for line in text.splitlines():
        low = line.strip().lower()
        if low.startswith(("system:", "developer:", "ignore previous", "you are now")):
            safe_lines.append("[redacted suspicious instruction]")
        else:
            safe_lines.append(line)
    return "\n".join(safe_lines)
