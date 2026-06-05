# Purpose: Pre-refactor checkpoint — snapshot + state report in 1 call.
# Docs: checkpoint.doc.md
"""Consolidates rollback_snapshot + codocs_check + git status + sin_search
+ pytest collection. Idempotent — safe to call twice with the same name
(returns existing snapshot id).

Docs: checkpoint.doc.md
"""

from __future__ import annotations

import json
import shutil
import subprocess
from pathlib import Path
from typing import Any, Dict, List, Optional

# Hard-coded fallback for the dev-machine layout from AGENTS.md so the
# MCP stdio process (which may have a stripped PATH) can still find the
# rollback CLI.
_ROLLBACK_FALLBACK = "/Users/jeremy/Library/Python/3.14/bin/sin-honcho-rollback"
_SCOUT_FALLBACK = "/Users/jeremy/.local/bin/scout"


class Checkpointer:
    """Pre-refactor checkpoint orchestrator.

    Creates a recoverable state AND reports on the current state of the
    working tree. Idempotent on ``name`` — calling twice with the same name
    does not create a duplicate snapshot.
    """

    def __init__(
        self,
        repo_root: Optional[Path] = None,
        db_path: str = ".sin/rollback.db",
    ) -> None:
        self.repo_root = Path(repo_root) if repo_root else Path.cwd()
        self.db_path = db_path

    def create(
        self,
        name: str,
        include: Optional[List[str]] = None,
        description: str = "",
    ) -> Dict[str, Any]:
        """Create checkpoint. Idempotent on ``name``.

        Args:
            name: snapshot name (e.g. ``"before-auth-refactor"``).
            include: subset of {snapshot, docs, git, usages, tests}.
                Defaults to all five.
            description: optional human-readable description.

        Returns:
            Dict with ``snapshot_id``, per-check counts, and per-check
            error fields when something fails. Always returns (never
            raises) so the caller can safely merge the result into a
            larger state report.
        """
        if include is None:
            include = ["snapshot", "docs", "git", "usages", "tests"]

        result: Dict[str, Any] = {
            "checkpoint_name": name,
            "include": include,
            "snapshot_id": None,
            "docs_broken": 0,
            "git_clean": True,
            "git_changes_count": 0,
            "usages_found": 0,
            "tests_status": "unknown",
            "tests_collected": None,
        }

        # ── 1. Snapshot (sin-honcho-rollback) ───────────────────────────
        # Skipped silently when the CLI is not installed (e.g. minimal
        # install) — the state report is still useful without a snapshot.
        if "snapshot" in include:
            try:
                rb_bin = shutil.which("sin-honcho-rollback") or _ROLLBACK_FALLBACK
                if Path(rb_bin).exists():
                    proc = subprocess.run(
                        [
                            rb_bin,
                            "snapshot",
                            name,
                            "--description",
                            description or f"Pre-change checkpoint: {name}",
                            "--db",
                            str(self.repo_root / self.db_path),
                        ],
                        capture_output=True,
                        text=True,
                        timeout=10,
                    )
                    if proc.returncode == 0 and proc.stdout.strip():
                        data = json.loads(proc.stdout)
                        # The CLI nests the id under "snapshot.id" — fall
                        # back to top-level "id" for older schemas.
                        result["snapshot_id"] = data.get("snapshot", {}).get("id") or data.get("id")
                    else:
                        result["snapshot_error"] = proc.stderr[-500:]
            except (subprocess.TimeoutExpired, json.JSONDecodeError, Exception) as exc:
                result["snapshot_error"] = str(exc)

        # ── 2. Docs (codocs.find_broken) ────────────────────────────────
        if "docs" in include:
            try:
                from . import codocs

                broken = codocs.find_broken(str(self.repo_root))
                result["docs_broken"] = len(broken)
            except Exception as exc:
                result["docs_error"] = str(exc)

        # ── 3. Git status ───────────────────────────────────────────────
        if "git" in include:
            try:
                if (self.repo_root / ".git").exists():
                    proc = subprocess.run(
                        ["git", "status", "--porcelain"],
                        cwd=self.repo_root,
                        capture_output=True,
                        text=True,
                        timeout=5,
                    )
                    if proc.returncode == 0:
                        changes = proc.stdout.strip()
                        result["git_clean"] = not bool(changes)
                        if changes:
                            result["git_changes_count"] = len(changes.split("\n"))
            except (subprocess.TimeoutExpired, FileNotFoundError, Exception) as exc:
                result["git_error"] = str(exc)

        # ── 4. Usages (scout, with grep fallback) ───────────────────────
        # scout gives better ranking + cross-source context; grep is the
        # always-available fallback that still answers "where is X used?".
        if "usages" in include:
            result["usages_found"] = self._count_usages(name)

        # ── 5. Tests (pytest --collect-only) ────────────────────────────
        if "tests" in include:
            try:
                has_tests = (self.repo_root / "tests").exists() or (
                    self.repo_root / "test"
                ).exists()
                if has_tests:
                    proc = subprocess.run(
                        ["python3", "-m", "pytest", "--collect-only", "-q"],
                        cwd=self.repo_root,
                        capture_output=True,
                        text=True,
                        timeout=15,
                    )
                    if proc.returncode == 0:
                        result["tests_status"] = "pass"
                        for line in proc.stdout.split("\n"):
                            if "tests collected" in line.lower():
                                result["tests_collected"] = line.strip()
                                break
                    else:
                        result["tests_status"] = "fail"
            except subprocess.TimeoutExpired:
                result["tests_status"] = "timeout"
            except FileNotFoundError:
                result["tests_status"] = "skipped"
            except Exception as exc:
                result["tests_error"] = str(exc)

        return result

    def _count_usages(self, name: str) -> int:
        """Best-effort usage count for ``name``.

        Tries ``scout --type usage`` first (better ranking), then falls back
        to ``grep -r -l -w`` for the always-available case. Returns 0 when
        neither tool is available so the caller can still reason about the
        result.
        """
        try:
            scout_bin = shutil.which("scout") or _SCOUT_FALLBACK
            if Path(scout_bin).exists():
                proc = subprocess.run(
                    [
                        scout_bin,
                        "--query",
                        name,
                        "--type",
                        "usage",
                        "--path",
                        str(self.repo_root),
                        "--format",
                        "json",
                    ],
                    capture_output=True,
                    text=True,
                    timeout=10,
                )
                if proc.returncode == 0 and proc.stdout.strip():
                    data = json.loads(proc.stdout)
                    return len(data.get("results", []))
        except (subprocess.TimeoutExpired, json.JSONDecodeError, Exception):
            pass

        # Fallback: grep -r -l -w (counts files, not occurrences).
        try:
            proc = subprocess.run(
                [
                    "grep",
                    "-r",
                    "-l",
                    "-w",
                    name,
                    str(self.repo_root),
                    "--include=*.py",
                    "--include=*.ts",
                    "--include=*.js",
                    "--include=*.go",
                ],
                capture_output=True,
                text=True,
                timeout=10,
            )
            if proc.returncode == 0 and proc.stdout.strip():
                return len(proc.stdout.strip().split("\n"))
        except (subprocess.TimeoutExpired, FileNotFoundError, Exception):
            pass

        return 0
