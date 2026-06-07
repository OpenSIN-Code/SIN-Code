# SPDX-License-Identifier: MIT
# Purpose: Typer subcommand for `sin mcp-server <sub>` (issue #29)
# Docs: mcp_server_builder.doc.md
"""Typer subcommand exposing ``sin mcp-server ...``.

Originally the standalone ``sin-mcp-server-builder`` MCP skill; merged into
sin-code-bundle v0.9.3 (issue #29). Exposes the meta-skill surface as a
single ``sin mcp-server`` subcommand group::

    sin mcp-server scaffold <name>       # create a new MCP server directory
    sin mcp-server add-tool <name>       # add a tool to an existing mcp_server.py
    sin mcp-server test-tool <name>      # generate pytest tests for a tool
    sin mcp-server register <name>       # register an MCP server in opencode.json
    sin mcp-server validate <path>       # run CoDocs + type-hint validator
    sin mcp-server publish <path>        # build + publish to PyPI / npm
    sin mcp-server audit <path>          # run ceo-audit on the project
    sin mcp-server template-list         # list available templates
"""

from __future__ import annotations

import json
import shutil
import sys
from pathlib import Path
from typing import Optional

import typer

from .publisher import Publisher
from .registrar import McpServerEntry, Registrar, build_local_entry
from .scaffolder import ScaffoldSpec, Scaffolder
from .templates import TemplateEngine, list_templates
from .test_gen import TestGenerator
from .tool_adder import ToolAdder, ToolSpec
from .validator import Validator

# `app` becomes `sin mcp-server <sub>` after `app.add_typer(app, name="mcp-server")`.
app = typer.Typer(
    name="mcp-server",
    help="Scaffold, add tools, validate, register, publish, and audit MCP servers.",
    no_args_is_help=True,
)


def _scaffolder() -> Scaffolder:
    return Scaffolder()


def _tool_adder() -> ToolAdder:
    return ToolAdder()


def _test_gen() -> TestGenerator:
    return TestGenerator()


def _validator() -> Validator:
    return Validator()


def _publisher() -> Publisher:
    return Publisher()


def _auditor():
    from .auditor import Auditor

    return Auditor()


def _registrar(config_path: Optional[Path]) -> Registrar:
    return Registrar(config_path=config_path) if config_path else Registrar()


@app.command("scaffold")
def mcp_scaffold(
    name: str = typer.Argument(..., help="Human-readable server name."),
    description: str = typer.Option("", "--description", "-d", help="One-line description."),
    tools: str = typer.Option("ping", "--tools", help="Comma-separated tool names."),
    template: str = typer.Option("python-fastmcp", "--template", "-t", help="Template key."),
    target: Path = typer.Option(Path("./out"), "--target", help="Output directory."),
    version: str = typer.Option("0.1.0", "--version", help="Initial version."),
    author: str = typer.Option(
        "OpenSIN-Code <contact@opensincode.org>", "--author", help="Author for pyproject/package.json."
    ),
) -> None:
    """Scaffold a new MCP server from a template."""
    spec = ScaffoldSpec(
        name=name,
        description=description,
        tools=[t.strip() for t in tools.split(",") if t.strip()],
        template=template,
        author=author,
        version=version,
    )
    scaffolder = _scaffolder()
    try:
        result = scaffolder.scaffold(target, spec)
    except (FileExistsError, FileNotFoundError, ValueError) as exc:
        typer.echo(f"[FAIL] {exc}", err=True)
        raise typer.Exit(code=1)
    typer.echo(f"[OK] Scaffolded {result['template']} server at {result['target']}")
    typer.echo(f"  Tools: {', '.join(result['tools'])}")
    typer.echo(f"  Files: {len(result['files'])}")


@app.command("add-tool")
def mcp_add_tool(
    tool_name: str = typer.Argument(..., help="Snake-case tool name."),
    server_path: Path = typer.Option(
        Path("mcp_server.py"), "--server", help="Path to mcp_server.py."
    ),
    description: str = typer.Option("", "--description", "-d", help="Tool docstring."),
    params: str = typer.Option(
        "", "--params", help="JSON list of [name, type, default] tuples."
    ),
    body: str = typer.Option('result = {"ok": True}', "--body", help="Python body."),
    test_path: Optional[Path] = typer.Option(None, "--test", help="Test file to append to."),
) -> None:
    """Add a new tool to an existing mcp_server.py."""
    params_list = []
    if params:
        try:
            params_list = json.loads(params)
        except json.JSONDecodeError as exc:
            typer.echo(f"[FAIL] Invalid --params JSON: {exc}", err=True)
            raise typer.Exit(code=1)
    spec = ToolSpec(
        name=tool_name,
        description=description,
        params=[(p[0], p[1], p[2] if len(p) > 2 else "") for p in params_list],
        body=body,
    )
    adder = _tool_adder()
    try:
        adder.add_to_python(server_path, spec)
    except (FileNotFoundError, ValueError) as exc:
        typer.echo(f"[FAIL] {exc}", err=True)
        raise typer.Exit(code=1)
    if test_path is not None:
        try:
            adder.add_test(test_path, spec)
        except (FileNotFoundError, ValueError) as exc:
            typer.echo(f"[FAIL] {exc}", err=True)
            raise typer.Exit(code=1)
    typer.echo(f"[OK] Added tool `{tool_name}` to {server_path}")


@app.command("test-tool")
def mcp_test_tool(
    tool_name: str = typer.Argument(..., help="Tool name to generate tests for."),
    server_path: Path = typer.Option(
        Path("mcp_server.py"), "--server", help="Path to mcp_server.py."
    ),
    output: Optional[Path] = typer.Option(
        None, "--output", help="Append tests to this file (default: stdout)."
    ),
) -> None:
    """Generate pytest tests for an existing tool."""
    gen = _test_gen()
    try:
        code = gen.generate(server_path, tool_name, output_path=output)
    except FileNotFoundError as exc:
        typer.echo(f"[FAIL] {exc}", err=True)
        raise typer.Exit(code=1)
    if output is None:
        typer.echo(code)
    else:
        typer.echo(f"[OK] Appended tests for `{tool_name}` to {output}")


@app.command("register")
def mcp_register(
    name: str = typer.Argument(..., help="Server name (mcp.<name> in opencode.json)."),
    command: str = typer.Argument(..., help="Space-separated command (e.g. 'uvx foo-mcp')."),
    config_path: Optional[Path] = typer.Option(
        None, "--config", help="Path to opencode.json (default: auto-detect)."
    ),
    enabled: bool = typer.Option(True, "--enabled/--disabled", help="Enable in mcp section."),
) -> None:
    """Register an MCP server in opencode.json."""
    cmd_list = command.split()
    entry = McpServerEntry(name=name, type="local", command=cmd_list, enabled=enabled)
    reg = _registrar(config_path)
    path = reg.register(entry)
    typer.echo(f"[OK] Registered {name} -> {path}")


@app.command("unregister")
def mcp_unregister(
    name: str = typer.Argument(..., help="Server name to remove."),
    config_path: Optional[Path] = typer.Option(
        None, "--config", help="Path to opencode.json (default: auto-detect)."
    ),
) -> None:
    """Remove an MCP server from opencode.json."""
    reg = _registrar(config_path)
    if reg.unregister(name):
        typer.echo(f"[OK] Unregistered {name}")
    else:
        typer.echo(f"[FAIL] {name} not found in mcp section", err=True)
        raise typer.Exit(code=1)


@app.command("list-registered")
def mcp_list_registered(
    config_path: Optional[Path] = typer.Option(
        None, "--config", help="Path to opencode.json (default: auto-detect)."
    ),
) -> None:
    """List all registered MCP servers in opencode.json."""
    reg = _registrar(config_path)
    for name in reg.list_registered():
        typer.echo(f"  {name}")


@app.command("validate")
def mcp_validate(
    path: Path = typer.Argument(Path("."), help="MCP server project root."),
    json_out: bool = typer.Option(False, "--json", help="JSON output."),
) -> None:
    """Run CoDocs + type-hint validator on a project."""
    validator = _validator()
    result = validator.validate(path)
    if json_out:
        typer.echo(json.dumps(result.to_dict(), indent=2))
    else:
        if result.ok:
            typer.echo(f"[OK] {len(result.tools)} tools validated")
        else:
            typer.echo(f"[FAIL] {len(result.issues)} issue(s)", err=True)
        for issue in result.issues:
            mark = {"error": "ERR ", "warning": "WARN", "info": "INFO"}.get(issue.level, "----")
            typer.echo(f"  {mark}  [{issue.code}] {issue.message}")
    if not result.ok:
        raise typer.Exit(code=1)


@app.command("publish")
def mcp_publish(
    path: Path = typer.Argument(Path("."), help="MCP server project root."),
    template: str = typer.Option("python-fastmcp", "--template", "-t", help="python-fastmcp | node-mcp | go-mcp"),
    test: bool = typer.Option(False, "--test", help="Publish to TestPyPI instead of PyPI."),
    skip_build: bool = typer.Option(False, "--skip-build", help="Skip `python -m build`."),
    registry: str = typer.Option("https://registry.npmjs.org/", "--registry", help="npm registry."),
    tag: str = typer.Option("latest", "--tag", help="npm publish tag."),
    dry_run: bool = typer.Option(False, "--dry-run", help="Don't actually publish."),
    json_out: bool = typer.Option(False, "--json", help="JSON output."),
) -> None:
    """Build + publish the MCP server to its registry."""
    pub = Publisher(dry_run=dry_run)
    if template == "python-fastmcp":
        result = pub.publish_pypi(path, test=test, skip_build=skip_build)
    elif template == "node-mcp":
        result = pub.publish_npm(path, registry=registry, tag=tag)
    elif template == "go-mcp":
        result = pub.publish_go(path)
    else:
        typer.echo(f"[FAIL] Unknown template: {template}", err=True)
        raise typer.Exit(code=2)
    if json_out:
        typer.echo(json.dumps(result.to_dict(), indent=2))
    else:
        if result.ok:
            typer.echo(f"[OK] {result.package} {result.version} -> {result.target}")
            if result.output:
                typer.echo(result.output)
        else:
            typer.echo(f"[FAIL] {result.error or result.output}", err=True)
            raise typer.Exit(code=1)


@app.command("audit")
def mcp_audit(
    path: Path = typer.Argument(Path("."), help="MCP server project root."),
    profile: str = typer.Option("QUICK", "--profile", help="ceo-audit profile (QUICK|FULL|SECURITY|RELEASE)."),
    grade: str = typer.Option("B", "--grade", help="Minimum grade gate (A|B|C)."),
    json_out: bool = typer.Option(False, "--json", help="JSON output."),
) -> None:
    """Run ceo-audit on the MCP server project."""
    auditor = _auditor()
    auditor.profile = profile
    auditor.grade = grade
    report = auditor.audit(path)
    if json_out:
        typer.echo(json.dumps(report.to_dict(), indent=2))
    else:
        if report.ok:
            typer.echo(f"[OK] {report.project} {report.grade} ({report.gates_passed}/{report.gates_total})")
        else:
            typer.echo(f"[FAIL] {report.project} {report.grade}", err=True)
    if not report.ok:
        raise typer.Exit(code=1)


@app.command("template-list")
def mcp_template_list(
    json_out: bool = typer.Option(False, "--json", help="JSON output."),
) -> None:
    """List available templates (python-fastmcp, node-mcp, go-mcp)."""
    templates = list_templates()
    if json_out:
        typer.echo(json.dumps(templates, indent=2))
    else:
        for t in templates:
            typer.echo(f"  {t['name']:18s} language={t['language']:11s} framework={t['framework']}")
