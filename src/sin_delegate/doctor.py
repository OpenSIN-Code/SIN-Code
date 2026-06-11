# SPDX-License-Identifier: MIT
"""sin delegate doctor — preflight health check.

Checks deterministically ALL dependencies and preconditions before a run
starts. Prevents cryptic mid-run failures due to missing git config,
broken backend CLIs, or unreadable ledger.
"""

from __future__ import annotations

import shutil
import sqlite3
import subprocess
from dataclasses import dataclass
from pathlib import Path


@dataclass
class Check:
    name: str
    ok: bool
    detail: str
    level: str = "error"  # error | warning | info


def _run(cmd: list, timeout: int = 10) -> tuple:
    try:
        p = subprocess.run(cmd, capture_output=True, text=True,
                           timeout=timeout)
        return p.returncode, p.stdout + p.stderr
    except subprocess.TimeoutExpired:
        return -1, "timeout"
    except FileNotFoundError:
        return -2, "not found"


def check_git() -> Check:
    code, out = _run(["git", "--version"])
    if code != 0:
        return Check("git", False, "git not installed or broken")
    return Check("git", True, out.split("\n")[0])


def check_git_config() -> Check:
    for key in ("user.name", "user.email"):
        code, _ = _run(["git", "config", "--global", key])
        if code != 0:
            return Check(
                "git config", False,
                f"{key} not set — run: git config --global {key} "
                f"'Your Name'")
    return Check("git config", True, "user.name and user.email configured")


def check_repo(path: str) -> Check:
    p = Path(path)
    if not p.exists():
        return Check("repository", False, f"{path} does not exist")
    if not (p / ".git").exists():
        return Check("repository", False,
                     f"{path} is not a git repository")
    code, out = _run(["git", "-C", str(p), "status", "--porcelain"])
    if code != 0:
        return Check("repository", False, f"git status failed: {out}")
    dirty = [l for l in out.splitlines() if l.strip()]
    if dirty:
        return Check(
            "repository", False,
            f"working directory has uncommitted changes: "
            f"{len(dirty)} file(s)", level="warning")
    return Check("repository", True, "clean working directory")


def check_backend(backend: str, model: str = "") -> Check:
    if backend == "command":
        return Check(backend, True, "custom command (skipped)")
    hints = {
        "opencode": "install via: npm install -g @opencode/cli",
        "claude": "install via: npm install -g @anthropic-ai/claude-cli",
        "codex": "install via: pip install codex-cli",
    }
    if not shutil.which(backend):
        return Check(
            backend, False,
            f"{backend} CLI not found. {hints.get(backend, '')}")
    code, out = _run([backend, "--version"])
    if code not in (0, -1):
        return Check(backend, False, f"{backend} --version failed")
    version = out.split("\n")[0][:60] if out else "installed"
    return Check(backend, True, version)


def check_ledger(ledger_path: str = "~/.sin-code/delegate/ledger.db"
                 ) -> Check:
    p = Path(ledger_path).expanduser()
    if not p.exists():
        return Check("ledger", True,
                     "will be created on first run", level="info")
    if not p.is_file():
        return Check("ledger", False, f"{p} is not a file")
    try:
        db = sqlite3.connect(str(p), timeout=5)
        db.execute("SELECT 1 FROM sqlite_master LIMIT 1").fetchone()
        db.close()
        return Check("ledger", True, f"{p.stat().st_size // 1024} KB")
    except Exception as e:
        return Check("ledger", False, f"ledger corrupt: {e}")


def check_memory() -> Check:
    try:
        from sin_brain import __version__  # type: ignore
        return Check("sin-brain", True, f"v{__version__} (memory loop active)",
                     level="info")
    except ImportError:
        return Check(
            "sin-brain", True,
            "not installed (memory loop disabled, install with "
            "pip install 'sin-code-delegate[memory]')", level="warning")


def doctor(repo: str = ".", backends: list | None = None) -> list:
    checks = [
        check_git(),
        check_git_config(),
        check_repo(repo),
        check_ledger(),
        check_memory(),
    ]
    for backend in backends or ["opencode"]:
        checks.append(check_backend(backend))
    return checks


def print_report(checks: list) -> int:
    icons = {"error": "✗", "warning": "⚠", "info": "ℹ"}
    for c in checks:
        icon = "✓" if c.ok else icons.get(c.level, "✗")
        print(f"  {icon} {c.name:<18} {c.detail}")
    errors = [c for c in checks if not c.ok and c.level == "error"]
    if errors:
        print(f"\n{len(errors)} error(s) must be fixed before running")
        return 1
    warnings = [c for c in checks if not c.ok and c.level == "warning"]
    if warnings:
        print(f"\n{len(warnings)} warning(s) — consider fixing")
    return 0
