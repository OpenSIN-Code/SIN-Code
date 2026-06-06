"""
Purpose: Test the Publisher and dispatcher.
Docs: test_publisher.doc.md
"""

import json

from sin_code_bundle.tools.mcp_server_builder.publisher import Publisher


class TestPublisher:
    def _make_python_project(self, tmp_path):
        (tmp_path / "pyproject.toml").write_text(
            "[project]\n" 'name = "demo"\n' 'version = "0.3.2"\n',
            encoding="utf-8",
        )
        (tmp_path / "README.md").write_text("# demo\n")
        return tmp_path

    def _make_node_project(self, tmp_path):
        (tmp_path / "package.json").write_text(
            json.dumps({"name": "demo", "version": "1.2.3"}),
            encoding="utf-8",
        )
        return tmp_path

    def _make_go_project(self, tmp_path):
        (tmp_path / "go.mod").write_text("module github.com/x/demo\n\ngo 1.22\n")
        return tmp_path

    def test_dry_run_pypi(self, tmp_path):
        proj = self._make_python_project(tmp_path)
        result = Publisher(dry_run=True).publish_pypi(proj)
        assert result.ok
        assert result.package == "demo"
        assert result.version == "0.3.2"
        assert result.target == "pypi"
        assert "dry-run" in result.output

    def test_dry_run_pypi_test(self, tmp_path):
        proj = self._make_python_project(tmp_path)
        result = Publisher(dry_run=True).publish_pypi(proj, test=True)
        assert result.target == "pypi-test"

    def test_dry_run_npm(self, tmp_path):
        proj = self._make_node_project(tmp_path)
        result = Publisher(dry_run=True).publish_npm(proj)
        assert result.ok
        assert result.package == "demo"
        assert result.version == "1.2.3"
        assert result.target == "npm"

    def test_go_returns_guidance(self, tmp_path):
        proj = self._make_go_project(tmp_path)
        result = Publisher().publish_go(proj)
        assert not result.ok
        assert "git tag" in result.output
        assert result.target == "go"

    def test_dispatch_unknown_template(self, tmp_path):
        result = Publisher().publish(tmp_path, template="rust-mcp")
        assert not result.ok
        assert "Unknown template" in result.error

    def test_dispatch_python(self, tmp_path):
        proj = self._make_python_project(tmp_path)
        result = Publisher(dry_run=True).publish(proj, template="python-fastmcp")
        assert result.target == "pypi"

    def test_dispatch_node(self, tmp_path):
        proj = self._make_node_project(tmp_path)
        result = Publisher(dry_run=True).publish(proj, template="node-mcp")
        assert result.target == "npm"

    def test_dispatch_go(self, tmp_path):
        proj = self._make_go_project(tmp_path)
        result = Publisher().publish(proj, template="go-mcp")
        assert result.target == "go"

    def test_read_pyproject_missing(self, tmp_path):
        pub = Publisher()
        name, version = pub._read_pyproject(tmp_path)
        assert name == "unknown"
        assert version == "0.0.0"

    def test_read_package_json_missing(self, tmp_path):
        pub = Publisher()
        name, version = pub._read_package_json(tmp_path)
        assert name == "unknown"
        assert version == "0.0.0"

    def test_read_package_json_invalid(self, tmp_path):
        (tmp_path / "package.json").write_text("{not json")
        pub = Publisher()
        name, version = pub._read_package_json(tmp_path)
        assert name == "unknown"
