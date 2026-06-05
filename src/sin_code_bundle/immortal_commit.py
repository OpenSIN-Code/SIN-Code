"""Purpose: One-call immortal commit — conventional commit + tag + push.

Docs: immortal_commit.doc.md

Wraps the git-immortal-commit ritual into a single MCP tool call.
Validates Conventional Commits format, ensures we are on main (NEVER a
branch), creates the commit, optionally tags + pushes, and returns a
structured result.
"""

from __future__ import annotations

import json
import re
import shutil
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

# Conventional Commit format: type(scope): subject (subject >= 5 chars).
# Permitted types mirror the git-immortal-commit skill (AGENTS.md) — adding
# a new type here means adding it to that skill as well.
_CC_PATTERN = re.compile(
    r"^(feat|fix|docs|chore|style|test|refactor|perf|ci|build)"
    r"(\([^)]+\))?"
    r": .{5,}"
)

# Substrings that should NEVER appear in a commit message (would be a
# leaked secret in the public history).
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

# Main-branch names. Repository-specific overrides (e.g. "master") are
# detected at runtime when the user passes a custom name.
_DEFAULT_MAIN = "main"

# Hard-coded fallback for the dev-machine layout (AGENTS.md). The MCP
# stdio process inherits a stripped PATH in some envs, so we look up
# the rollback CLI at well-known locations first.
_ROLLBACK_FALLBACK = "/Users/jeremy/Library/Python/3.14/bin/sin-honcho-rollback"


class ImmortalCommitter:
    """One-call commit/tag/push with all safety checks applied."""

    def __init__(self, repo_root: Optional[Path] = None) -> None:
        self.repo_root = Path(repo_root) if repo_root else Path.cwd()

    def commit(
        self,
        message: str,
        tag: str = "",
        push: bool = True,
        force_main: bool = True,
        main_branch: str = _DEFAULT_MAIN,
        snapshot_first: bool = True,
    ) -> Dict[str, Any]:
        """Run the full immortal-commit ritual.

        Args:
            message: Conventional Commits message (``feat(scope): subject``).
            tag: optional annotated tag name (e.g. ``v0.8.0``).
            push: if True, push commit (and tag) to ``origin/<main_branch>``.
            force_main: if True, refuse to run on any branch other than main.
            main_branch: which branch counts as ``main`` (default ``main``).

        Returns:
            Dict with ``success``, ``sha``, ``tag``, ``pushed``, ``branch``,
            ``warnings`` and ``steps`` (per-step status) — always returns
            (never raises) so the caller can surface a useful error.
        """
        result: Dict[str, Any] = {
            "success": False,
            "message": message,
            "tag": tag or None,
            "pushed": False,
            "branch": None,
            "sha": None,
            "warnings": [],
            "steps": [],
            "snapshot": None,
            "timestamp": datetime.now(timezone.utc).isoformat(),
        }

        # ── Step 0 (optional): Pre-commit snapshot ─────────────────────
        # Lets the user roll back to the pre-commit state if the change
        # later turns out to be broken. Independent of the commit itself.
        if snapshot_first:
            snap_name = (
                f"pre-commit-{(message[:24].replace(' ', '-').replace(':', '').lower() or 'auto')}"
            )
            snap = self._create_snapshot(snap_name, message)
            result["snapshot"] = snap
            if snap.get("ok"):
                result["steps"].append(
                    {"step": "snapshot", "ok": True, "id": snap.get("snapshot_id")}
                )
            else:
                # Snapshot is best-effort; log but don't fail the commit.
                result["steps"].append(
                    {"step": "snapshot", "ok": False, "warning": snap.get("error")}
                )

        # ── Step 1: Validate Conventional Commits format ─────────────
        if not _CC_PATTERN.match(message):
            result["error"] = (
                "Not a Conventional Commits message. "
                "Required: type(scope): subject (subject >= 5 chars). "
                "Valid types: feat, fix, docs, chore, style, test, refactor, perf, ci, build."
            )
            result["steps"].append({"step": "validate_format", "ok": False})
            return result
        result["steps"].append({"step": "validate_format", "ok": True})

        # ── Step 2: Scan message for secrets (cheap, not exhaustive) ──
        secret_hits = [s for s in _SECRET_HINTS if s in message]
        if secret_hits:
            result["error"] = f"Possible secret material in commit message: {secret_hits}"
            result["steps"].append({"step": "secret_scan", "ok": False})
            return result
        result["steps"].append({"step": "secret_scan", "ok": True})

        # ── Step 3: Detect current branch ─────────────────────────────
        branch = self._git(["branch", "--show-current"], default="").strip()
        result["branch"] = branch

        if force_main and branch != main_branch:
            result["error"] = (
                f"Refusing to commit: on branch '{branch}', expected '{main_branch}'. "
                "Per the NEVER-BRANCHES mandate, switch to main first: "
                f"`git checkout {main_branch} && git pull origin {main_branch}`."
            )
            result["steps"].append({"step": "branch_check", "ok": False})
            return result
        result["steps"].append({"step": "branch_check", "ok": True, "branch": branch})

        # ── Step 4: Working-tree dirty? Warn if so, but still proceed ─
        # Per skill: "agents are autonomous, stop blocking on dirty tree,
        # but flag it so the user can see what is being committed together".
        status = self._git(["status", "--porcelain"], default="")
        if status.strip():
            result["warnings"].append(
                f"Working tree is dirty ({len(status.splitlines())} entries) — committing all"
            )

        # ── Step 5: git add -A + commit ────────────────────────────────
        add_proc = self._run(["git", "add", "-A"])
        if not add_proc["ok"]:
            result["error"] = f"git add failed: {add_proc['stderr']}"
            result["steps"].append({"step": "git_add", "ok": False})
            return result
        result["steps"].append({"step": "git_add", "ok": True})

        commit_proc = self._run(["git", "commit", "-m", message])
        if not commit_proc["ok"]:
            # No changes staged is a soft error: surface it but don't
            # treat it as fatal — the user may have wanted a no-op.
            if "nothing to commit" in (commit_proc["stdout"] + commit_proc["stderr"]).lower():
                result["error"] = "Nothing to commit — working tree clean after git add"
                result["steps"].append({"step": "git_commit", "ok": False, "soft": True})
                return result
            result["error"] = f"git commit failed: {commit_proc['stderr']}"
            result["steps"].append({"step": "git_commit", "ok": False})
            return result
        result["steps"].append({"step": "git_commit", "ok": True})

        # ── Step 6: Capture the new SHA ───────────────────────────────
        sha = self._git(["rev-parse", "HEAD"], default="").strip()
        result["sha"] = sha

        # ── Step 7: Optional annotated tag ────────────────────────────
        if tag:
            tag_proc = self._run(["git", "tag", "-a", tag, "-m", f"Release {tag}"])
            if not tag_proc["ok"]:
                # If tag already exists, that's a soft error (idempotent).
                if "already exists" in (tag_proc["stderr"]).lower():
                    result["warnings"].append(f"Tag '{tag}' already exists locally — keeping")
                else:
                    result["error"] = f"git tag failed: {tag_proc['stderr']}"
                    result["steps"].append({"step": "git_tag", "ok": False})
                    return result
            result["steps"].append({"step": "git_tag", "ok": True})

        # ── Step 8: Optional push to origin ───────────────────────────
        if push:
            push_proc = self._run(["git", "push", "origin", branch or main_branch])
            if not push_proc["ok"]:
                result["error"] = f"git push failed: {push_proc['stderr']}"
                result["steps"].append({"step": "git_push", "ok": False})
                return result
            result["pushed"] = True
            result["steps"].append({"step": "git_push", "ok": True})

            if tag:
                # Tags are pushed separately (unless --follow-tags was used).
                tag_push = self._run(["git", "push", "origin", tag])
                if not tag_push["ok"]:
                    result["warnings"].append(
                        f"Tag '{tag}' was not pushed: {tag_push['stderr'][-200:]}"
                    )
                else:
                    result["steps"].append({"step": "git_push_tag", "ok": True})

        result["success"] = True
        return result

    # ── helpers ─────────────────────────────────────────────────────
    def _git(self, args: List[str], default: str = "") -> str:
        """Run a git command, return stdout (or ``default`` on failure)."""
        proc = subprocess.run(
            ["git", *args],
            cwd=self.repo_root,
            capture_output=True,
            text=True,
            timeout=30,
        )
        return proc.stdout if proc.returncode == 0 else default

    def _run(self, args: List[str]) -> Dict[str, Any]:
        """Run a subprocess, return dict with ok/stdout/stderr/returncode."""
        try:
            proc = subprocess.run(
                args,
                cwd=self.repo_root,
                capture_output=True,
                text=True,
                timeout=30,
            )
            return {
                "ok": proc.returncode == 0,
                "stdout": proc.stdout,
                "stderr": proc.stderr,
                "returncode": proc.returncode,
            }
        except subprocess.TimeoutExpired:
            return {"ok": False, "stdout": "", "stderr": "timeout after 30s", "returncode": -1}
        except Exception as exc:
            return {"ok": False, "stdout": "", "stderr": str(exc), "returncode": -1}

    def _create_snapshot(self, name: str, description: str) -> Dict[str, Any]:
        """Best-effort pre-commit snapshot via sin-honcho-rollback.

        Never raises. Returns ``{ok, snapshot_id}`` on success, ``{ok: False,
        error}`` on any failure (missing CLI, non-zero exit, JSON parse
        error, timeout).
        """
        try:
            rb_bin = shutil.which("sin-honcho-rollback") or _ROLLBACK_FALLBACK
            if not Path(rb_bin).exists():
                return {"ok": False, "error": "sin-honcho-rollback not installed"}
            proc = subprocess.run(
                [
                    rb_bin,
                    "snapshot",
                    name,
                    "--description",
                    description or f"pre-commit checkpoint: {name}",
                    "--db",
                    str(self.repo_root / ".sin" / "rollback.db"),
                ],
                capture_output=True,
                text=True,
                timeout=15,
            )
            if proc.returncode == 0 and proc.stdout.strip():
                data = json.loads(proc.stdout)
                return {
                    "ok": True,
                    "snapshot_id": data.get("snapshot", {}).get("id") or data.get("id"),
                }
            return {"ok": False, "error": proc.stderr[-300:]}
        except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
            return {"ok": False, "error": str(exc)}
        except Exception as exc:
            return {"ok": False, "error": str(exc)}
