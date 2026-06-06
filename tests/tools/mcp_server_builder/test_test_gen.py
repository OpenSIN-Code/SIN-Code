"""
Purpose: Test the TestGenerator.
Docs: test_test_gen.doc.md
"""

import pytest

from sin_code_bundle.tools.mcp_server_builder.test_gen import TestGenerator, _empty_value, _sample_value


class TestSampleValue:
    def test_str(self):
        assert _sample_value("str") == '"x"'

    def test_int(self):
        assert _sample_value("int") == "1"

    def test_list(self):
        assert _sample_value("list") == '["a", "b"]'
        assert _sample_value("list[str]") == '["a", "b"]'

    def test_dict(self):
        assert _sample_value("dict") == '{"key": "val"}'


class TestEmptyValue:
    def test_str(self):
        assert _empty_value("str") == '""'

    def test_int(self):
        assert _empty_value("int") == "0"

    def test_bool(self):
        assert _empty_value("bool") == "False"

    def test_list(self):
        assert _empty_value("list") == "[]"


class TestTestGenerator:
    def _write_minimal_server(self, path):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('test')\n"
            "\n"
            "@mcp.tool()\n"
            "def do_x(input: str, count: int = 0) -> str:\n"
            '    """X tool."""\n'
            '    return f"{input}:{count}"\n',
            encoding="utf-8",
        )

    def test_extract_params(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        self._write_minimal_server(server)
        gen = TestGenerator()
        text = server.read_text(encoding="utf-8")
        params = gen._extract_params(text, "do_x")
        assert len(params) == 2
        names = [p[0] for p in params]
        types = [p[1] for p in params]
        assert "input" in names
        assert "count" in names
        assert "str" in types
        assert "int" in types

    def test_extract_params_unknown_tool(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        self._write_minimal_server(server)
        text = server.read_text(encoding="utf-8")
        params = TestGenerator()._extract_params(text, "unknown")
        assert params == []

    def test_generate_basic(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        self._write_minimal_server(server)
        code = TestGenerator().generate(server, "do_x")
        assert "def test_do_x" in code
        assert "smoke test" in code.lower()
        assert "isinstance(data, dict)" in code

    def test_generate_includes_parametrized(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        self._write_minimal_server(server)
        code = TestGenerator().generate(server, "do_x")
        assert "parametrize" in code
        # Two params → two rows.
        assert code.count("(") >= 2

    def test_generate_includes_error_test(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        self._write_minimal_server(server)
        code = TestGenerator().generate(server, "do_x")
        assert "error" in code.lower()

    def test_generate_appends_to_file(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        out = tmp_path / "test_mcp_server.py"
        self._write_minimal_server(server)
        TestGenerator().generate(server, "do_x", output_path=out)
        assert out.is_file()
        text = out.read_text(encoding="utf-8")
        assert "def test_do_x" in text

    def test_generate_missing_server_raises(self, tmp_path):
        with pytest.raises(FileNotFoundError):
            TestGenerator().generate(tmp_path / "missing.py", "do_x")
