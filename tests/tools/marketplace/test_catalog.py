# SPDX-License-Identifier: MIT
# Purpose: Test catalog loading, search, and filtering
# Docs: test_catalog.py.doc.md
"""Tests for sin_code_bundle.tools.marketplace.catalog."""

import json
import tempfile
from pathlib import Path

import pytest
import respx
from httpx import Response

from sin_code_bundle.tools.marketplace.catalog import Catalog, CatalogError

# ── Fixtures ──────────────────────────────────────────────────────────────────
@pytest.fixture
def sample_catalog() -> list[dict]:
    return [
        {
            "slug": "sin-infisical",
            "name": "sin-infisical",
            "title": "SIN Infisical",
            "description": "Secret management skill",
            "source": "https://github.com/OpenSIN-Code/sin-infisical",
            "category": "security",
        },
        {
            "slug": "ceo-audit",
            "name": "CEO Audit",
            "title": "CEO Audit",
            "description": "Repository audit skill",
            "source": "https://github.com/OpenSIN-Code/ceo-audit",
            "category": "audit",
        },
        {
            "slug": "sin-browser-tools",
            "name": "sin-browser-tools",
            "title": "SIN Browser Tools",
            "description": "Browser automation skill",
            "source": "https://github.com/OpenSIN-Code/sin-browser-tools",
            "category": "automation",
        },
    ]


# ── Loading ───────────────────────────────────────────────────────────────────
class TestCatalogLoading:
    def test_load_file(self, sample_catalog: list) -> None:
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as fh:
            json.dump(sample_catalog, fh)
            path = fh.name

        catalog = Catalog()
        catalog.load_file(path)
        assert len(catalog) == 3
        Path(path).unlink()

    def test_load_file_not_found(self) -> None:
        catalog = Catalog()
        with pytest.raises(CatalogError):
            catalog.load_file("/nonexistent/catalog.json")

    def test_load_file_invalid_json(self) -> None:
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as fh:
            fh.write("not json")
            path = fh.name

        catalog = Catalog()
        with pytest.raises(CatalogError):
            catalog.load_file(path)
        Path(path).unlink()

    def test_load_file_not_a_list(self) -> None:
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as fh:
            json.dump({"key": "value"}, fh)
            path = fh.name

        catalog = Catalog()
        with pytest.raises(CatalogError):
            catalog.load_file(path)
        Path(path).unlink()

    @respx.mock
    async def test_load_remote_success(self, sample_catalog: list) -> None:
        route = respx.get("https://example.com/catalog.json").mock(
            return_value=Response(200, json=sample_catalog)
        )
        catalog = Catalog()
        await catalog.load_remote("https://example.com/catalog.json")
        assert len(catalog) == 3
        assert route.called

    @respx.mock
    async def test_load_remote_http_error(self) -> None:
        respx.get("https://example.com/catalog.json").mock(
            return_value=Response(404, text="Not Found")
        )
        catalog = Catalog()
        with pytest.raises(CatalogError):
            await catalog.load_remote("https://example.com/catalog.json")

    @respx.mock
    async def test_load_remote_invalid_json(self) -> None:
        respx.get("https://example.com/catalog.json").mock(
            return_value=Response(200, text="not json")
        )
        catalog = Catalog()
        with pytest.raises(CatalogError):
            await catalog.load_remote("https://example.com/catalog.json")

    @respx.mock
    async def test_load_remote_not_a_list(self) -> None:
        respx.get("https://example.com/catalog.json").mock(
            return_value=Response(200, json={"key": "value"})
        )
        catalog = Catalog()
        with pytest.raises(CatalogError):
            await catalog.load_remote("https://example.com/catalog.json")


# ── Queries ───────────────────────────────────────────────────────────────────
class TestCatalogQueries:
    def test_list_skills(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        skills = catalog.list_skills()
        assert len(skills) == 3
        assert skills[0]["slug"] == "sin-infisical"

    def test_search_by_name(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        results = catalog.search("audit")
        assert len(results) == 1
        assert results[0]["slug"] == "ceo-audit"

    def test_search_by_title(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        results = catalog.search("Browser")
        assert len(results) == 1
        assert results[0]["slug"] == "sin-browser-tools"

    def test_search_by_description(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        results = catalog.search("automation")
        assert len(results) == 1
        assert results[0]["slug"] == "sin-browser-tools"

    def test_search_no_matches(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        results = catalog.search("nonexistent")
        assert len(results) == 0

    def test_search_case_insensitive(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        results_upper = catalog.search("BROWSER")
        results_lower = catalog.search("browser")
        assert len(results_upper) == len(results_lower) == 1

    def test_get_by_slug_found(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        entry = catalog.get_by_slug("ceo-audit")
        assert entry is not None
        assert entry["name"] == "CEO Audit"

    def test_get_by_slug_not_found(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        entry = catalog.get_by_slug("not-real")
        assert entry is None

    def test_get_by_name_found(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        entry = catalog.get_by_name("CEO Audit")
        assert entry is not None
        assert entry["slug"] == "ceo-audit"

    def test_get_by_name_not_found(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        entry = catalog.get_by_name("Not Real")
        assert entry is None

    def test_filter_by_category(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        results = catalog.filter_by_category("security")
        assert len(results) == 1
        assert results[0]["slug"] == "sin-infisical"

    def test_filter_by_category_no_matches(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        results = catalog.filter_by_category("nope")
        assert len(results) == 0

    def test_bool_true(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        assert bool(catalog) is True

    def test_bool_false(self) -> None:
        catalog = Catalog()
        assert bool(catalog) is False

    def test_len(self, sample_catalog: list) -> None:
        catalog = Catalog(sample_catalog)
        assert len(catalog) == 3
