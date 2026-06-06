# Purpose: Test skill installation, removal, and opencode.json registration
# Docs: test_installer.py.doc.md
"""Tests for sin_code_bundle.tools.marketplace.installer."""

import json
import tempfile
from pathlib import Path

import pytest

from sin_code_bundle.tools.marketplace.installer import InstallError, Installer


# ── Fixtures ──────────────────────────────────────────────────────────────────
@pytest.fixture
def tmp_installer() -> Installer:
    with tempfile.TemporaryDirectory() as tmpdir:
        skills_dir = Path(tmpdir) / "skills"
        config_path = Path(tmpdir) / "opencode.json"
        yield Installer(skills_dir=skills_dir, config_path=config_path)


# ── Install ───────────────────────────────────────────────────────────────────
class TestInstallerInstall:
    def test_install_from_git_url(self, tmp_installer: Installer) -> None:
        # We'll simulate a simple git repo locally
        with tempfile.TemporaryDirectory() as repo_dir:
            import subprocess
            subprocess.run(["git", "init", "--bare", repo_dir], check=True, capture_output=True)
            # Actually, let's use a real local repo
        # For this test, we'll test with a simple file path
        with tempfile.TemporaryDirectory() as source_dir:
            source_path = Path(source_dir)
            (source_path / "SKILL.md").write_text("# Test Skill")
            # Make it a git repo
            import subprocess
            subprocess.run(["git", "init", source_dir], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.email", "test@test.com"],
                check=True, capture_output=True,
            )
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.name", "Test"],
                check=True, capture_output=True,
            )
            (source_path / "file.txt").write_text("content")
            subprocess.run(["git", "-C", source_dir, "add", "."], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "commit", "-m", "init"],
                check=True, capture_output=True,
            )

            record = tmp_installer.install(
                slug="test-skill",
                source=source_dir,
                destination="test-skill",
                name="Test Skill",
                description="A test skill",
            )
            assert record["slug"] == "test-skill"
            assert (tmp_installer.skills_dir / "test-skill").exists()

    def test_install_overwrites_existing(self, tmp_installer: Installer) -> None:
        with tempfile.TemporaryDirectory() as source_dir:
            source_path = Path(source_dir)
            (source_path / "SKILL.md").write_text("# Test Skill")
            import subprocess
            subprocess.run(["git", "init", source_dir], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.email", "test@test.com"],
                check=True, capture_output=True,
            )
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.name", "Test"],
                check=True, capture_output=True,
            )
            (source_path / "file.txt").write_text("content")
            subprocess.run(["git", "-C", source_dir, "add", "."], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "commit", "-m", "init"],
                check=True, capture_output=True,
            )

            tmp_installer.install(
                slug="test-skill",
                source=source_dir,
                destination="test-skill",
            )
            # Install again
            record = tmp_installer.install(
                slug="test-skill",
                source=source_dir,
                destination="test-skill",
            )
            assert record["slug"] == "test-skill"

    def test_install_registers_in_opencode_json(self, tmp_installer: Installer) -> None:
        with tempfile.TemporaryDirectory() as source_dir:
            source_path = Path(source_dir)
            (source_path / "SKILL.md").write_text("# Test Skill")
            import subprocess
            subprocess.run(["git", "init", source_dir], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.email", "test@test.com"],
                check=True, capture_output=True,
            )
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.name", "Test"],
                check=True, capture_output=True,
            )
            (source_path / "file.txt").write_text("content")
            subprocess.run(["git", "-C", source_dir, "add", "."], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "commit", "-m", "init"],
                check=True, capture_output=True,
            )

            tmp_installer.install(
                slug="test-skill",
                source=source_dir,
                destination="test-skill",
                name="Test Skill",
            )
            with tmp_installer.config_path.open("r", encoding="utf-8") as fh:
                config = json.load(fh)
            assert "skills" in config
            assert "test-skill" in config["skills"]
            assert config["skills"]["test-skill"]["name"] == "Test Skill"

    def test_install_bad_source(self, tmp_installer: Installer) -> None:
        with pytest.raises(InstallError):
            tmp_installer.install(
                slug="test-skill",
                source="/nonexistent/repo",
                destination="test-skill",
            )


# ── Remove ────────────────────────────────────────────────────────────────────
class TestInstallerRemove:
    def test_remove(self, tmp_installer: Installer) -> None:
        with tempfile.TemporaryDirectory() as source_dir:
            source_path = Path(source_dir)
            (source_path / "SKILL.md").write_text("# Test Skill")
            import subprocess
            subprocess.run(["git", "init", source_dir], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.email", "test@test.com"],
                check=True, capture_output=True,
            )
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.name", "Test"],
                check=True, capture_output=True,
            )
            (source_path / "file.txt").write_text("content")
            subprocess.run(["git", "-C", source_dir, "add", "."], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "commit", "-m", "init"],
                check=True, capture_output=True,
            )

            tmp_installer.install(
                slug="test-skill",
                source=source_dir,
                destination="test-skill",
            )
            assert tmp_installer.remove("test-skill") is True
            assert not (tmp_installer.skills_dir / "test-skill").exists()

    def test_remove_not_found(self, tmp_installer: Installer) -> None:
        assert tmp_installer.remove("not-found") is False

    def test_remove_updates_opencode_json(self, tmp_installer: Installer) -> None:
        with tempfile.TemporaryDirectory() as source_dir:
            source_path = Path(source_dir)
            (source_path / "SKILL.md").write_text("# Test Skill")
            import subprocess
            subprocess.run(["git", "init", source_dir], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.email", "test@test.com"],
                check=True, capture_output=True,
            )
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.name", "Test"],
                check=True, capture_output=True,
            )
            (source_path / "file.txt").write_text("content")
            subprocess.run(["git", "-C", source_dir, "add", "."], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "commit", "-m", "init"],
                check=True, capture_output=True,
            )

            tmp_installer.install(
                slug="test-skill",
                source=source_dir,
                destination="test-skill",
            )
            tmp_installer.remove("test-skill")
            with tmp_installer.config_path.open("r", encoding="utf-8") as fh:
                config = json.load(fh)
            assert "test-skill" not in config.get("skills", {})


# ── List ──────────────────────────────────────────────────────────────────────
class TestInstallerList:
    def test_list_installed(self, tmp_installer: Installer) -> None:
        with tempfile.TemporaryDirectory() as source_dir:
            source_path = Path(source_dir)
            (source_path / "SKILL.md").write_text("# Test Skill")
            import subprocess
            subprocess.run(["git", "init", source_dir], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.email", "test@test.com"],
                check=True, capture_output=True,
            )
            subprocess.run(
                ["git", "-C", source_dir, "config", "user.name", "Test"],
                check=True, capture_output=True,
            )
            (source_path / "file.txt").write_text("content")
            subprocess.run(["git", "-C", source_dir, "add", "."], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", source_dir, "commit", "-m", "init"],
                check=True, capture_output=True,
            )

            tmp_installer.install(
                slug="test-skill",
                source=source_dir,
                destination="test-skill",
            )
            installed = tmp_installer.list_installed()
            assert len(installed) == 1
            assert installed[0]["slug"] == "test-skill"

    def test_list_installed_empty(self, tmp_installer: Installer) -> None:
        installed = tmp_installer.list_installed()
        assert len(installed) == 0
