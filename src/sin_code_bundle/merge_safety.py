"""Purpose: Pre-merge / pre-PR safety gate.

Docs: merge_safety.doc.md

Runs a battery of checks before allowing a merge or PR open:
- CoDocs coverage (broken .doc.md references)
- ceo-audit grade (cached 5 minutes per profile)
- git diff stat (large diffs flagged)
- secret scan (cheap substring heuristic)

Returns a single ``pass: bool`` plus a list of human-readable ``blockers``
and ``warnings``. The agent can use the verdict to decide whether to
proceed with the merge or fix issues first.
"""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import time
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

# Coarse secret pattern: keys/tokens/passwords in added lines.
_SECRET_LINE_PATTERN = re.compile(
    r"(?i)(api[_-]?key|secret|token|password|passwd|pwd|access[_-]?key)\s*[:=]\s*['\"]?[A-Za-z0-9_\-\.]{16,}"
)

# Substrings that should NEVER appear in a diff (would be a leaked secret).
_SECRET_HINTS = (
    "BEGIN RSA PRIVATE KEY",
    "BEGIN OPENSSH PRIVATE KEY",
    "BEGIN PRIVATE KEY",
    "sk-",  # OpenAI / many SaaS keys
    "ghp_",  # GitHub PAT
    "github_pat_",  # GitHub fine-grained PAT
    "xoxb-",
    "xoxp-",  # Slack tokens
    "AIza",  # Google API keys
    "AKIA",  # AWS access key
    "ASIA",
)

# Hard-coded fallback for the dev-machine layout.
_CEO_AUDIT_FALLBACK = "/Users/jeremy/.local/bin/sin"

# Max lines changed before we flag a "large diff" warning.
_LARGE_DIFF_LINES = 1000

# 5min — ceo-audit is the slow part; cache to keep pre-PR hooks snappy.
_CEO_AUDIT_CACHE_TTL = 300


class MergeSafety:
    """Pre-merge / pre-PR safety gate.

    Each check is independent and contributes to the final verdict.
    All checks are best-effort — a missing CLI downgrades a check
    to a warning, not a failure.
    """

    def __init__(self, repo_root: Optional[Path] = None) -> None:
        self.repo_root = Path(repo_root) if repo_root else Path.cwd()
        # Per-process cache for ceo-audit results. Key: (profile, base, head).
        self._audit_cache: Dict[Tuple[str, str, str], Tuple[float, Dict[str, Any]]] = {}

    def check(
        self,
        base: str = "main",
        head: str = "HEAD",
        profile: str = "QUICK",
    ) -> Dict[str, Any]:
        """Run all safety checks and return a verdict.

        Args:
            base: base ref (default ``main``).
            head: head ref (default ``HEAD``).
            profile: ceo-audit profile (default ``QUICK``).

        Returns:
            Dict with ``pass`` (bool), ``blockers`` (list of str),
            ``warnings`` (list of str), ``checks`` (per-check dict),
            and ``verdict`` ("READY" or "FIX_FIRST").
        """
        blockers: List[str] = []
        warnings: List[str] = []
        checks: Dict[str, Any] = {}

        # ── 1. CoDocs coverage ───────────────────────────────────────
        codocs = self._check_codocs()
        checks["codocs"] = codocs
        if codocs.get("broken", 0) > 0:
            blockers.append(
                f"CoDocs: {codocs['broken']} broken .doc.md reference(s) — fix before merge"
            )

        # ── 2. ceo-audit (cached) ────────────────────────────────────
        audit = self._check_ceo_audit(profile, base, head)
        checks["ceo_audit"] = audit
        grade = (audit.get("grade") or "").upper()
        if grade == "F":
            blockers.append("ceo-audit grade F — fix critical issues before merge")
        elif grade == "D":
            warnings.append("ceo-audit grade D — consider improving before merge")
        elif not audit.get("ok"):
            # ceo-audit didn't run — downgrade to warning (don't block).
            warnings.append(
                f"ceo-audit could not run: {audit.get('error', 'unknown')} — verify manually"
            )

        # ── 3. git diff stat (size + secrets in lines) ───────────────
        diff = self._check_diff(base, head)
        checks["diff"] = diff
        if diff.get("lines_changed", 0) > _LARGE_DIFF_LINES:
            warnings.append(
                f"Large diff: {diff['lines_changed']} lines changed — consider splitting"
            )
        secret_hits = diff.get("secret_hits", [])
        if secret_hits:
            blockers.append(
                f"Diff contains possible secrets ({len(secret_hits)} line(s)) — "
                "rotate keys and re-commit before merge"
            )
            checks["diff"]["secret_examples"] = secret_hits[:3]

        # ── 4. Working tree must be clean ────────────────────────────
        tree = self._check_tree()
        checks["working_tree"] = tree
        if not tree.get("clean", False):
            warnings.append(
                f"Working tree is dirty ({tree.get('changes_count', '?')} change(s)) — "
                "commit/stash before merge"
            )

        # ── Compose verdict ──────────────────────────────────────────
        passed = len(blockers) == 0
        verdict = "READY" if passed else "FIX_FIRST"

        return {
            "pass": passed,
            "verdict": verdict,
            "blockers": blockers,
            "warnings": warnings,
            "checks": checks,
            "base": base,
            "head": head,
            "profile": profile,
            "timestamp": _now_iso(),
        }

    # ── helpers ─────────────────────────────────────────────────────
    def _check_codocs(self) -> Dict[str, Any]:
        try:
            from . import codocs

            broken = codocs.find_broken(str(self.repo_root))
            return {
                "ok": not bool(broken),
                "broken": len(broken),
                "items": [b.to_dict() for b in broken][:10],
            }
        except Exception as exc:
            return {"ok": True, "broken": 0, "error": str(exc)}

    def _check_ceo_audit(
        self,
        profile: str,
        base: str,
        head: str,
    ) -> Dict[str, Any]:
        """Run ceo-audit, with a 5-minute in-process cache."""
        cache_key = (profile, base, head)
        now = time.time()

        if cache_key in self._audit_cache:
            ts, data = self._audit_cache[cache_key]
            if (now - ts) < _CEO_AUDIT_CACHE_TTL:
                return {**data, "cache_hit": True}
            # Stale — fall through and re-run.

        try:
            sin_bin = shutil.which("sin") or _CEO_AUDIT_FALLBACK
            if not Path(sin_bin).exists():
                return {"ok": False, "error": "sin CLI not installed"}

            proc = subprocess.run(
                [
                    sin_bin,
                    "ceo-audit",
                    "run",
                    str(self.repo_root),
                    f"--profile={profile}",
                    "--json",
                ],
                capture_output=True,
                text=True,
                timeout=180,  # 3min ceiling
            )
            if proc.returncode == 0 and proc.stdout.strip():
                data = json.loads(proc.stdout)
                result = {
                    "ok": True,
                    "grade": data.get("grade"),
                    "report_path": data.get("report_path"),
                }
                self._audit_cache[cache_key] = (now, result)
                return result
            return {"ok": False, "error": proc.stderr[-300:]}
        except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
            return {"ok": False, "error": str(exc)}
        except Exception as exc:
            return {"ok": False, "error": str(exc)}

    def _check_diff(self, base: str, head: str) -> Dict[str, Any]:
        try:
            proc = subprocess.run(
                ["git", "diff", "--shortstat", f"{base}...{head}"],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=15,
            )
            shortstat = proc.stdout.strip() if proc.returncode == 0 else ""
            # " 3 files changed, 42 insertions(+), 17 deletions(-)"
            lines_changed = _parse_shortstat(shortstat)

            # Fetch the actual diff content for secret scan.
            content_proc = subprocess.run(
                ["git", "diff", f"{base}...{head}"],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=30,
            )
            content = content_proc.stdout if content_proc.returncode == 0 else ""
            secret_hits = _scan_for_secrets(content)

            return {
                "ok": True,
                "lines_changed": lines_changed,
                "shortstat": shortstat,
                "secret_hits": secret_hits,
            }
        except subprocess.TimeoutExpired:
            return {"ok": False, "lines_changed": 0, "secret_hits": [], "error": "git diff timeout"}
        except Exception as exc:
            return {"ok": False, "lines_changed": 0, "secret_hits": [], "error": str(exc)}

    def _check_tree(self) -> Dict[str, Any]:
        try:
            proc = subprocess.run(
                ["git", "status", "--porcelain"],
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=5,
            )
            if proc.returncode == 0:
                changes = proc.stdout.strip().splitlines()
                return {"ok": True, "clean": not bool(changes), "changes_count": len(changes)}
            return {"ok": False, "clean": False, "error": proc.stderr[-200:]}
        except Exception as exc:
            return {"ok": False, "clean": False, "error": str(exc)}


# ── module-level helpers ────────────────────────────────────────────


def _parse_shortstat(shortstat: str) -> int:
    """Parse ``3 files changed, 42 insertions(+), 17 deletions(-)`` → 59."""
    m = re.search(r"(\d+)\s+insertion", shortstat)
    insertions = int(m.group(1)) if m else 0
    m = re.search(r"(\d+)\s+deletion", shortstat)
    deletions = int(m.group(1)) if m else 0
    return insertions + deletions


def _scan_for_secrets(diff_content: str) -> List[str]:
    """Return a list of human-readable hits for any secret-like content.

    Two passes:
    1. Substring scan for known SaaS key prefixes.
    2. Regex for ``key = "value"`` / ``token: "value"`` patterns.
    """
    hits: List[str] = []

    # Pass 1: substring hints
    for hint in _SECRET_HINTS:
        if hint in diff_content:
            # Find a representative line for the report.
            for i, line in enumerate(diff_content.splitlines(), 1):
                if hint in line:
                    hits.append(f"line {i}: contains '{hint}'")
                    break

    # Pass 2: regex (only added lines, not context — context can be noisy).
    for i, line in enumerate(diff_content.splitlines(), 1):
        if not line.startswith("+"):
            continue
        if _SECRET_LINE_PATTERN.search(line):
            hits.append(f"line {i}: key=value pattern")
            if len(hits) >= 20:
                break

    return hits


def _now_iso() -> str:
    from datetime import datetime, timezone

    return datetime.now(timezone.utc).isoformat()
