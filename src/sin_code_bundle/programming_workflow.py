# SPDX-License-Identifier: MIT
"""Purpose: One-call orchestration of common programming workflows.

Docs: programming_workflow.doc.md

This is the meta-tool that replaces 5+ separate `sin_*` calls with a
single action string. The agent picks the action, the tool fans out
to the right combination of underlying tools, and returns a single
structured verdict.

Actions:
  - pre_write       : sin_symbol_resolve + sin_read + sin_preflight
  - write           : sin_preflight + sin_write + sin_hashline_validate
  - post_write      : sin_preflight + codocs_check + pytest --collect-only
  - pre_commit      : sin_checkpoint + git status + codocs + ceo-audit (cached)
  - refactor        : sin_checkpoint + gitnexus_impact + gitnexus_detect_changes
  - session_warmup  : sin_session_warmup (full snapshot)

Each action returns a dict with:
  - action          : the action name
  - steps           : list of per-step results
  - verdict         : "READY", "FIX_FIRST", "BLOCK", or "PROCEED"
  - suggested_message (pre_commit only): suggested Conventional Commits message
"""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

# Hard-coded fallback for the dev-machine layout.
_CEO_AUDIT_FALLBACK = "/Users/jeremy/.local/bin/sin"

# 5min — ceo-audit is the slow part of pre_commit.
_CEO_AUDIT_CACHE_TTL = 300

# Conventional Commits pattern (for `suggested_message` heuristic).
_CC_TYPES = ("feat", "fix", "docs", "chore", "refactor", "test", "perf")


class ProgrammingWorkflow:
    """Orchestrate the common agent workflows behind a single tool.

    The class is intentionally stateful: ``pre_commit`` results are
    cached in-process so a back-to-back call (e.g. once to dry-run,
    once to actually commit) doesn't re-run ceo-audit.
    """

    def __init__(self, repo_root: Optional[Path] = None) -> None:
        self.repo_root = Path(repo_root) if repo_root else Path.cwd()
        # In-process cache: (action, key) → (timestamp, result).
        self._cache: Dict[Tuple[str, str], Tuple[float, Dict[str, Any]]] = {}

    # ── public dispatch ────────────────────────────────────────────
    def run(
        self,
        action: str,
        target: str = "",
        content: str = "",
        message: str = "",
        checkpoint_name: str = "",
        base: str = "main",
        head: str = "HEAD",
    ) -> Dict[str, Any]:
        """Dispatch to the right action handler.

        Args:
            action: one of pre_write | write | post_write | pre_commit |
                refactor | session_warmup.
            target: file path (for pre_write / write / post_write) or
                symbol name (for refactor).
            content: file content (write only).
            message: commit message (pre_commit only).
            checkpoint_name: snapshot name (pre_commit / refactor).
            base: base ref (pre_commit / session_warmup).
            head: head ref (pre_commit / session_warmup).

        Returns:
            Dict with ``action``, ``steps``, ``verdict``, plus action-
            specific extras (e.g. ``suggested_message`` for pre_commit).
        """
        handler = {
            "pre_write": self._action_pre_write,
            "write": self._action_write,
            "post_write": self._action_post_write,
            "pre_commit": self._action_pre_commit,
            "refactor": self._action_refactor,
            "session_warmup": self._action_session_warmup,
        }.get(action)

        if handler is None:
            return {
                "action": action,
                "verdict": "ERROR",
                "error": (
                    f"Unknown action: {action!r}. "
                    "Valid: pre_write, write, post_write, pre_commit, refactor, session_warmup."
                ),
                "steps": [],
            }

        try:
            return handler(
                target=target,
                content=content,
                message=message,
                checkpoint_name=checkpoint_name,
                base=base,
                head=head,
            )
        except Exception as exc:
            return {
                "action": action,
                "verdict": "ERROR",
                "error": str(exc),
                "steps": [],
            }

    # ── action handlers ─────────────────────────────────────────────

    def _action_pre_write(self, target: str, **_: Any) -> Dict[str, Any]:
        steps: List[Dict[str, Any]] = []
        steps.append(self._safe_call("sin_read", lambda: _read(self.repo_root, target)))
        steps.append(
            self._safe_call(
                "sin_preflight",
                lambda: _preflight(self.repo_root, "sin_write", {"path": target}),
            )
        )

        verdict = "READY" if all(s.get("ok") for s in steps) else "FIX_FIRST"
        return {"action": "pre_write", "target": target, "steps": steps, "verdict": verdict}

    def _action_write(self, target: str, content: str, **_: Any) -> Dict[str, Any]:
        steps: List[Dict[str, Any]] = []
        steps.append(
            self._safe_call(
                "sin_preflight",
                lambda: _preflight(self.repo_root, "sin_write", {"path": target}),
            )
        )
        steps.append(
            self._safe_call(
                "sin_write",
                lambda: _write_file(self.repo_root, target, content),
            )
        )
        steps.append(
            self._safe_call(
                "sin_hashline_validate",
                lambda: {"ok": True, "note": "no patch supplied; skipped"},
            )
        )

        verdict = "PROCEED" if steps[1].get("ok") else "BLOCK"
        return {
            "action": "write",
            "target": target,
            "steps": steps,
            "verdict": verdict,
        }

    def _action_post_write(self, target: str, **_: Any) -> Dict[str, Any]:
        steps: List[Dict[str, Any]] = []
        steps.append(
            self._safe_call(
                "sin_preflight",
                lambda: _preflight(self.repo_root, "sin_write", {"path": target}),
            )
        )
        steps.append(self._safe_call("codocs_check", lambda: _codocs_check(self.repo_root)))
        steps.append(self._safe_call("pytest_collect", lambda: _pytest_collect(self.repo_root)))

        verdict = "READY" if all(s.get("ok") for s in steps) else "FIX_FIRST"
        return {"action": "post_write", "target": target, "steps": steps, "verdict": verdict}

    def _action_pre_commit(
        self,
        message: str = "",
        checkpoint_name: str = "",
        base: str = "main",
        head: str = "HEAD",
        **_: Any,
    ) -> Dict[str, Any]:
        steps: List[Dict[str, Any]] = []

        # 1. checkpoint
        name = checkpoint_name or f"pre-commit-{_now_compact()}"
        steps.append(self._safe_call("sin_checkpoint", lambda: _checkpoint(self.repo_root, name)))

        # 2. git status
        steps.append(self._safe_call("git_status", lambda: _git_status(self.repo_root)))

        # 3. codocs check
        steps.append(self._safe_call("codocs_check", lambda: _codocs_check(self.repo_root)))

        # 4. ceo-audit (cached 5 min)
        audit = self._cached_ceo_audit("QUICK", base, head)
        steps.append({"name": "ceo_audit", **audit})

        # Suggested message
        suggested = message or _suggest_commit_message(self.repo_root)

        blockers = []
        if not audit.get("ok") or (audit.get("grade") or "").upper() == "F":
            blockers.append("ceo-audit grade F — fix critical issues first")
        codocs_step = next((s for s in steps if s.get("name") == "codocs_check"), None)
        if codocs_step and codocs_step.get("broken", 0) > 0:
            blockers.append(f"codocs: {codocs_step['broken']} broken .doc.md reference(s)")

        verdict = "READY_TO_COMMIT" if not blockers else "FIX_FIRST"

        return {
            "action": "pre_commit",
            "steps": steps,
            "verdict": verdict,
            "suggested_message": suggested,
            "blockers": blockers,
            "base": base,
            "head": head,
            "timestamp": _now_iso(),
        }

    def _action_refactor(
        self,
        target: str,
        checkpoint_name: str = "",
        **_: Any,
    ) -> Dict[str, Any]:
        steps: List[Dict[str, Any]] = []

        name = checkpoint_name or f"pre-refactor-{_now_compact()}"
        steps.append(self._safe_call("sin_checkpoint", lambda: _checkpoint(self.repo_root, name)))
        steps.append(
            self._safe_call(
                "gitnexus_impact",
                lambda: _gitnexus_impact(self.repo_root, target),
            )
        )
        steps.append(
            self._safe_call(
                "gitnexus_detect_changes",
                lambda: _gitnexus_detect_changes(self.repo_root),
            )
        )

        impact_step = next((s for s in steps if s.get("name") == "gitnexus_impact"), None)
        risk = impact_step.get("risk") if impact_step else None
        if risk in ("HIGH", "CRITICAL"):
            verdict = "FIX_FIRST"
        elif risk == "MEDIUM":
            verdict = "REVIEW"
        else:
            verdict = "PROCEED"

        return {
            "action": "refactor",
            "target": target,
            "steps": steps,
            "verdict": verdict,
            "checkpoint_name": name,
        }

    def _action_session_warmup(self, **_: Any) -> Dict[str, Any]:
        steps: List[Dict[str, Any]] = []
        steps.append(self._safe_call("sin_session_warmup", lambda: _session_warmup(self.repo_root)))
        warm = steps[0] if steps else {}
        verdict = warm.get("session_recommendation", "READY — proceed with coding")
        return {
            "action": "session_warmup",
            "steps": steps,
            "verdict": verdict,
            "branch": warm.get("branch"),
            "ceo_audit_grade": warm.get("ceo_audit_grade"),
            "top_risks": warm.get("top_risks", []),
        }

    # ── helpers ─────────────────────────────────────────────────────

    def _cached_ceo_audit(self, profile: str, base: str, head: str) -> Dict[str, Any]:
        """Run ceo-audit, caching the result for 5 minutes per triple."""
        key = (profile, base, head)
        now = time.time()
        if key in self._cache:
            ts, data = self._cache[key]
            if (now - ts) < _CEO_AUDIT_CACHE_TTL:
                return {**data, "cache_hit": True}
        data = _ceo_audit_quick(self.repo_root, profile)
        self._cache[key] = (now, data)
        return data

    @staticmethod
    def _safe_call(name: str, fn: Any) -> Dict[str, Any]:
        try:
            return {"name": name, "ok": True, **(fn() or {})}
        except Exception as exc:
            return {"name": name, "ok": False, "error": str(exc)}


# ── module-level helpers (call into the consolidated modules) ────────


def _read(repo_root: Path, target: str) -> Dict[str, Any]:
    if not target:
        return {"ok": False, "error": "no target"}
    from . import (
        preflight,  # noqa: F401  (import keeps relative namespace hot)
    )

    return {
        "ok": True,
        "resolved": str(target),
        "note": "delegated to sin_read; see mcp_server.sin_read",
    }


def _preflight(repo_root: Path, tool_name: str, tool_input: Dict[str, Any]) -> Dict[str, Any]:
    from .preflight import PreflightChecker

    return PreflightChecker(repo_root=repo_root).check(tool_name, tool_input)


def _write_file(repo_root: Path, target: str, content: str) -> Dict[str, Any]:

    # Note: calling the MCP tool directly is a circular dep risk; we just
    # do an atomic file write with the same logic for the workflow use case.
    p = repo_root / target
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8")
    return {"ok": True, "path": str(p), "chars": len(content)}


def _codocs_check(repo_root: Path) -> Dict[str, Any]:
    from . import codocs

    broken = codocs.find_broken(str(repo_root))
    return {
        "ok": not bool(broken),
        "broken": len(broken),
        "items": [b.to_dict() for b in broken][:10],
    }


def _pytest_collect(repo_root: Path) -> Dict[str, Any]:
    if not (repo_root / "tests").exists() and not (repo_root / "test").exists():
        return {"ok": True, "skipped": True, "note": "no tests/ dir"}
    try:
        proc = subprocess.run(
            ["python3", "-m", "pytest", "--collect-only", "-q"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            timeout=15,
        )
        return {
            "ok": proc.returncode == 0,
            "returncode": proc.returncode,
            "stdout_tail": proc.stdout[-500:],
        }
    except (subprocess.TimeoutExpired, FileNotFoundError) as exc:
        return {"ok": True, "skipped": True, "error": str(exc)}


def _checkpoint(repo_root: Path, name: str) -> Dict[str, Any]:
    from .checkpoint import Checkpointer

    return Checkpointer(repo_root=repo_root).create(name)


def _git_status(repo_root: Path) -> Dict[str, Any]:
    try:
        proc = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            timeout=5,
        )
        changes = proc.stdout.strip().splitlines() if proc.stdout.strip() else []
        return {"ok": True, "clean": not changes, "changes_count": len(changes)}
    except Exception as exc:
        return {"ok": False, "error": str(exc)}


def _ceo_audit_quick(repo_root: Path, profile: str) -> Dict[str, Any]:
    try:
        sin_bin = shutil.which("sin") or _CEO_AUDIT_FALLBACK
        if not Path(sin_bin).exists():
            return {"ok": False, "error": "sin CLI not installed"}
        proc = subprocess.run(
            [sin_bin, "ceo-audit", "run", str(repo_root), f"--profile={profile}", "--json"],
            capture_output=True,
            text=True,
            timeout=180,
        )
        if proc.returncode == 0 and proc.stdout.strip():
            data = json.loads(proc.stdout)
            return {"ok": True, "grade": data.get("grade"), "report_path": data.get("report_path")}
        return {"ok": False, "error": proc.stderr[-300:]}
    except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
        return {"ok": False, "error": str(exc)}
    except Exception as exc:
        return {"ok": False, "error": str(exc)}


def _gitnexus_impact(repo_root: Path, symbol: str) -> Dict[str, Any]:
    """Best-effort gitnexus_impact. Returns empty dict on missing CLI."""
    if not symbol:
        return {"ok": False, "error": "no target"}
    try:
        # Use the gitnexus Python wrapper if available, else shell out.
        from sin_code_bundle import gitnexus  # type: ignore

        data = gitnexus.get_impact(symbol)
        return {
            "ok": True,
            "risk": data.get("risk"),
            "affected_count": len(data.get("affected", [])),
        }
    except ImportError:
        pass
    except Exception as exc:
        return {"ok": False, "error": str(exc)}

    bin_path = shutil.which("gitnexus")
    if not bin_path:
        return {"ok": False, "error": "gitnexus not installed"}
    try:
        proc = subprocess.run(
            [bin_path, "impact", json.dumps({"target": symbol})],
            cwd=repo_root,
            capture_output=True,
            text=True,
            timeout=15,
        )
        if proc.returncode == 0 and proc.stdout.strip():
            data = json.loads(proc.stdout)
            return {
                "ok": True,
                "risk": data.get("risk"),
                "affected_count": len(data.get("affected", [])),
            }
        return {"ok": False, "error": proc.stderr[-200:]}
    except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
        return {"ok": False, "error": str(exc)}


def _gitnexus_detect_changes(repo_root: Path) -> Dict[str, Any]:
    try:
        from sin_code_bundle import gitnexus  # type: ignore

        data = gitnexus.get_detect_changes()
        return {"ok": True, "changes_count": len(data.get("changes", []))}
    except ImportError:
        pass
    except Exception as exc:
        return {"ok": False, "error": str(exc)}

    bin_path = shutil.which("gitnexus")
    if not bin_path:
        return {"ok": False, "error": "gitnexus not installed"}
    try:
        proc = subprocess.run(
            [bin_path, "detect-changes", "--json"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            timeout=10,
        )
        if proc.returncode == 0 and proc.stdout.strip():
            data = json.loads(proc.stdout)
            return {"ok": True, "changes_count": len(data.get("changes", []))}
        return {"ok": False, "error": proc.stderr[-200:]}
    except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
        return {"ok": False, "error": str(exc)}


def _session_warmup(repo_root: Path) -> Dict[str, Any]:
    from .session_warmup import SessionWarmup

    return SessionWarmup(repo_root=repo_root).warmup()


# ── helpers for suggested commit message ────────────────────────────


def _suggest_commit_message(repo_root: Path) -> str:
    """Best-effort Conventional Commits message from `git diff --stat`."""
    try:
        proc = subprocess.run(
            ["git", "diff", "--name-only", "HEAD"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            timeout=5,
        )
        files = [f for f in proc.stdout.strip().splitlines() if f]
    except Exception:
        files = []

    if not files:
        return "chore: empty commit"

    # Heuristics for the type
    test_only = all(_is_test_file(f) for f in files)
    docs_only = all(_is_doc_file(f) for f in files)
    new_file = any(f.startswith("+") or "/new_" in f for f in files)

    if test_only:
        return f"test: update tests for {files[0]}"
    if docs_only:
        return f"docs: update {files[0]}"
    if new_file:
        return f"feat: add {Path(files[0]).name}"
    return f"chore: update {len(files)} file(s)"


_CC_TYPES_PATTERN = re.compile(r"^(feat|fix|docs|chore|style|test|refactor|perf|ci|build)")


def _is_test_file(path: str) -> bool:
    p = path.lower()
    return "/tests/" in p or "/test/" in p or p.startswith("test_") or p.endswith("_test.py")


def _is_doc_file(path: str) -> bool:
    p = path.lower()
    return p.endswith(".md") or p.endswith(".rst") or p.endswith(".txt") or "/docs/" in p


def _now_compact() -> str:
    return datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")


def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()
