# Purpose: Check for updates, git pull, and version comparison
# Docs: updater.py.doc.md
"""
Skill updater for the OpenSIN-Code marketplace.

Checks installed skills for updates, performs git pull, and
compares local HEAD with remote to determine if a skill is outdated.
"""

import logging
from pathlib import Path
from typing import Any

import git

# ── Logger ──────────────────────────────────────────────────────────────────
logger = logging.getLogger(__name__)

# ── Constants ─────────────────────────────────────────────────────────────────
DEFAULT_SKILLS_DIR = Path.home() / ".config" / "opencode" / "skills"


class UpdateError(Exception):
    """Raised when update operations fail."""

    pass


# ── Updater ───────────────────────────────────────────────────────────────────
class Updater:
    """Manages updates for installed skills.

    Uses GitPython to inspect local repositories and determine
    whether the remote has newer commits.
    """

    def __init__(self, skills_dir: Path | str | None = None) -> None:
        """Initialize with skills directory.

        Args:
            skills_dir: Directory containing cloned skill repos.
                        Defaults to ``~/.config/opencode/skills``.
        """
        self.skills_dir = Path(skills_dir) if skills_dir else DEFAULT_SKILLS_DIR

    # ── Update single ───────────────────────────────────────────────────────────
    def update(self, slug: str) -> dict[str, Any]:
        """Pull latest changes for a single skill.

        Args:
            slug: Skill identifier (directory name under ``skills_dir``).

        Returns:
            Dict with ``slug``, ``success``, ``commits_behind``, and ``message``.

        Raises:
            UpdateError: If the skill directory is not a valid git repo.
        """
        path = self.skills_dir / slug
        if not path.exists():
            return {"slug": slug, "success": False, "message": "Not installed"}

        try:
            repo = git.Repo(path)
        except git.InvalidGitRepositoryError as exc:
            raise UpdateError(f"{path} is not a git repository") from exc

        if repo.bare:
            return {"slug": slug, "success": False, "message": "Bare repo"}

        try:
            origin = repo.remotes.origin
            fetch_info = origin.fetch()
            local_sha = repo.head.commit.hexsha
            remote_sha = origin.refs[repo.active_branch.name].commit.hexsha
        except (AttributeError, IndexError, git.GitCommandError) as exc:
            return {"slug": slug, "success": False, "message": f"Fetch failed: {exc}"}

        if local_sha == remote_sha:
            return {"slug": slug, "success": True, "commits_behind": 0, "message": "Up to date"}

        try:
            origin.pull()
        except (AttributeError, git.GitCommandError) as exc:
            return {"slug": slug, "success": False, "message": f"Pull failed: {exc}"}

        commits_behind = sum(1 for _ in repo.iter_commits(f"{local_sha}..{remote_sha}"))
        return {
            "slug": slug,
            "success": True,
            "commits_behind": commits_behind,
            "message": f"Updated ({commits_behind} commits)",
        }

    # ── Update all ──────────────────────────────────────────────────────────────
    def update_all(self) -> list[dict[str, Any]]:
        """Pull latest changes for all installed skills.

        Returns:
            List of result dicts from each ``update()`` call.
        """
        results: list[dict[str, Any]] = []
        if not self.skills_dir.exists():
            return results
        for entry in self.skills_dir.iterdir():
            if entry.is_dir() and (entry / ".git").exists():
                try:
                    results.append(self.update(entry.name))
                except UpdateError as exc:
                    results.append({"slug": entry.name, "success": False, "message": str(exc)})
        return results

    # ── Check status ────────────────────────────────────────────────────────────
    def check_status(self, slug: str) -> dict[str, Any]:
        """Check whether a skill is behind remote without pulling.

        Args:
            slug: Skill identifier.

        Returns:
            Dict with ``slug``, ``behind``, ``commits_behind``, and ``message``.
        """
        path = self.skills_dir / slug
        if not path.exists():
            return {"slug": slug, "behind": False, "commits_behind": 0, "message": "Not installed"}

        try:
            repo = git.Repo(path)
        except git.InvalidGitRepositoryError:
            return {"slug": slug, "behind": False, "commits_behind": 0, "message": "Not a git repo"}

        if repo.bare:
            return {"slug": slug, "behind": False, "commits_behind": 0, "message": "Bare repo"}

        try:
            origin = repo.remotes.origin
            origin.fetch()
            local_sha = repo.head.commit.hexsha
            remote_sha = origin.refs[repo.active_branch.name].commit.hexsha
        except (AttributeError, IndexError, git.GitCommandError) as exc:
            return {"slug": slug, "behind": False, "commits_behind": 0, "message": f"Fetch failed: {exc}"}

        if local_sha == remote_sha:
            return {"slug": slug, "behind": False, "commits_behind": 0, "message": "Up to date"}

        commits_behind = sum(1 for _ in repo.iter_commits(f"{local_sha}..{remote_sha}"))
        return {
            "slug": slug,
            "behind": True,
            "commits_behind": commits_behind,
            "message": f"{commits_behind} commits behind",
        }

    def check_all(self) -> list[dict[str, Any]]:
        """Check update status for all installed skills.

        Returns:
            List of status dicts.
        """
        results: list[dict[str, Any]] = []
        if not self.skills_dir.exists():
            return results
        for entry in self.skills_dir.iterdir():
            if entry.is_dir() and (entry / ".git").exists():
                results.append(self.check_status(entry.name))
        return results
