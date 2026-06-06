# Purpose: Clone repos, install dependencies, and register skills in opencode.json
# Docs: installer.py.doc.md
"""
Skill installer for the OpenSIN-Code marketplace.

Handles cloning skill repositories, running setup scripts, and
registering the skill in the local opencode.json configuration.
"""

import json
import logging
import shutil
import subprocess
from pathlib import Path
from typing import Any

import git

# ── Logger ──────────────────────────────────────────────────────────────────
logger = logging.getLogger(__name__)

# ── Constants ─────────────────────────────────────────────────────────────────
DEFAULT_SKILLS_DIR = Path.home() / ".config" / "opencode" / "skills"
OPENCODE_CONFIG = Path.home() / ".config" / "opencode" / "opencode.json"


class InstallError(Exception):
    """Raised when installation fails."""

    pass


# ── Installer ─────────────────────────────────────────────────────────────────
class Installer:
    """Installs skills from remote or local sources.

    Steps:
    1. Clone (or copy) the skill repository to the local skills directory.
    2. Run any setup script (``install.sh``, ``setup.py``, etc.).
    3. Register the skill in ``opencode.json``.
    4. Record the installation in the SQLite registry.
    """

    def __init__(
        self,
        skills_dir: Path | str | None = None,
        config_path: Path | str | None = None,
    ) -> None:
        """Initialize with paths.

        Args:
            skills_dir: Directory where skills are cloned. Defaults to
                        ``~/.config/opencode/skills``.
            config_path: Path to opencode.json. Defaults to
                         ``~/.config/opencode/opencode.json``.
        """
        self.skills_dir = Path(skills_dir) if skills_dir else DEFAULT_SKILLS_DIR
        self.config_path = Path(config_path) if config_path else OPENCODE_CONFIG
        self.skills_dir.mkdir(parents=True, exist_ok=True)

    # ── Install ───────────────────────────────────────────────────────────────
    def install(
        self,
        slug: str,
        source: str,
        destination: str | None = None,
        name: str | None = None,
        title: str | None = None,
        description: str | None = None,
    ) -> dict[str, Any]:
        """Install a skill from a Git repository.

        Args:
            slug: Unique skill identifier.
            source: Git URL or local path to the skill repository.
            destination: Local directory name under ``skills_dir``.
                           Defaults to ``slug``.
            name: Human-readable name. Defaults to ``slug``.
            title: Short title. Defaults to ``name``.
            description: Long description.

        Returns:
            Metadata dict about the installed skill.

        Raises:
            InstallError: If cloning or registration fails.
        """
        dest_name = destination or slug
        dest_path = self.skills_dir / dest_name

        if dest_path.exists():
            logger.info("Destination %s exists — removing old copy", dest_path)
            shutil.rmtree(dest_path)

        logger.info("Cloning %s → %s", source, dest_path)
        try:
            git.Repo.clone_from(source, dest_path, depth=1)
        except git.GitCommandError as exc:
            raise InstallError(f"Clone failed: {exc}") from exc

        self._run_setup(dest_path)

        record = {
            "slug": slug,
            "name": name or slug,
            "title": title or name or slug,
            "description": description,
            "source": source,
            "destination": str(dest_path),
        }
        self._register_in_opencode_json(record)
        return record

    def _run_setup(self, path: Path) -> None:
        """Run ``install.sh`` or ``setup.py`` if present."""
        install_script = path / "install.sh"
        if install_script.exists():
            logger.info("Running install.sh in %s", path)
            try:
                subprocess.run(
                    ["bash", str(install_script)],
                    cwd=path,
                    check=True,
                    capture_output=True,
                    text=True,
                )
            except subprocess.CalledProcessError as exc:
                logger.warning("install.sh failed: %s", exc.stderr)

        setup_py = path / "setup.py"
        pyproject = path / "pyproject.toml"
        if setup_py.exists() or pyproject.exists():
            logger.info("Running pip install in %s", path)
            try:
                subprocess.run(
                    ["python3", "-m", "pip", "install", "-e", str(path)],
                    check=True,
                    capture_output=True,
                    text=True,
                )
            except subprocess.CalledProcessError as exc:
                logger.warning("pip install failed: %s", exc.stderr)

    def _register_in_opencode_json(self, record: dict[str, Any]) -> None:
        """Add the skill to opencode.json under the ``skills`` key.

        Args:
            record: Skill metadata with at least ``slug`` and ``destination``.
        """
        config: dict[str, Any] = {}
        if self.config_path.exists():
            try:
                with self.config_path.open("r", encoding="utf-8") as fh:
                    config = json.load(fh)
            except (json.JSONDecodeError, OSError) as exc:
                logger.warning("Could not read opencode.json: %s", exc)

        skills = config.setdefault("skills", {})
        skills[record["slug"]] = {
            "path": record["destination"],
            "name": record.get("name", record["slug"]),
            "description": record.get("description", ""),
        }

        try:
            with self.config_path.open("w", encoding="utf-8") as fh:
                json.dump(config, fh, indent=2)
        except OSError as exc:
            raise InstallError(f"Could not write opencode.json: {exc}") from exc

        logger.info("Registered %s in opencode.json", record["slug"])

    # ── Remove ──────────────────────────────────────────────────────────────────
    def remove(self, slug: str) -> bool:
        """Remove a skill from disk and opencode.json.

        Args:
            slug: Skill identifier.

        Returns:
            True if the skill was removed, False if not found.
        """
        dest_path = self.skills_dir / slug
        removed = False
        if dest_path.exists():
            shutil.rmtree(dest_path)
            logger.info("Removed %s from disk", dest_path)
            removed = True

        if self.config_path.exists():
            try:
                with self.config_path.open("r", encoding="utf-8") as fh:
                    config = json.load(fh)
            except (json.JSONDecodeError, OSError):
                config = {}

            skills = config.get("skills", {})
            if slug in skills:
                del skills[slug]
                with self.config_path.open("w", encoding="utf-8") as fh:
                    json.dump(config, fh, indent=2)
                logger.info("Unregistered %s from opencode.json", slug)
                removed = True

        return removed

    def list_installed(self) -> list[dict[str, Any]]:
        """List all skills currently present in the skills directory.

        Returns:
            List of dicts with ``slug`` and ``path``.
        """
        results: list[dict[str, Any]] = []
        if not self.skills_dir.exists():
            return results
        for entry in self.skills_dir.iterdir():
            if entry.is_dir():
                results.append({"slug": entry.name, "path": str(entry)})
        return results
