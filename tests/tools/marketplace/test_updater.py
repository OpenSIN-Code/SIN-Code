# SPDX-License-Identifier: MIT
# Purpose: Test git pull and update status checks
# Docs: test_updater.py.doc.md
"""Tests for sin_code_bundle.tools.marketplace.updater."""

import subprocess
import tempfile
from pathlib import Path

import pytest

from sin_code_bundle.tools.marketplace.updater import Updater, UpdateError


# ── Helpers ───────────────────────────────────────────────────────────────────
def _init_git_repo(path: str) -> None:
    subprocess.run(["git", "init", path], check=True, capture_output=True)
    subprocess.run(
        ["git", "-C", path, "config", "user.email", "test@test.com"],
        check=True, capture_output=True,
    )
    subprocess.run(
        ["git", "-C", path, "config", "user.name", "Test"],
        check=True, capture_output=True,
    )
    Path(path, "file.txt").write_text("v1")
    subprocess.run(["git", "-C", path, "add", "."], check=True, capture_output=True)
    subprocess.run(
        ["git", "-C", path, "commit", "-m", "v1"],
        check=True, capture_output=True,
    )


@pytest.fixture
def tmp_updater() -> Updater:
    with tempfile.TemporaryDirectory() as tmpdir:
        skills_dir = Path(tmpdir) / "skills"
        skills_dir.mkdir()
        yield Updater(skills_dir=skills_dir)


# ── Update ────────────────────────────────────────────────────────────────────
class TestUpdaterUpdate:
    def test_update_not_installed(self, tmp_updater: Updater) -> None:
        result = tmp_updater.update("not-installed")
        assert result["success"] is False
        assert "Not installed" in result["message"]

    def test_update_up_to_date(self, tmp_updater: Updater) -> None:
        skill_dir = tmp_updater.skills_dir / "test-skill"
        skill_dir.mkdir()
        _init_git_repo(str(skill_dir))
        result = tmp_updater.update("test-skill")
        # It will try to fetch but there's no remote
        assert result["success"] is False or "Fetch failed" in result["message"]

    def test_update_all_empty(self, tmp_updater: Updater) -> None:
        results = tmp_updater.update_all()
        assert len(results) == 0

    def test_update_all_with_skills(self, tmp_updater: Updater) -> None:
        skill_dir = tmp_updater.skills_dir / "test-skill"
        skill_dir.mkdir()
        _init_git_repo(str(skill_dir))
        results = tmp_updater.update_all()
        assert len(results) == 1

    def test_check_status_not_installed(self, tmp_updater: Updater) -> None:
        result = tmp_updater.check_status("not-installed")
        assert result["behind"] is False
        assert "Not installed" in result["message"]

    def test_check_status_up_to_date(self, tmp_updater: Updater) -> None:
        skill_dir = tmp_updater.skills_dir / "test-skill"
        skill_dir.mkdir()
        _init_git_repo(str(skill_dir))
        result = tmp_updater.check_status("test-skill")
        # No remote set, so fetch will fail
        assert "Fetch failed" in result["message"]

    def test_check_all_empty(self, tmp_updater: Updater) -> None:
        results = tmp_updater.check_all()
        assert len(results) == 0

    def test_check_all_with_skills(self, tmp_updater: Updater) -> None:
        skill_dir = tmp_updater.skills_dir / "test-skill"
        skill_dir.mkdir()
        _init_git_repo(str(skill_dir))
        results = tmp_updater.check_all()
        assert len(results) == 1

    def test_invalid_git_repo(self, tmp_updater: Updater) -> None:
        skill_dir = tmp_updater.skills_dir / "test-skill"
        skill_dir.mkdir()
        # Not a git repo
        with pytest.raises(UpdateError):
            tmp_updater.update("test-skill")

    def test_bare_repo(self, tmp_updater: Updater) -> None:
        skill_dir = tmp_updater.skills_dir / "test-skill"
        skill_dir.mkdir()
        subprocess.run(["git", "init", "--bare", str(skill_dir)], check=True, capture_output=True)
        result = tmp_updater.update("test-skill")
        assert result["success"] is False
        assert "Bare repo" in result["message"]
