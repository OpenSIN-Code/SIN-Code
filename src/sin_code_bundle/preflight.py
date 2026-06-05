# Purpose: Pre-flight safety gate — checks before state-changing tool calls.
# Docs: preflight.doc.md
"""Consolidates policy (sin_check_architecture) + docs (codocs) + git + tests
into 1 call. Run BEFORE sin_write, sin_edit, sin_bash, sin_ast_edit.

Docs: preflight.doc.md
"""

from __future__ import annotations

import json
import subprocess
from pathlib import Path
from typing import Any, Dict, Optional


class PreflightChecker:
    """Run all pre-flight checks in one go.

    Each check is independent — failure of one does not block the others.
    Returns a structured dict with per-check results and a derived risk score.
    """

    def __init__(self, repo_root: Optional[Path] = None) -> None:
        self.repo_root = Path(repo_root) if repo_root else Path.cwd()

    def check(self, tool_name: str, tool_input: Dict[str, Any]) -> Dict[str, Any]:
        """Run policy + docs + git + tests checks.

        Args:
            tool_name: tool about to be called (e.g. ``sin_write``).
            tool_input: arguments to that tool.

        Returns:
            Dict with ``allowed``, ``policy_ok``, ``docs_ok``, ``git_clean``,
            ``tests_status``, ``estimated_risk``, ``violations`` and ``details``.
        """
        result: Dict[str, Any] = {
            "tool_name": tool_name,
            "allowed": True,
            "policy_ok": True,
            "docs_ok": True,
            "git_clean": True,
            "tests_status": "unknown",
            "estimated_risk": "low",
            "violations": [],
            "details": {},
        }

        # ── 1. Policy check (existing SINInterceptor) ────────────────────
        # Reuses the same rule engine as sin_check_architecture, so behaviour
        # stays consistent with the single-call variant.
        try:
            from .interceptor import SINInterceptor

            policy = SINInterceptor(repo_root=self.repo_root).preflight(tool_name, tool_input)
            result["policy_ok"] = policy.get("allowed", True)
            result["violations"] = policy.get("violations", [])
            if not result["policy_ok"]:
                result["allowed"] = False
                result["estimated_risk"] = "high"
        except Exception as exc:
            result["details"]["policy_error"] = str(exc)

        # ── 2. Docs check (codocs) ───────────────────────────────────────
        # Surfaces broken .doc.md references; non-fatal but raises risk.
        try:
            from . import codocs

            broken = codocs.find_broken(str(self.repo_root))
            result["docs_ok"] = not bool(broken)
            if not result["docs_ok"]:
                result["details"]["broken_docs"] = [b.to_dict() for b in broken]
        except Exception as exc:
            result["details"]["docs_error"] = str(exc)

        # ── 3. Git status ────────────────────────────────────────────────
        # Skipped silently if the directory is not a git repository.
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
                        result["details"]["git_changes_count"] = len(changes.split("\n"))
                else:
                    result["git_clean"] = False
                    result["details"]["git_error"] = proc.stderr[-500:]
        except subprocess.TimeoutExpired:
            result["details"]["git_error"] = "git status timeout"
        except FileNotFoundError:
            result["details"]["git_error"] = "git not installed"
        except Exception as exc:
            result["details"]["git_error"] = str(exc)

        # ── 4. Test collection (pytest --collect-only) ───────────────────
        # Only runs when a tests/ or test/ directory exists AND pytest is
        # importable. Collection (not execution) keeps the pre-flight cheap.
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
                    # Capture the "N tests collected" summary line for context.
                    for line in proc.stdout.split("\n"):
                        if "tests collected" in line.lower():
                            result["details"]["tests_collected"] = line.strip()
                            break
                else:
                    result["tests_status"] = "fail"
                    result["details"]["test_errors"] = proc.stderr[-500:]
        except subprocess.TimeoutExpired:
            result["tests_status"] = "timeout"
        except FileNotFoundError:
            # pytest not installed — non-fatal, leave status as "unknown".
            result["tests_status"] = "skipped"
        except Exception as exc:
            result["details"]["tests_error"] = str(exc)

        # ── 5. Risk estimation ───────────────────────────────────────────
        # 5 independent signals; 0 → low, 1-2 → medium, 3+ → high + block.
        risk_signals = sum(
            [
                not result["policy_ok"],
                not result["docs_ok"],
                not result["git_clean"],
                result["tests_status"] == "fail",
                len(result["violations"]) > 0,
            ]
        )
        if risk_signals == 0:
            result["estimated_risk"] = "low"
        elif risk_signals <= 2:
            result["estimated_risk"] = "medium"
        else:
            result["estimated_risk"] = "high"
            result["allowed"] = False

        return result
