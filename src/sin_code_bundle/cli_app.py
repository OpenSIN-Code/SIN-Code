# SPDX-License-Identifier: MIT
"""Shared Typer app instances for the SIN-Code CLI.

All sub-apps are defined here so that cli_*.py modules can import them
without creating circular dependencies. cli.py imports all cli_*.py
modules to register commands via decorators.
"""
from __future__ import annotations

import typer

app = typer.Typer(help="SIN-Code Bundle - Unified SOTA Agent-Engineering Stack")

gitnexus_app = typer.Typer(help="GitNexus bridge - mandatory graph context for coder agents.")
app.add_typer(gitnexus_app, name="gitnexus")

markitdown_app = typer.Typer(
    help="MarkItDown bridge - document->Markdown context for coder agents."
)
app.add_typer(markitdown_app, name="markitdown")

rtk_app = typer.Typer(help="RTK bridge - token-saving command proxy for coder agents.")
app.add_typer(rtk_app, name="rtk")

codocs_app = typer.Typer(help="CoDocs - co-located docs standard (.doc.md companions).")
app.add_typer(codocs_app, name="codocs")

sin_code_app = typer.Typer(
    help="SIN-Code Go Tools - discovery, execution, mapping, grasping, scouting, harvesting, orchestration."
)
app.add_typer(sin_code_app, name="sin-code")

security_app = typer.Typer(
    help="SIN-Code Security Bundle - 8 tools (secrets, sast, sca, sbom, container, iac, license, dast) + 8 compliance frameworks (cis, nist, soc2, iso27001, gdpr, hipaa, pci, owasp)."
)
app.add_typer(security_app, name="security")

sckg_app = typer.Typer(help="SCKG - Semantic Codebase Knowledge Graph")
app.add_typer(sckg_app, name="sckg")

ceo_audit_app = typer.Typer(
    help="CEO Audit - SOTA repo review (delegates to the opencode skill)."
)
app.add_typer(ceo_audit_app, name="ceo-audit")

pocock_app = typer.Typer(
    help="Pocock workflow helpers - grill-me, tdd-enforcer, dag-kanban, cleanup, safe-start."
)
app.add_typer(pocock_app, name="pocock")

browser_app = typer.Typer(
    help="SIN-Browser-Tools bridge - browser automation, screenshots, DevTools."
)
app.add_typer(browser_app, name="browser")

vfs_app = typer.Typer(
    help="VFS - Virtual File System for transparent remote/local file access."
)
app.add_typer(vfs_app, name="vfs")

hashline_app = typer.Typer(
    help="Hashline - LINE:HASH anchored editing for sin_edit."
)
app.add_typer(hashline_app, name="hashline")

ast_app = typer.Typer(help="AST-based code editing (requires tree-sitter)")
app.add_typer(ast_app, name="ast")

update_app = typer.Typer(
    help="Update SIN-Code stack components (Python packages, Go binaries, skills)."
)
app.add_typer(update_app, name="update")

config_app = typer.Typer(
    help="View and manage sin-code configuration (user + project merged view)."
)
app.add_typer(config_app, name="config")

lint_app = typer.Typer(help="Lint code with popular linters (ruff, flake8, mypy, eslint, etc.).")
app.add_typer(lint_app, name="lint")

docs_app = typer.Typer(help="Documentation helpers — generate README, API docs, check coverage.")
app.add_typer(docs_app, name="docs")

git_app = typer.Typer(help="Git workflow helpers — status, commit, push, clean branches.")
app.add_typer(git_app, name="git")
