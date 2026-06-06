# Purpose: End-to-end integration tests for full marketplace workflow
# Docs: test_integration.py.doc.md
"""Integration tests for the full marketplace workflow."""

import json
import subprocess
import tempfile
from pathlib import Path

import pytest

from sin_code_bundle.tools.marketplace.catalog import Catalog
from sin_code_bundle.tools.marketplace.installer import Installer
from sin_code_bundle.tools.marketplace.registry import Registry
from sin_code_bundle.tools.marketplace.updater import Updater


# ── Integration workflow ────────────────────────────────────────────────────
class TestIntegration:
    def test_full_workflow(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            skills_dir = Path(tmpdir) / "skills"
            config_path = Path(tmpdir) / "opencode.json"
            db_path = Path(tmpdir) / "marketplace.db"

            # Create a fake skill repo
            source_dir = Path(tmpdir) / "source_repo"
            source_dir.mkdir()
            (source_dir / "SKILL.md").write_text("# Test Skill")
            subprocess.run(["git", "init", str(source_dir)], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", str(source_dir), "config", "user.email", "test@test.com"],
                check=True, capture_output=True,
            )
            subprocess.run(
                ["git", "-C", str(source_dir), "config", "user.name", "Test"],
                check=True, capture_output=True,
            )
            (source_dir / "file.txt").write_text("content")
            subprocess.run(["git", "-C", str(source_dir), "add", "."], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", str(source_dir), "commit", "-m", "init"],
                check=True, capture_output=True,
            )

            # Catalog
            catalog = Catalog([
                {
                    "slug": "test-skill",
                    "name": "Test Skill",
                    "title": "Test",
                    "description": "Integration test skill",
                    "source": str(source_dir),
                    "destination": str(skills_dir / "test-skill"),
                }
            ])
            assert len(catalog) == 1

            # Install
            installer = Installer(skills_dir=skills_dir, config_path=config_path)
            entry = catalog.get_by_slug("test-skill")
            assert entry is not None
            record = installer.install(
                slug="test-skill",
                source=entry["source"],
                destination=entry["destination"],
                name=entry["name"],
                title=entry["title"],
                description=entry["description"],
            )
            assert (skills_dir / "test-skill").exists()
            assert config_path.exists()

            # Register
            registry = Registry(db_path)
            registry.install(record)
            assert registry.exists("test-skill")

            # List
            skills = registry.list_all()
            assert len(skills) == 1
            assert skills[0]["slug"] == "test-skill"

            # Check update status
            updater = Updater(skills_dir=skills_dir)
            status = updater.check_status("test-skill")
            assert "message" in status

            # Update
            update_result = updater.update("test-skill")
            assert "message" in update_result

            # Remove
            installer.remove("test-skill")
            registry.remove("test-skill")
            assert not registry.exists("test-skill")
            assert not (skills_dir / "test-skill").exists()

            # Verify config cleaned
            with config_path.open("r", encoding="utf-8") as fh:
                config = json.load(fh)
            assert "test-skill" not in config.get("skills", {})

    def test_sync_to_cache(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            cache_path = Path(tmpdir) / "skills_catalog.json"
            skills = [
                {"slug": "a", "name": "A", "description": "desc"},
                {"slug": "b", "name": "B", "description": "desc"},
            ]
            with cache_path.open("w", encoding="utf-8") as fh:
                json.dump(skills, fh)

            catalog = Catalog()
            catalog.load_file(cache_path)
            assert len(catalog) == 2
            assert catalog.get_by_slug("a") is not None

    def test_search_across_fields(self) -> None:
        catalog = Catalog([
            {"slug": "skill-a", "name": "Alpha", "title": "Alpha Skill", "description": "First"},
            {"slug": "skill-b", "name": "Beta", "title": "Beta Skill", "description": "Second"},
            {"slug": "skill-c", "name": "Gamma", "title": "Gamma Skill", "description": "Third"},
        ])
        # Search by slug
        assert len(catalog.search("skill-a")) == 1
        # Search by name
        assert len(catalog.search("Beta")) == 1
        # Search by title
        assert len(catalog.search("Gamma")) == 1
        # Search by description
        assert len(catalog.search("First")) == 1
        # Search across all
        assert len(catalog.search("Skill")) == 3

    def test_installer_idempotent(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            skills_dir = Path(tmpdir) / "skills"
            config_path = Path(tmpdir) / "opencode.json"
            source_dir = Path(tmpdir) / "source_repo"
            source_dir.mkdir()
            (source_dir / "SKILL.md").write_text("# Test")
            subprocess.run(["git", "init", str(source_dir)], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", str(source_dir), "config", "user.email", "test@test.com"],
                check=True, capture_output=True,
            )
            subprocess.run(
                ["git", "-C", str(source_dir), "config", "user.name", "Test"],
                check=True, capture_output=True,
            )
            (source_dir / "file.txt").write_text("content")
            subprocess.run(["git", "-C", str(source_dir), "add", "."], check=True, capture_output=True)
            subprocess.run(
                ["git", "-C", str(source_dir), "commit", "-m", "init"],
                check=True, capture_output=True,
            )

            installer = Installer(skills_dir=skills_dir, config_path=config_path)
            installer.install(slug="test", source=str(source_dir), destination="test")
            # Reinstall
            installer.install(slug="test", source=str(source_dir), destination="test")
            # Should still be one entry in config
            with config_path.open("r", encoding="utf-8") as fh:
                config = json.load(fh)
            assert len(config["skills"]) == 1
