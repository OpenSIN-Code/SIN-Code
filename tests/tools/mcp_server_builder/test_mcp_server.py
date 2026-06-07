# SPDX-License-Identifier: MIT
"""
Purpose: Test the FastMCP server entry points.
Docs: test_mcp_server.doc.md
"""

import json

from sin_code_bundle.tools.mcp_server_builder.mcp_server import (
    mcp_audit,
    mcp_publish,
    mcp_register,
    mcp_scaffold,
    mcp_template_list,
    mcp_tool_add,
    mcp_tool_test,
    mcp_validate,
)


class TestMcpTemplateList:
    def test_returns_three(self):
        result = json.loads(mcp_template_list())
        assert len(result) == 3
        names = {t["name"] for t in result}
        assert names == {"python-fastmcp", "node-mcp", "go-mcp"}


class TestMcpScaffold:
    def test_basic(self, tmp_path):
        target = tmp_path / "out"
        result = json.loads(
            mcp_scaffold(
                name="Demo",
                description="Test scaffold",
                target=str(target),
                tools="do_x,do_y",
                template="python-fastmcp",
            )
        )
        assert result["template"] == "python-fastmcp"
        assert result["tools"] == ["do_x", "do_y"]
        assert (target / "pyproject.toml").is_file()

    def test_default_target(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        mcp_scaffold(
            name="X",
            description="X",
            target="./out",
        )
        assert (tmp_path / "out").is_dir()


class TestMcpToolAdd:
    def test_appends(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        server.write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('test')\n\n"
            "@mcp.tool()\n"
            "def existing(x: str) -> str:\n"
            '    """Existing."""\n'
            "    return x\n",
            encoding="utf-8",
        )
        result = json.loads(
            mcp_tool_add(
                server_path=str(server),
                tool_name="do_new",
                description="New tool.",
            )
        )
        assert result["tool"] == "do_new"
        assert "def do_new(" in result["source"]
        assert "def existing" in server.read_text()


class TestMcpToolTest:
    def test_generates_tests(self, tmp_path):
        server = tmp_path / "mcp_server.py"
        server.write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('test')\n\n"
            "@mcp.tool()\n"
            "def do_x(input: str) -> str:\n"
            '    """X."""\n'
            "    return input\n",
            encoding="utf-8",
        )
        out = tmp_path / "test_mcp_server.py"
        result = json.loads(
            mcp_tool_test(
                server_path=str(server),
                tool_name="do_x",
                output_path=str(out),
            )
        )
        assert "def test_do_x" in result["code"]
        assert out.is_file()


class TestMcpRegister:
    def test_registers(self, tmp_path, monkeypatch):
        cfg = tmp_path / "opencode.json"
        monkeypatch.setenv("OPENCODE_CONFIG", str(cfg))
        # Re-import the registrar inside mcp_server to pick up the env var.
        # The MCP tool uses Registrar(config_path=None) which honors OPENCODE_CONFIG.
        result = json.loads(
            mcp_register(
                name="my-tool",
                command="uvx my-tool-mcp",
                env='{"KEY": "val"}',
            )
        )
        assert "config_path" in result
        assert result["entry"]["type"] == "local"
        assert result["entry"]["command"] == ["uvx", "my-tool-mcp"]


class TestMcpValidate:
    def test_valid_project(self, tmp_path):
        # Create a minimal valid project.
        src = tmp_path / "src" / "demo"
        src.mkdir(parents=True)
        (src / "__init__.py").write_text('"""Demo."""\n')
        (src / "mcp_server.py").write_text(
            "from fastmcp import FastMCP\n"
            "mcp = FastMCP('demo')\n"
            "@mcp.tool()\n"
            "def ping() -> str:\n"
            '    """P."""\n'
            '    return "{}"\n',
            encoding="utf-8",
        )
        (src / "mcp_server.doc.md").write_text("# d\n")
        (tmp_path / "pyproject.toml").write_text("[project]\n")
        tests = tmp_path / "tests"
        tests.mkdir()
        (tests / "test_x.py").write_text("def test_x(): pass\n")
        result = json.loads(mcp_validate(project_dir=str(tmp_path)))
        assert result["ok"] is True


class TestMcpPublish:
    def test_dry_run(self, tmp_path):
        (tmp_path / "pyproject.toml").write_text(
            "[project]\nname='x'\nversion='0.1.0'\n"
        )
        result = json.loads(
            mcp_publish(
                project_dir=str(tmp_path),
                template="python-fastmcp",
                dry_run=True,
            )
        )
        assert result["ok"] is True
        assert result["package"] == "x"


class TestMcpAudit:
    def test_degraded(self, tmp_path, monkeypatch):
        monkeypatch.setattr("shutil.which", lambda cmd: None)
        result = json.loads(mcp_audit(project_dir=str(tmp_path)))
        # ok=False because sin is missing.
        assert result["ok"] is False
        assert result["gates_total"] == 47
