"""
Purpose: Test the ToolAdder and ToolSpec.
Docs: test_tool_adder.doc.md
"""

import pytest

from sin_code_bundle.tools.mcp_server_builder.tool_adder import ToolAdder, ToolSpec, _sample_value


class TestSampleValue:
    def test_string(self):
        assert _sample_value("str") == '"x"'
        assert _sample_value("string") == '"x"'

    def test_int(self):
        assert _sample_value("int") == "1"
        assert _sample_value("integer") == "1"

    def test_bool(self):
        assert _sample_value("bool") == "True"

    def test_unknown_defaults_to_string(self):
        assert _sample_value("weird_type") == '"x"'


class TestToolSpec:
    def test_valid(self):
        spec = ToolSpec(name="do_x", description="X")
        assert spec.name == "do_x"
        assert spec.body == 'result = {"ok": True}'

    def test_invalid_name_raises(self):
        with pytest.raises(ValueError):
            ToolSpec(name="1bad", description="x")

    def test_with_params(self):
        spec = ToolSpec(
            name="do_y",
            description="Y",
            params=[("input", "str", '""'), ("count", "int", "0")],
            body="result = {}",
        )
        assert spec.params == [("input", "str", '""'), ("count", "int", "0")]


class TestToolAdder:
    def _write_minimal_server(self, path):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('test')\n"
            "\n"
            "@mcp.tool()\n"
            "def existing_tool(x: str) -> str:\n"
            '    """Existing."""\n'
            "    return x\n",
            encoding="utf-8",
        )

    def test_render_python_tool_basic(self):
        spec = ToolSpec(name="do_x", description="Perform X.")
        rendered = ToolAdder().render_python_tool(spec)
        assert "def do_x(" in rendered
        assert "@mcp.tool()" in rendered
        assert "Perform X." in rendered

    def test_render_python_tool_with_params(self):
        spec = ToolSpec(
            name="do_y",
            description="Y.",
            params=[("input", "str", '""')],
        )
        rendered = ToolAdder().render_python_tool(spec)
        assert "input: str" in rendered
        assert "input: str parameter." in rendered

    def test_add_to_python_appends_tool(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        self._write_minimal_server(server)
        spec = ToolSpec(name="do_x", description="X.")
        new = ToolAdder().add_to_python(server, spec)
        assert "def do_x(" in new
        # Re-read to confirm persisted.
        text = server.read_text(encoding="utf-8")
        assert "def do_x(" in text
        assert "def existing_tool" in text  # original preserved

    def test_add_to_python_missing_file_raises(self, tmp_path):
        with pytest.raises(FileNotFoundError):
            ToolAdder().add_to_python(
                tmp_path / "missing.py", ToolSpec(name="x", description="x")
            )

    def test_add_test_appends(self, tmp_path):
        test = tmp_path / "test_mcp_server.py"
        test.write_text('"""Header."""\n', encoding="utf-8")
        spec = ToolSpec(name="do_x", description="X.")
        new = ToolAdder().add_test(test, spec)
        assert "test_do_x" in new
        text = test.read_text(encoding="utf-8")
        assert "test_do_x" in text
