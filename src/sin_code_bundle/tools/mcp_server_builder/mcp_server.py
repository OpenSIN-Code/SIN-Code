"""
Purpose: FastMCP server exposing 8 meta-tools for building MCP servers.
Docs: mcp_server.doc.md
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from fastmcp import FastMCP

from .auditor import Auditor
from .publisher import Publisher
from .registrar import Registrar, build_local_entry
from .scaffolder import Scaffolder, ScaffoldSpec
from .templates import list_templates
from .test_gen import TestGenerator
from .tool_adder import ToolAdder, ToolSpec
from .validator import Validator

mcp = FastMCP("sin_mcp_server_builder")

# ── Constants ──────────────────────────────────────
DEFAULT_TARGET = "./out"


def _json(payload: Any) -> str:
    """Serialize `payload` as JSON for the MCP wire protocol."""
    return json.dumps(payload, indent=2, default=str)


# ── Tools ──────────────────────────────────────


@mcp.tool()
def mcp_scaffold(
    name: str,
    description: str,
    target: str = DEFAULT_TARGET,
    tools: str = "ping",
    template: str = "python-fastmcp",
) -> str:
    """
    Scaffold a new MCP server from a spec.

    Args:
        name: Human-readable name (e.g. "My Cool Tool").
        description: One-line description for README/pyproject.
        target: Output directory (must be empty).
        tools: Comma-separated tool names (default: "ping").
        template: One of `python-fastmcp`, `node-mcp`, `go-mcp`.

    Returns:
        JSON with `target`, `template`, `files`, `tools`.
    """
    spec = ScaffoldSpec(
        name=name,
        description=description,
        tools=[t.strip() for t in tools.split(",") if t.strip()],
        template=template,
    )
    summary = Scaffolder().scaffold(target, spec)
    return _json(summary)


@mcp.tool()
def mcp_template_list() -> str:
    """
    List available MCP server templates.

    Returns:
        JSON array of `{name, language, framework, entry_point, ...}`.
    """
    return _json(list_templates())


@mcp.tool()
def mcp_tool_add(
    server_path: str,
    tool_name: str,
    description: str,
    params: str = "[]",
    body: str = 'result = {"ok": True}',
) -> str:
    """
    Add a new tool to an existing MCP server.

    Args:
        server_path: Path to the `mcp_server.py` file.
        tool_name: Snake-case tool name (valid Python identifier).
        description: One-line docstring for the tool.
        params: JSON list of [name, type, default] tuples.
        body: Python statements for the tool body.

    Returns:
        JSON with the rendered tool source code.
    """
    param_list = json.loads(params) if params.strip() else []
    spec = ToolSpec(
        name=tool_name,
        description=description,
        params=[(p[0], p[1], p[2] if len(p) > 2 else "") for p in param_list],
        body=body,
    )
    rendered = ToolAdder().add_to_python(Path(server_path), spec)
    return _json({"tool": tool_name, "source": rendered})


@mcp.tool()
def mcp_tool_test(
    server_path: str,
    tool_name: str,
    output_path: str = "",
) -> str:
    """
    Generate pytest tests for an MCP tool.

    Args:
        server_path: Path to the `mcp_server.py` file.
        tool_name: Name of the tool to test.
        output_path: Optional path to append the generated tests to.

    Returns:
        JSON with the generated test code and output path.
    """
    out = Path(output_path) if output_path else None
    code = TestGenerator().generate(Path(server_path), tool_name, out)
    return _json({"tool": tool_name, "code": code, "output_path": output_path or None})


@mcp.tool()
def mcp_register(
    name: str,
    command: str,
    enabled: bool = True,
    env: str = "{}",
    config_path: str = "",
) -> str:
    """
    Register a new MCP server in `opencode.json`.

    Args:
        name: Server name (used as the mcp section key).
        command: Space-separated command + args, e.g. "uvx my-tool-mcp".
        enabled: Whether the server starts enabled.
        env: JSON object of env vars to set.
        config_path: Optional path to opencode.json (default: auto-discover).

    Returns:
        JSON with the resolved config path and the registered entry.
    """
    cmd_list = command.split() if command else []
    env_dict = json.loads(env) if env.strip() else {}
    entry = build_local_entry(name, cmd_list, env_dict)
    entry.enabled = enabled
    registrar = Registrar(config_path=Path(config_path) if config_path else None)
    path = registrar.register(entry)
    return _json(
        {
            "config_path": str(path),
            "entry": entry.to_json(),
        }
    )


@mcp.tool()
def mcp_validate(
    project_dir: str,
) -> str:
    """
    Validate an MCP server (tools, type hints, docstrings, CoDocs).

    Args:
        project_dir: Path to the project root.

    Returns:
        JSON with `ok`, `tools`, `errors`, `warnings`, `info`.
    """
    result = Validator().validate(Path(project_dir))
    return _json(result.to_dict())


@mcp.tool()
def mcp_publish(
    project_dir: str,
    template: str = "python-fastmcp",
    test: bool = False,
    dry_run: bool = True,
    registry: str = "",
) -> str:
    """
    Publish an MCP server to PyPI (Python) or npm (Node).

    Args:
        project_dir: Path to the project root.
        template: One of `python-fastmcp`, `node-mcp`, `go-mcp`.
        test: If True, publish to TestPyPI (Python only).
        dry_run: If True, do not actually upload (default: True for safety).
        registry: Optional npm registry URL (Node only).

    Returns:
        JSON with `ok`, `package`, `version`, `target`, `output`, `error`.
    """
    kwargs: dict[str, Any] = {"test": test}
    if registry:
        kwargs["registry"] = registry
    pub = Publisher(dry_run=dry_run)
    result = pub.publish(Path(project_dir), template=template, **kwargs)
    return _json(result.to_dict())


@mcp.tool()
def mcp_audit(
    project_dir: str,
    grade: str = "B",
    profile: str = "QUICK",
) -> str:
    """
    Run a ceo-audit (47 quality gates) on a new MCP server.

    Args:
        project_dir: Path to the project root.
        grade: Minimum acceptable grade (default: B).
        profile: Audit profile — `QUICK` or `FULL` (default: QUICK).

    Returns:
        JSON with `ok`, `project`, `grade`, `score`, `gates_passed`,
        `gates_total`, `findings_count`, `findings`.
    """
    auditor = Auditor(grade=grade, profile=profile)
    report = auditor.audit(Path(project_dir))
    return _json(report.to_dict())


def main() -> None:
    """Entry point for `sin-mcp-server-builder-mcp`."""
    mcp.run()


if __name__ == "__main__":
    main()
