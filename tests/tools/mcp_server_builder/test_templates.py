# SPDX-License-Identifier: MIT
"""
Purpose: Test the templates engine and template discovery.
Docs: test_templates.doc.md
"""

import pytest

from sin_code_bundle.tools.mcp_server_builder.templates import (
    TEMPLATE_REGISTRY,
    TemplateEngine,
    list_templates,
)


class TestTemplateRegistry:
    def test_registry_has_three_templates(self):
        """All three canonical templates must be registered."""
        assert set(TEMPLATE_REGISTRY) == {"python-fastmcp", "node-mcp", "go-mcp"}

    def test_python_template_metadata(self):
        meta = TEMPLATE_REGISTRY["python-fastmcp"]
        assert meta["language"] == "python"
        assert meta["framework"] == "fastmcp"
        assert meta["min_python"] == "3.10"

    def test_node_template_metadata(self):
        meta = TEMPLATE_REGISTRY["node-mcp"]
        assert meta["language"] == "javascript"
        assert "min_node" in meta

    def test_go_template_metadata(self):
        meta = TEMPLATE_REGISTRY["go-mcp"]
        assert meta["language"] == "go"
        assert "min_go" in meta


class TestTemplateEngine:
    def test_list_templates_returns_three(self):
        engine = TemplateEngine()
        result = engine.list_templates()
        assert len(result) == 3
        assert {t["name"] for t in result} == {"python-fastmcp", "node-mcp", "go-mcp"}

    def test_list_templates_includes_metadata(self):
        engine = TemplateEngine()
        result = engine.list_templates()
        first = result[0]
        assert "name" in first
        assert "language" in first
        assert "framework" in first
        assert "entry_point" in first

    def test_get_template_dir_python(self):
        engine = TemplateEngine()
        path = engine.get_template_dir("python-fastmcp")
        assert path.is_dir()
        assert (path / "pyproject.toml").is_file()

    def test_get_template_dir_node(self):
        engine = TemplateEngine()
        path = engine.get_template_dir("node-mcp")
        assert path.is_dir()
        assert (path / "package.json").is_file()

    def test_get_template_dir_go(self):
        engine = TemplateEngine()
        path = engine.get_template_dir("go-mcp")
        assert path.is_dir()
        assert (path / "go.mod").is_file()

    def test_get_template_dir_unknown_raises(self):
        engine = TemplateEngine()
        with pytest.raises(FileNotFoundError):
            engine.get_template_dir("rust-mcp")

    def test_get_template_dir_missing_on_disk(self, tmp_path):
        engine = TemplateEngine(templates_dir=tmp_path)
        with pytest.raises(FileNotFoundError):
            engine.get_template_dir("python-fastmcp")

    def test_render_to_string_substitutes(self):
        engine = TemplateEngine()
        text = engine.render_to_string(
            "python-fastmcp",
            "pyproject.toml",
            {
                "name": "my-tool",
                "slug": "my-tool",
                "version": "0.2.0",
                "description": "Test",
                "pkg": "my_tool",
            },
        )
        assert "my-tool" in text
        assert "0.2.0" in text
        assert "Test" in text

    def test_iter_files(self):
        engine = TemplateEngine()
        files = list(engine.iter_files("python-fastmcp"))
        assert len(files) > 0
        # All entries must be files (we filter to is_file()).
        for f in files:
            assert f.is_file()


class TestModuleLevelHelper:
    def test_list_templates_helper(self):
        result = list_templates()
        assert len(result) == 3
