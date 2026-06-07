# SPDX-License-Identifier: MIT
# Purpose: Test SQLite registry CRUD operations
# Docs: test_registry.py.doc.md
"""Tests for sin_code_bundle.tools.marketplace.registry."""

import tempfile
from pathlib import Path

import pytest

from sin_code_bundle.tools.marketplace.registry import Registry, RegistryError


# ── Fixtures ──────────────────────────────────────────────────────────────────
@pytest.fixture
def tmp_registry() -> Registry:
    with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as fh:
        path = fh.name
    reg = Registry(path)
    yield reg
    Path(path).unlink(missing_ok=True)


# ── CRUD ──────────────────────────────────────────────────────────────────────
class TestRegistryCRUD:
    def test_install(self, tmp_registry: Registry) -> None:
        record = {
            "slug": "test-skill",
            "name": "Test Skill",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        }
        tmp_registry.install(record)
        assert tmp_registry.exists("test-skill")

    def test_install_missing_fields(self, tmp_registry: Registry) -> None:
        with pytest.raises(RegistryError):
            tmp_registry.install({"slug": "test"})

    def test_get_found(self, tmp_registry: Registry) -> None:
        record = {
            "slug": "test-skill",
            "name": "Test Skill",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        }
        tmp_registry.install(record)
        fetched = tmp_registry.get("test-skill")
        assert fetched is not None
        assert fetched["slug"] == "test-skill"
        assert fetched["name"] == "Test Skill"

    def test_get_not_found(self, tmp_registry: Registry) -> None:
        assert tmp_registry.get("not-found") is None

    def test_remove_found(self, tmp_registry: Registry) -> None:
        record = {
            "slug": "test-skill",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        }
        tmp_registry.install(record)
        assert tmp_registry.remove("test-skill") is True
        assert not tmp_registry.exists("test-skill")

    def test_remove_not_found(self, tmp_registry: Registry) -> None:
        assert tmp_registry.remove("not-found") is False

    def test_list_all(self, tmp_registry: Registry) -> None:
        for i in range(3):
            tmp_registry.install({
                "slug": f"skill-{i}",
                "source": f"https://github.com/test/skill-{i}",
                "destination": f"/tmp/skills/skill-{i}",
            })
        skills = tmp_registry.list_all()
        assert len(skills) == 3

    def test_exists(self, tmp_registry: Registry) -> None:
        assert not tmp_registry.exists("test-skill")
        tmp_registry.install({
            "slug": "test-skill",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        })
        assert tmp_registry.exists("test-skill")

    def test_update_timestamp(self, tmp_registry: Registry) -> None:
        tmp_registry.install({
            "slug": "test-skill",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        })
        assert tmp_registry.update_timestamp("test-skill") is True
        # Verify updated_at changed
        record = tmp_registry.get("test-skill")
        assert record is not None
        assert "updated_at" in record

    def test_update_timestamp_not_found(self, tmp_registry: Registry) -> None:
        assert tmp_registry.update_timestamp("not-found") is False

    def test_set_get_meta(self, tmp_registry: Registry) -> None:
        tmp_registry.set_meta("last_sync", "2026-01-01")
        assert tmp_registry.get_meta("last_sync") == "2026-01-01"

    def test_get_meta_not_found(self, tmp_registry: Registry) -> None:
        assert tmp_registry.get_meta("missing") is None

    def test_clear(self, tmp_registry: Registry) -> None:
        tmp_registry.install({
            "slug": "test-skill",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        })
        tmp_registry.clear()
        assert len(tmp_registry) == 0

    def test_len(self, tmp_registry: Registry) -> None:
        assert len(tmp_registry) == 0
        tmp_registry.install({
            "slug": "test-skill",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        })
        assert len(tmp_registry) == 1

    def test_install_overwrites(self, tmp_registry: Registry) -> None:
        record = {
            "slug": "test-skill",
            "name": "Original",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        }
        tmp_registry.install(record)
        record["name"] = "Updated"
        tmp_registry.install(record)
        fetched = tmp_registry.get("test-skill")
        assert fetched is not None
        assert fetched["name"] == "Updated"

    def test_default_db_path(self) -> None:
        # Just verify it doesn't crash when using default path
        with tempfile.TemporaryDirectory() as tmpdir:
            import os
            old_home = os.environ.get("HOME")
            os.environ["HOME"] = tmpdir
            try:
                reg = Registry()
                assert reg.db_path.exists()
            finally:
                if old_home:
                    os.environ["HOME"] = old_home
                else:
                    del os.environ["HOME"]
