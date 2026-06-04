"""Purpose: Isolated worktree orchestration — parallel agent tasks without conflicts.

Docs: orchestration_worktrees.doc.md
"""
from __future__ import annotations

import os
import shutil
import subprocess
import uuid
from pathlib import Path
from typing import Any, Callable, Optional


class SINWorktreeOrchestrator:
    """Manages isolated git worktrees for parallel agent task execution."""

    def __init__(self, repo_root: Optional[Path] = None):
        self.repo_root = repo_root or Path.cwd()
        self.active_worktrees: list[Path] = []

    def is_git_repo(self) -> bool:
        return (self.repo_root / ".git").exists()

    def create_worktree(self, branch_name: Optional[str] = None) -> dict:
        if not self.is_git_repo():
            return {"error": "Not a git repository. Worktree isolation requires git."}
        branch = branch_name or f"sin-task-{uuid.uuid4().hex[:8]}"
        worktree_path = self.repo_root.parent / f".sin-worktrees-{self.repo_root.name}" / branch
        try:
            subprocess.run(
                ["git", "worktree", "add", str(worktree_path), "-b", branch],
                cwd=self.repo_root, check=True, capture_output=True, text=True,
            )
            self.active_worktrees.append(worktree_path)
            return {
                "success": True, "worktree_path": str(worktree_path),
                "branch": branch,
                "message": f"Isolated worktree created at {worktree_path}",
            }
        except subprocess.CalledProcessError as e:
            return {"error": f"Git worktree creation failed: {e.stderr}"}

    def execute_in_worktree(self, worktree_path: str, task_func: Callable, *args, **kwargs) -> dict:
        original_cwd = os.getcwd()
        try:
            os.chdir(worktree_path)
            result = task_func(*args, **kwargs)
            return {"success": True, "result": result}
        except Exception as e:
            return {"success": False, "error": str(e)}
        finally:
            os.chdir(original_cwd)

    def cleanup_worktree(self, worktree_path: str, merge_back: bool = False) -> dict:
        path = Path(worktree_path)
        if path not in self.active_worktrees:
            return {"error": "Worktree not managed by this orchestrator"}
        try:
            if merge_back:
                branch = path.name
                subprocess.run(["git", "checkout", "main"], cwd=self.repo_root, check=True, capture_output=True)
                merge_result = subprocess.run(
                    ["git", "merge", "--no-ff", branch, "-m", f"Auto-merge from SIN worktree: {branch}"],
                    cwd=self.repo_root, capture_output=True, text=True,
                )
                if merge_result.returncode != 0:
                    return {"error": f"Merge conflict: {merge_result.stderr}"}
            subprocess.run(
                ["git", "worktree", "remove", str(path), "--force"],
                cwd=self.repo_root, check=True, capture_output=True,
            )
            self.active_worktrees.remove(path)
            if path.exists():
                shutil.rmtree(path)
            return {"success": True, "message": "Worktree cleaned up successfully"}
        except subprocess.CalledProcessError as e:
            return {"error": f"Worktree cleanup failed: {e.stderr}"}
