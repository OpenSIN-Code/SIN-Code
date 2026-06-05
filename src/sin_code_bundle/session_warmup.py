"""Purpose: One-call session context primer.

Docs: session_warmup.doc.md

Returns a snapshot of the current repository: branch, git state, CoDocs
coverage, ceo-audit grade (cached), top risks, and a session-level
recommendation ("ready to code" vs "fix first"). Designed to be the first
call an agent makes at the start of a session.
"""

from __future__ import annotations

import json
import shutil
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

# Hard-coded fallback for the dev-machine layout (AGENTS.md).
_CEO_AUDIT_FALLBACK = "/Users/jeremy/.local/bin/sin"
_ROLLBACK_FALLBACK = "/Users/jeremy/Library/Python/3.14/bin/sin-honcho-rollback"


def _human_age(seconds: int) -> str:
    """Format a duration in seconds as a short human-readable string.

    Examples:
        >>> _human_age(45)
        '45s'
        >>> _human_age(3700)
        '1h 1m'
        >>> _human_age(90000)
        '1d 1h'
    """
    if seconds < 0:
        return "0s"
    if seconds < 60:
        return f"{seconds}s"
    minutes, sec = divmod(seconds, 60)
    if minutes < 60:
        return f"{minutes}m {sec}s"
    hours, m = divmod(minutes, 60)
    if hours < 24:
        return f"{hours}h {m}m"
    days, h = divmod(hours, 24)
    return f"{days}d {h}h"


class SessionWarmup:
    """Assemble session context in a single call.

    Runs 5 independent signals (git state, CoDocs coverage, ceo-audit grade,
    top-risk file scan, last commit time) and returns a structured summary
    with a single ``session_recommendation`` field — so the agent can decide
    "ready" vs "fix first" in one read.
    """

    def __init__(self, repo_root: Optional[Path] = None) -> None:
        self.repo_root = Path(repo_root) if repo_root else Path.cwd()

    def warmup(self) -> Dict[str, Any]:
        """Gather all session signals.

        Returns a dict with the following keys (always present, default
        values on failure): ``branch``, ``git_state``, ``git_changes_count``,
        ``last_commit_age``, ``codocs_coverage``, ``ceo_audit_grade``,
        ``top_risks``, ``session_recommendation``, ``signals``.
        """
        signals: Dict[str, Any] = {}

        # ── 1. Git state ─────────────────────────────────────────────
        signals["git"] = self._git_state()

        # ── 2. CoDocs coverage ───────────────────────────────────────
        signals["codocs"] = self._codocs_coverage()

        # ── 3. ceo-audit (best-effort, no cache here — caller's job) ─
        signals["ceo_audit"] = self._ceo_audit_quick()

        # ── 4. Top risks (file-level complexity heuristic) ───────────
        signals["top_risks"] = self._top_risks()

        # ── 5. Last commit time ──────────────────────────────────────
        signals["last_commit"] = self._last_commit_age()

        # ── Compose final summary ────────────────────────────────────
        branch = signals["git"].get("branch") or "unknown"
        git_clean = signals["git"].get("clean", False)
        docs_ok = signals["codocs"].get("ok", True)
        audit_grade = signals["ceo_audit"].get("grade") or "UNKNOWN"
        top_risks = signals["top_risks"]

        verdict = self._verdict(
            git_clean=git_clean,
            docs_ok=docs_ok,
            audit_grade=audit_grade,
            top_risks=top_risks,
        )

        return {
            "branch": branch,
            "git_state": "clean" if git_clean else "dirty",
            "git_changes_count": signals["git"].get("changes_count", 0),
            "last_commit_age": signals["last_commit"].get("age_human"),
            "codocs_coverage": {
                "ok": docs_ok,
                "broken": signals["codocs"].get("broken", 0),
                "checked": signals["codocs"].get("checked", 0),
            },
            "ceo_audit_grade": audit_grade,
            "ceo_audit_path": signals["ceo_audit"].get("report_path"),
            "top_risks": top_risks[:5],
            "session_recommendation": verdict,
            "signals": signals,
            "timestamp": datetime.now(timezone.utc).isoformat(),
        }

    # ── helpers ─────────────────────────────────────────────────────
    def _git_state(self) -> Dict[str, Any]:
        try:
            branch = (
                subprocess.run(
                    ["git", "branch", "--show-current"],
                    cwd=self.repo_root,
                    capture_output=True,
                    text=True,
                    timeout=5,
                ).stdout.strip()
                or "(detached)"
            )
            status = subprocess.run(
                ["git", "status", "--porcelain"],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=5,
            ).stdout.strip()
            changes = status.splitlines() if status else []
            return {
                "ok": True,
                "branch": branch,
                "clean": not bool(changes),
                "changes_count": len(changes),
            }
        except Exception as exc:
            return {"ok": False, "error": str(exc), "branch": None, "clean": False}

    def _codocs_coverage(self) -> Dict[str, Any]:
        try:
            from . import codocs

            broken = codocs.find_broken(str(self.repo_root))
            return {"ok": not bool(broken), "broken": len(broken), "checked": "auto"}
        except Exception as exc:
            return {"ok": True, "broken": 0, "error": str(exc)}

    def _ceo_audit_quick(self) -> Dict[str, Any]:
        try:
            sin_bin = shutil.which("sin") or _CEO_AUDIT_FALLBACK
            if not Path(sin_bin).exists():
                return {"ok": False, "grade": None, "error": "sin CLI not installed"}
            proc = subprocess.run(
                [sin_bin, "ceo-audit", "run", str(self.repo_root), "--profile=QUICK", "--json"],
                capture_output=True,
                text=True,
                timeout=180,  # 3min ceiling; QUICK profile is normally < 1min
            )
            if proc.returncode == 0 and proc.stdout.strip():
                data = json.loads(proc.stdout)
                return {
                    "ok": True,
                    "grade": data.get("grade"),
                    "report_path": data.get("report_path"),
                }
            return {"ok": False, "grade": None, "error": proc.stderr[-300:]}
        except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
            return {"ok": False, "grade": None, "error": str(exc)}
        except Exception as exc:
            return {"ok": False, "grade": None, "error": str(exc)}

    def _top_risks(self) -> List[Dict[str, Any]]:
        """Heuristic: largest 5 Python files by line count are 'top risks'.

        Cheap proxy for "where could go wrong". Returns up to 5 entries with
        ``path`` and ``lines``. No external tool required.
        """
        try:
            files: List[Dict[str, Any]] = []
            for p in self.repo_root.rglob("*.py"):
                rel = p.relative_to(self.repo_root)
                if any(
                    part.startswith(".") or part in {"__pycache__", "node_modules", "venv", ".venv"}
                    for part in rel.parts
                ):
                    continue
                try:
                    n = sum(1 for _ in p.open("r", encoding="utf-8", errors="ignore"))
                except Exception:
                    continue
                if n > 200:  # Only flag meaningfully large files
                    files.append({"path": str(rel), "lines": n})
            files.sort(key=lambda x: x["lines"], reverse=True)
            return files[:5]
        except Exception:
            return []

    def _last_commit_age(self) -> Dict[str, Any]:
        try:
            proc = subprocess.run(
                ["git", "log", "-1", "--format=%ct"],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=5,
            )
            if proc.returncode == 0 and proc.stdout.strip():
                ts = int(proc.stdout.strip())
                age_sec = int(datetime.now(timezone.utc).timestamp()) - ts
                return {"ok": True, "age_sec": age_sec, "age_human": _human_age(age_sec)}
            return {"ok": False, "age_human": "unknown"}
        except Exception as exc:
            return {"ok": False, "error": str(exc), "age_human": "unknown"}

    def _verdict(
        self,
        git_clean: bool,
        docs_ok: bool,
        audit_grade: str,
        top_risks: List[Dict[str, Any]],
    ) -> str:
        """Single-line recommendation for the agent.

        Logic, in order:
        1. ceo-audit F → "BLOCK — fix critical issues first"
        2. ceo-audit D or many broken docs → "FIX — improve before coding"
        3. Dirty tree with > 20 changes → "STASH or COMMIT first"
        4. Everything else → "READY — proceed with coding"
        """
        grade = (audit_grade or "UNKNOWN").upper()
        if grade == "F":
            return "BLOCK — ceo-audit grade F. Fix critical issues first."
        if grade == "D" or (not docs_ok and len(top_risks) > 0):
            return "FIX — improve docs/quality before coding"
        if not git_clean:
            return "STASH or COMMIT first — working tree dirty"
        return "READY — proceed with coding"
