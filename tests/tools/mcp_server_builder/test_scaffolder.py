"""
Purpose: Test the Scaffolder and ScaffoldSpec.
Docs: test_scaffolder.doc.md
"""

import pytest

from sin_code_bundle.tools.mcp_server_builder.scaffolder import (
    Scaffolder,
    ScaffoldSpec,
    package_name,
    slugify,
)


class TestSlugify:
    def test_simple(self):
        assert slugify("My Cool Tool") == "my-cool-tool"

    def test_special_chars(self):
        assert slugify("Hello, World!") == "hello-world"

    def test_empty_falls_back(self):
        assert slugify("") == "unnamed"
        assert slugify("   ") == "unnamed"
        assert slugify("!!!") == "unnamed"

    def test_already_slug(self):
        assert slugify("already-slug") == "already-slug"


class TestPackageName:
    def test_no_dashes(self):
        assert package_name("my-tool") == "my_tool"

    def test_already_snake(self):
        assert package_name("my_tool") == "my_tool"


class TestScaffoldSpec:
    def test_basic(self):
        spec = ScaffoldSpec(name="My Tool", description="X")
        assert spec.slug == "my-tool"
        assert spec.pkg == "my_tool"
        assert spec.tools == ["ping"]  # default

    def test_unknown_template_raises(self):
        with pytest.raises(ValueError):
            ScaffoldSpec(name="x", template="rust-mcp")

    def test_invalid_tool_name_raises(self):
        with pytest.raises(ValueError):
            ScaffoldSpec(name="x", tools=["1bad"])

    def test_context_dict(self):
        spec = ScaffoldSpec(
            name="My Tool", tools=["do_x", "do_y"], template="python-fastmcp"
        )
        ctx = spec.to_context()
        assert ctx["name"] == "My Tool"
        assert ctx["slug"] == "my-tool"
        assert ctx["pkg"] == "my_tool"
        assert ctx["tools"] == ["do_x", "do_y"]
        assert "do_x" in ctx["tools_json"]
        assert ctx["version"] == "0.1.0"


class TestScaffolder:
    def test_dry_run(self):
        spec = ScaffoldSpec(name="Demo", template="python-fastmcp", tools=["a", "b"])
        summary = Scaffolder().dry_run(spec)
        assert summary["template"] == "python-fastmcp"
        assert summary["tools"] == ["a", "b"]
        assert "pyproject.toml" in summary["files"]

    def test_scaffold_creates_directory(self, tmp_path):
        target = tmp_path / "out"
        spec = ScaffoldSpec(name="Demo", template="python-fastmcp", tools=["a"])
        summary = Scaffolder().scaffold(target, spec)
        assert target.is_dir()
        assert (target / "pyproject.toml").is_file()
        assert (target / "README.md").is_file()
        assert "pyproject.toml" in summary["files"]

    def test_scaffold_renders_template_vars(self, tmp_path):
        target = tmp_path / "out"
        spec = ScaffoldSpec(name="My Tool", template="python-fastmcp", tools=["a"])
        Scaffolder().scaffold(target, spec)
        text = (target / "pyproject.toml").read_text(encoding="utf-8")
        assert "my-tool" in text
        assert (
            "My Tool" not in text
        )  # render_to_string only handles {{ var }} — README is the only multi-line Jinja, so the name "My Tool" only appears in README via Jinja
        # README has the human name.
        readme = (target / "README.md").read_text(encoding="utf-8")
        assert "My Tool" in readme

    def test_scaffold_rejects_non_empty(self, tmp_path):
        target = tmp_path / "out"
        target.mkdir()
        (target / "preexisting.txt").write_text("hi")
        spec = ScaffoldSpec(name="Demo", template="python-fastmcp")
        with pytest.raises(FileExistsError):
            Scaffolder().scaffold(target, spec)

    def test_scaffold_creates_src_and_tests(self, tmp_path):
        target = tmp_path / "out"
        spec = ScaffoldSpec(name="Demo", template="python-fastmcp", tools=["ping"])
        Scaffolder().scaffold(target, spec)
        assert (target / "src" / "demo" / "__init__.py").is_file()
        assert (target / "src" / "demo" / "mcp_server.py").is_file()
        assert (target / "tests" / "test_mcp_server.py").is_file()

    def test_scaffold_renders_pkg_name(self, tmp_path):
        """The `{{ pkg }}` path placeholder must be replaced."""
        target = tmp_path / "out"
        spec = ScaffoldSpec(
            name="My Cool Tool", template="python-fastmcp", tools=["ping"]
        )
        Scaffolder().scaffold(target, spec)
        # 'My Cool Tool' → 'my_cool_tool' (package form).
        assert (target / "src" / "my_cool_tool" / "mcp_server.py").is_file()

    def test_scaffold_node_template(self, tmp_path):
        target = tmp_path / "out"
        spec = ScaffoldSpec(name="Node Tool", template="node-mcp", tools=["ping"])
        Scaffolder().scaffold(target, spec)
        assert (target / "package.json").is_file()
        assert (target / "src" / "index.js").is_file()

    def test_scaffold_go_template(self, tmp_path):
        target = tmp_path / "out"
        spec = ScaffoldSpec(name="Go Tool", template="go-mcp", tools=["ping"])
        Scaffolder().scaffold(target, spec)
        assert (target / "go.mod").is_file()
        assert (target / "main.go").is_file()

    def test_scaffold_returns_files_list(self, tmp_path):
        target = tmp_path / "out"
        spec = ScaffoldSpec(name="X", template="python-fastmcp", tools=["a", "b"])
        summary = Scaffolder().scaffold(target, spec)
        assert isinstance(summary["files"], list)
        assert len(summary["files"]) > 5  # pyproject, README, SKILL, src/, tests/, etc.

    def test_summary_serializable(self, tmp_path):
        """The summary dict must be JSON-serializable."""
        import json

        target = tmp_path / "out"
        spec = ScaffoldSpec(name="X", template="python-fastmcp", tools=["ping"])
        summary = Scaffolder().scaffold(target, spec)
        # Should not raise.
        json.dumps(summary)
