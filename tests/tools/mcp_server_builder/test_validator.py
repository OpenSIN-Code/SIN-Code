# SPDX-License-Identifier: MIT
"""
Purpose: Test the Validator.
Docs: test_validator.doc.md
"""



from sin_code_bundle.tools.mcp_server_builder.validator import Validator


class TestValidator:
    def _make_project(self, tmp_path, *, with_doc=True, with_tests=True):
        """Create a minimal project with one valid tool."""
        src = tmp_path / "src" / "demo"
        src.mkdir(parents=True)
        (src / "__init__.py").write_text('"""Demo."""\n')
        (src / "mcp_server.py").write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('demo')\n"
            "\n"
            "@mcp.tool()\n"
            "def ping(x: str = 'hi') -> str:\n"
            '    """Ping tool."""\n'
            "    return x\n",
            encoding="utf-8",
        )
        if with_doc:
            (src / "mcp_server.doc.md").write_text("# docs\n")
        if with_tests:
            t = tmp_path / "tests"
            t.mkdir()
            (t / "test_x.py").write_text("def test_x(): pass\n")
        (tmp_path / "pyproject.toml").write_text("[project]\nname='demo'\n")
        return tmp_path

    def test_valid_project(self, tmp_path):
        result = Validator().validate(self._make_project(tmp_path))
        assert result.ok
        assert "mcp_server.ping" in result.tools

    def test_missing_docstring_is_error(self, tmp_path):
        proj = self._make_project(tmp_path)
        (proj / "src" / "demo" / "mcp_server.py").write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('demo')\n"
            "\n"
            "@mcp.tool()\n"
            "def bad(x: str) -> str:\n"
            "    return x\n",  # no docstring
            encoding="utf-8",
        )
        result = Validator().validate(proj)
        assert not result.ok
        assert any(i.code == "MISSING_DOCSTRING" for i in result.issues)

    def test_missing_type_hint_is_warning(self, tmp_path):
        proj = self._make_project(tmp_path)
        (proj / "src" / "demo" / "mcp_server.py").write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('demo')\n"
            "\n"
            "@mcp.tool()\n"
            "def x(y) -> str:\n"
            '    """X."""\n'
            "    return y\n",
            encoding="utf-8",
        )
        result = Validator().validate(proj)
        assert result.ok  # warnings only
        assert any(i.code == "MISSING_TYPE_HINT" for i in result.issues)

    def test_missing_return_hint_is_warning(self, tmp_path):
        proj = self._make_project(tmp_path)
        (proj / "src" / "demo" / "mcp_server.py").write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('demo')\n"
            "\n"
            "@mcp.tool()\n"
            "def x(y: str):\n"
            '    """X."""\n'
            "    return y\n",
            encoding="utf-8",
        )
        result = Validator().validate(proj)
        assert result.ok
        assert any(i.code == "MISSING_RETURN_HINT" for i in result.issues)

    def test_syntax_error(self, tmp_path):
        proj = self._make_project(tmp_path)
        (proj / "src" / "demo" / "mcp_server.py").write_text(
            "def broken(:\n",
            encoding="utf-8",
        )
        result = Validator().validate(proj)
        assert not result.ok
        assert any(i.code == "SYNTAX_ERROR" for i in result.issues)

    def test_missing_codoc(self, tmp_path):
        proj = self._make_project(tmp_path, with_doc=False)
        result = Validator().validate(proj)
        assert any(i.code == "MISSING_CODOC" for i in result.issues)

    def test_no_tests_dir_is_error(self, tmp_path):
        proj = self._make_project(tmp_path, with_tests=False)
        result = Validator().validate(proj)
        assert not result.ok
        assert any(i.code == "NO_TESTS_DIR" for i in result.issues)

    def test_no_tests_files_is_warning(self, tmp_path):
        proj = self._make_project(tmp_path)
        (proj / "tests" / "test_x.py").unlink()
        (proj / "tests" / "README.md").write_text("hi")
        result = Validator().validate(proj)
        assert any(i.code == "NO_TESTS" for i in result.issues)

    def test_missing_root(self):
        result = Validator().validate("/nonexistent/path/xyz")
        assert not result.ok
        assert any(i.code == "ROOT_MISSING" for i in result.issues)

    def test_no_pyproject_is_info(self, tmp_path):
        proj = self._make_project(tmp_path)
        (proj / "pyproject.toml").unlink()
        result = Validator().validate(proj)
        assert any(i.code == "NO_PYPROJECT" for i in result.issues)

    def test_to_dict_shape(self, tmp_path):
        proj = self._make_project(tmp_path)
        data = Validator().validate(proj).to_dict()
        assert "ok" in data
        assert "tools" in data
        assert "errors" in data
        assert "warnings" in data
        assert "info" in data

    def test_async_tool_decorator_supported(self, tmp_path):
        """Validator should detect `@mcp.tool()` on `async def` functions too."""
        proj = self._make_project(tmp_path)
        (proj / "src" / "demo" / "mcp_server.py").write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('demo')\n"
            "@mcp.tool()\n"
            "async def aping() -> str:\n"
            '    """A."""\n'
            '    return "{}"\n',
            encoding="utf-8",
        )
        result = Validator().validate(proj)
        assert any("aping" in t for t in result.tools)

    def test_non_tool_functions_ignored(self, tmp_path):
        """Functions without `@mcp.tool()` must be ignored."""
        proj = self._make_project(tmp_path)
        (proj / "src" / "demo" / "mcp_server.py").write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('demo')\n"
            "def helper():\n"
            "    return 1\n"
            "@mcp.tool()\n"
            "def real(x: str) -> str:\n"
            '    """R."""\n'
            "    return x\n",
            encoding="utf-8",
        )
        result = Validator().validate(proj)
        assert any("real" in t for t in result.tools)
        assert not any("helper" in t for t in result.tools)

    def test_decorator_with_kwargs(self, tmp_path):
        """`@mcp.tool(name='x')` should also be detected."""
        proj = self._make_project(tmp_path)
        (proj / "src" / "demo" / "mcp_server.py").write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('demo')\n"
            "@mcp.tool(name='renamed')\n"
            "def internal(x: str) -> str:\n"
            '    """R."""\n'
            "    return x\n",
            encoding="utf-8",
        )
        result = Validator().validate(proj)
        assert any("internal" in t for t in result.tools)
