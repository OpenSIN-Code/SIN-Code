# SPDX-License-Identifier: MIT
"""CoDocs sub-commands — extracted from cli.py."""
from __future__ import annotations

import json
import shutil
from pathlib import Path

import typer

from sin_code_bundle.cli_app import codocs_app

_EXCLUDE = {"venv", ".venv", "node_modules", ".git", "__pycache__"}


@codocs_app.command("check")
def codocs_check(
    root: str = typer.Argument(".", help="Repository root to scan"),
    json_out: bool = typer.Option(False, "--json", help="Emit machine-readable JSON"),
):
    """Verify every `# Docs: x.doc.md` reference points to an existing file."""
    from sin_code_bundle import codocs

    broken = codocs.find_broken(root, exclude=set(_EXCLUDE))
    if json_out:
        typer.echo(json.dumps([ref.to_dict() for ref in broken], indent=2))
    else:
        if not broken:
            typer.echo("[CODOCS] OK - no broken .doc.md references.")
        else:
            for ref in broken:
                typer.echo(f"[CODOCS] MISSING: {ref.source} -> {ref.doc}")
            typer.echo(f"[CODOCS] {len(broken)} broken reference(s).")
    if broken:
        raise typer.Exit(code=1)


@codocs_app.command("check-inline")
def codocs_check_inline(
    root: str = typer.Argument(".", help="Repository root to scan"),
    json_out: bool = typer.Option(False, "--json", help="Emit machine-readable JSON"),
):
    """Check that code files have proper inline docs (Purpose header, etc.)."""
    from sin_code_bundle import codocs

    issues = codocs.check_inline_docs(root, exclude=set(_EXCLUDE))
    if json_out:
        typer.echo(codocs._check_inline_docs_json(root, exclude=set(_EXCLUDE)))
    else:
        if not issues:
            typer.echo("[CODOCS] OK - all files have Purpose header.")
        else:
            for issue in issues:
                typer.echo(f"[CODOCS] {issue.kind}: {issue.path} - {issue.detail}")
            typer.echo(f"[CODOCS] {len(issues)} inline doc issue(s).")
    if issues:
        raise typer.Exit(code=1)


@codocs_app.command("list")
def codocs_list(root: str = typer.Argument(".", help="Repository root to scan")):
    """List all discovered CoDocs references and whether they resolve."""
    from sin_code_bundle import codocs

    refs = codocs.scan(root, exclude=set(_EXCLUDE))
    if not refs:
        typer.echo("[CODOCS] No `Docs:` references found.")
        return
    for ref in refs:
        mark = "ok" if ref.exists else "MISSING"
        typer.echo(f"[{mark}] {ref.source} -> {ref.doc}")


@codocs_app.command("install-skill")
def codocs_install_skill(
    agent: str = typer.Option(
        "all", help="Which agent skill dir to install into: hermes | opencode | all"
    ),
):
    """Install the CoDocs skill into the local agent skill directory."""
    skill_src = Path(__file__).parent / "data" / "codocs" / "SKILL.md"
    if not skill_src.is_file():
        # Fallback to the repo-level skills/ dir (editable installs).
        skill_src = Path(__file__).resolve().parents[2] / "skills" / "sin-codocs" / "SKILL.md"
    if not skill_src.is_file():
        typer.echo("[CODOCS] Skill file not found in package.", err=True)
        raise typer.Exit(code=1)

    targets = {
        "hermes": Path.home() / ".hermes" / "skills" / "sin-codocs",
        "opencode": Path.home() / ".config" / "opencode" / "skills" / "sin-codocs",
    }
    chosen = targets.keys() if agent == "all" else [agent]
    for name in chosen:
        if name not in targets:
            typer.echo(f"[CODOCS] Unknown agent: {name}", err=True)
            raise typer.Exit(code=1)
        dest_dir = targets[name]
        dest_dir.mkdir(parents=True, exist_ok=True)
        shutil.copy2(skill_src, dest_dir / "SKILL.md")
        typer.echo(f"[CODOCS] Installed skill -> {dest_dir / 'SKILL.md'}")
