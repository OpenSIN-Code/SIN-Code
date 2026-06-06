# Purpose: Test MCP server tools (search, install, list, remove, info, update, sync)
# Docs: test_server.py.doc.md
"""Tests for sin_code_bundle.tools.marketplace.server MCP tools."""

import json
import tempfile
from pathlib import Path

import pytest
import respx
from httpx import Response

from sin_code_bundle.tools.marketplace.server import (
    marketplace_info,
    marketplace_install,
    marketplace_list,
    marketplace_remove,
    marketplace_search,
    marketplace_sync,
    marketplace_update,
)

CATALOG_URL = "https://raw.githubusercontent.com/OpenSIN-Code/Infra-SIN-OpenCode-Stack/main/skills/catalog.json"


def _clear_cache() -> None:
    cache_path = Path.home() / ".config" / "opencode" / "skills_catalog.json"
    if cache_path.exists():
        cache_path.unlink()


# ── Fixtures ──────────────────────────────────────────────────────────────────
@pytest.fixture
def sample_catalog() -> list[dict]:
    return [
        {
            "slug": "test-skill",
            "name": "Test Skill",
            "title": "Test",
            "description": "A test skill",
            "source": "https://github.com/test/skill",
            "destination": "/tmp/skills/test-skill",
        },
    ]


# ── Search ────────────────────────────────────────────────────────────────────
class TestMcpSearch:
    @respx.mock
    async def test_marketplace_search(self, sample_catalog: list) -> None:
        _clear_cache()
        respx.get(CATALOG_URL).mock(
            return_value=Response(200, json=sample_catalog)
        )
        result = await marketplace_search("test")
        data = json.loads(result)
        assert data["count"] == 1
        assert data["results"][0]["slug"] == "test-skill"

    @respx.mock
    async def test_marketplace_search_no_matches(self, sample_catalog: list) -> None:
        _clear_cache()
        respx.get(CATALOG_URL).mock(
            return_value=Response(200, json=sample_catalog)
        )
        result = await marketplace_search("nonexistent")
        data = json.loads(result)
        assert data["count"] == 0


# ── Install ───────────────────────────────────────────────────────────────────
class TestMcpInstall:
    @respx.mock
    async def test_marketplace_install_not_found(self) -> None:
        _clear_cache()
        respx.get(CATALOG_URL).mock(
            return_value=Response(200, json=[])
        )
        result = await marketplace_install("not-found")
        data = json.loads(result)
        assert "error" in data

    @respx.mock
    async def test_marketplace_install_found(self, sample_catalog: list) -> None:
        _clear_cache()
        respx.get(CATALOG_URL).mock(
            return_value=Response(200, json=sample_catalog)
        )
        # Will fail because source is not a real git repo
        result = await marketplace_install("test-skill")
        data = json.loads(result)
        assert "error" in data or data.get("success") is False


# ── List ──────────────────────────────────────────────────────────────────────
class TestMcpList:
    async def test_marketplace_list_empty(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            import sin_code_bundle.tools.marketplace.registry
            old_default = sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = db_path
            result = await marketplace_list()
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = old_default
            data = json.loads(result)
            assert data["count"] == 0


# ── Remove ────────────────────────────────────────────────────────────────────
class TestMcpRemove:
    async def test_marketplace_remove(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            import sin_code_bundle.tools.marketplace.registry
            old_default = sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = db_path
            result = await marketplace_remove("not-installed")
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = old_default
            data = json.loads(result)
            assert data["success"] is False


# ── Info ───────────────────────────────────────────────────────────────────────
class TestMcpInfo:
    @respx.mock
    async def test_marketplace_info_found(self, sample_catalog: list) -> None:
        _clear_cache()
        respx.get(CATALOG_URL).mock(
            return_value=Response(200, json=sample_catalog)
        )
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            import sin_code_bundle.tools.marketplace.registry
            old_default = sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = db_path
            result = await marketplace_info("test-skill")
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = old_default
            data = json.loads(result)
            assert data["slug"] == "test-skill"
            assert data["catalog"] is not None

    @respx.mock
    async def test_marketplace_info_not_found(self) -> None:
        _clear_cache()
        respx.get(CATALOG_URL).mock(
            return_value=Response(200, json=[])
        )
        result = await marketplace_info("not-found")
        data = json.loads(result)
        assert data["slug"] == "not-found"
        assert data["catalog"] is None


# ── Update ───────────────────────────────────────────────────────────────────
class TestMcpUpdate:
    async def test_marketplace_update_specific(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            skills_dir = Path(tmpdir) / "skills"
            skills_dir.mkdir()
            import sin_code_bundle.tools.marketplace.updater
            old_default = sin_code_bundle.tools.marketplace.updater.DEFAULT_SKILLS_DIR
            sin_code_bundle.tools.marketplace.updater.DEFAULT_SKILLS_DIR = skills_dir
            result = await marketplace_update("test-skill")
            sin_code_bundle.tools.marketplace.updater.DEFAULT_SKILLS_DIR = old_default
            data = json.loads(result)
            assert "results" in data

    async def test_marketplace_update_all(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            skills_dir = Path(tmpdir) / "skills"
            skills_dir.mkdir()
            import sin_code_bundle.tools.marketplace.updater
            old_default = sin_code_bundle.tools.marketplace.updater.DEFAULT_SKILLS_DIR
            sin_code_bundle.tools.marketplace.updater.DEFAULT_SKILLS_DIR = skills_dir
            result = await marketplace_update()
            sin_code_bundle.tools.marketplace.updater.DEFAULT_SKILLS_DIR = old_default
            data = json.loads(result)
            assert "results" in data


# ── Sync ───────────────────────────────────────────────────────────────────────
class TestMcpSync:
    @respx.mock
    async def test_marketplace_sync_success(self, sample_catalog: list) -> None:
        _clear_cache()
        respx.get(CATALOG_URL).mock(
            return_value=Response(200, json=sample_catalog)
        )
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            import sin_code_bundle.tools.marketplace.registry
            old_default = sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = db_path
            result = await marketplace_sync()
            sin_code_bundle.tools.marketplace.registry.DEFAULT_DB_PATH = old_default
            data = json.loads(result)
            assert data["success"] is True
            assert data["count"] == 1

    @respx.mock
    async def test_marketplace_sync_failure(self) -> None:
        _clear_cache()
        respx.get(CATALOG_URL).mock(
            return_value=Response(404, text="Not Found")
        )
        result = await marketplace_sync()
        data = json.loads(result)
        assert "error" in data
