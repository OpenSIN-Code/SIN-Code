# SPDX-License-Identifier: MIT
"""Git worktree isolation + saga-style merge-back with compensation."""

from __future__ import annotations

import subprocess
from dataclasses import dataclass
from pathlib import Path


class GitError(RuntimeError):
    pass


def _git(repo: str | Path, *args: str, check: bool = True) -> str:
    proc = subprocess.run(
        ["git", "-C", str(repo), *args],
        capture_output=True, text=True, timeout=120,
    )
    if check and proc.returncode != 0:
        raise GitError(f"git {' '.join(args)}: {proc.stderr.strip()}")
    return proc.stdout.strip()


@dataclass
class Worktree:
    repo: Path
    path: Path
    branch: str
    base_branch: str

    def diff_stat(self) -> str:
        return _git(self.path, "diff", "--stat", self.base_branch)

    def diff(self, max_bytes: int = 200_000) -> str:
        out = _git(self.path, "diff", self.base_branch)
        return out[:max_bytes]

    def commit_all(self, message: str) -> bool:
        _git(self.path, "add", "-A")
        status = _git(self.path, "status", "--porcelain")
        if not status:
            return False
        _git(self.path, "commit", "-m", message, "--no-verify")
        return True

    def merge_back(self) -> str:
        snapshot = f"sin-snap/{self.branch.replace('/', '_')}"
        _git(self.repo, "tag", "-f", snapshot, self.base_branch)
        try:
            _git(self.path, "rebase", self.base_branch)
        except GitError as e:
            _git(self.path, "rebase", "--abort", check=False)
            raise GitError(
                f"rebase conflict on {self.branch}; branch preserved for "
                f"manual resolution. base untouched. ({e})") from e
        try:
            _git(self.repo, "merge", "--ff-only", self.branch)
        except GitError as e:
            _git(self.repo, "reset", "--hard", snapshot, check=False)
            raise GitError(
                f"ff-merge failed, base restored to {snapshot}: {e}"
            ) from e
        return snapshot

    def destroy(self, delete_branch: bool = True) -> None:
        _git(self.repo, "worktree", "remove", "--force",
            str(self.path), check=False)
        if delete_branch:
            _git(self.repo, "branch", "-D", self.branch, check=False)


class WorktreeManager:
    def __init__(self, repo: str | Path, base_branch: str = "main") -> None:
        self.repo = Path(repo).resolve()
        self.base_branch = base_branch
        if not (self.repo / ".git").exists():
            raise GitError(f"{self.repo} is not a git repository")

    def create(self, plan_id: str, task_id: str) -> Worktree:
        branch = f"sin/delegate/{plan_id}/{task_id}"
        wt_path = self.repo / ".sin-worktrees" / plan_id / task_id
        wt_path.parent.mkdir(parents=True, exist_ok=True)
        if wt_path.exists():
            return Worktree(self.repo, wt_path, branch, self.base_branch)
        _git(self.repo, "branch", "-f", branch, self.base_branch)
        _git(self.repo, "worktree", "add", str(wt_path), branch)
        return Worktree(self.repo, wt_path, branch, self.base_branch)
