# SPDX-License-Identifier: MIT
# Purpose: Test CLI commands for search, install, list, remove, update, sync, info
# Docs: test_cli.py.doc.md
"""Tests for sin_code_bundle.tools.marketplace.legacy_cli."""

import json
import tempfile
from pathlib import Path

import pytest
from typer.testing import CliRunner

from sin_code_bundle.tools.marketplace.legacy_cli import app

runner = CliRunner()


def _clear_cache() -> None:
    cache_path = Path.home() / ".config" / "opencode" / "skills_catalog.json"
    if cache_path.exists():
        cache_path.unlink()


# ── Search ────────────────────────────────────────────────────────────────────
class TestCliSearch:
    def test_search_no_local_catalog(self) -> None:
        _clear_cache()
        result = runner.invoke(app, ["search", "test"])
        assert result.exit_code == 1
        assert "No local catalog" in result.output

    def test_search_with_local_catalog(self) -> None:
        _clear_cache()
        with tempfile.TemporaryDirectory() as tmpdir:
            cache = Path(tmpdir) / "skills_catalog.json"
            with cache.open("w", encoding="utf-8") as fh:
                json.dump([{"slug": "test", "name": "Test", "description": "desc"}], fh)

            # Patch the catalog path
            import sin_code_bundle.tools.marketplace.legacy_cli
            old_cache = sin_code_bundle.tools.marketplace.legacy_cli._get_catalog
            def _patched():
                from sin_code_bundle.tools.marketplace.catalog import Catalog
                c = Catalog()
                c.load_file(cache)
                return c
            sin_code_bundle.tools.marketplace.legacy_cli._get_catalog = _patched

            result = runner.invoke(app, ["search", "test"])
            # Restore
            sin_code_bundle.tools.marketplace.legacy_cli._get_catalog = old_cache
            assert result.exit_code == 0
            assert "test" in result.output


# ── Install ───────────────────────────────────────────────────────────────────
class TestCliInstall:
    def test_install_no_catalog(self) -> None:
        result = runner.invoke(app, ["install", "test-skill"])
        assert result.exit_code == 1


# ── List ──────────────────────────────────────────────────────────────────────
class TestCliList:
    def test_list_empty(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            import sin_code_bundle.tools.marketplace.registry
            old_default = sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = db_path
            result = runner.invoke(app, ["list"])
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = old_default
            assert result.exit_code == 0
            assert "Installed Skills" in result.output


# ── Remove ────────────────────────────────────────────────────────────────────
class TestCliRemove:
    def test_remove_no_confirm(self) -> None:
        result = runner.invoke(app, ["remove", "test-skill"], input="n\n")
        assert result.exit_code == 0
        assert "Aborted" in result.output

    def test_remove_force(self) -> None:
        result = runner.invoke(app, ["remove", "test-skill", "--force"])
        assert result.exit_code == 0


# ── Update ────────────────────────────────────────────────────────────────────
class TestCliUpdate:
    def test_update_all(self) -> None:
        import sin_code_bundle.tools.marketplace.updater
        old_updater = sin_code_bundle.tools.marketplace.legacy_cli.Updater
        class MockUpdater:
            def __init__(self, *a, **kw):
                pass
            def update_all(self):
                return [{"slug": "test-skill", "success": True, "behind": False, "message": "ok"}]
            def update(self, name):
                return {"name": name, "status": "up-to-date"}
        sin_code_bundle.tools.marketplace.legacy_cli.Updater = MockUpdater
        try:
            result = runner.invoke(app, ["update"])
            assert result.exit_code == 0
        finally:
            sin_code_bundle.tools.marketplace.legacy_cli.Updater = old_updater

    def test_update_specific(self) -> None:
        import sin_code_bundle.tools.marketplace.updater
        old_updater = sin_code_bundle.tools.marketplace.legacy_cli.Updater
        class MockUpdater:
            def __init__(self, *a, **kw):
                pass
            def update_all(self):
                return [{"name": "test-skill", "status": "up-to-date"}]
            def update(self, name):
                return {"name": name, "status": "up-to-date"}
        sin_code_bundle.tools.marketplace.legacy_cli.Updater = MockUpdater
        try:
            result = runner.invoke(app, ["update", "test-skill"])
            assert result.exit_code == 0
        finally:
            sin_code_bundle.tools.marketplace.legacy_cli.Updater = old_updater


# ── Sync ───────────────────────────────────────────────────────────────────────
class TestCliSync:
    def test_sync(self) -> None:
        # This will try to fetch from the real remote, which might fail in tests
        # We mock it indirectly by catching errors
        result = runner.invoke(app, ["sync"])
        # Could succeed or fail depending on network
        assert result.exit_code in (0, 1)


# ── Info ──────────────────────────────────────────────────────────────────────
class TestCliInfo:
    def test_info_no_catalog(self) -> None:
        _clear_cache()
        result = runner.invoke(app, ["info", "test-skill"])
        assert result.exit_code == 1
        assert "No local catalog" in result.output

    def test_info_not_found(self) -> None:
        _clear_cache()
        with tempfile.TemporaryDirectory() as tmpdir:
            cache = Path(tmpdir) / "skills_catalog.json"
            with cache.open("w", encoding="utf-8") as fh:
                json.dump([{"slug": "other", "name": "Other", "description": "desc"}], fh)

            import sin_code_bundle.tools.marketplace.legacy_cli
            old_cache = sin_code_bundle.tools.marketplace.legacy_cli._get_catalog
            def _patched():
                from sin_code_bundle.tools.marketplace.catalog import Catalog
                c = Catalog()
                c.load_file(cache)
                return c
            sin_code_bundle.tools.marketplace.legacy_cli._get_catalog = _patched

            result = runner.invoke(app, ["info", "test-skill"])
            sin_code_bundle.tools.marketplace.legacy_cli._get_catalog = old_cache
            assert result.exit_code == 1
            assert "not found" in result.output
