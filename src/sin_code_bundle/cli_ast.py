# SPDX-License-Identifier: MIT
"""AST-based code editing sub-commands — extracted from cli.py."""

from __future__ import annotations

import json
from pathlib import Path

import typer

from sin_code_bundle.cli_app import ast_app


@ast_app.command("edit")
def ast_edit(
    file: Path = typer.Argument(..., help="File to edit"),
    old: str = typer.Option(..., "--old", help="Old substring"),
    new: str = typer.Option(..., "--new", help="Replacement"),
    apply: bool = typer.Option(False, "--apply", help="Apply changes immediately"),
    no_poc: bool = typer.Option(False, "--no-poc", help="Skip POC verification"),
    json_out: bool = typer.Option(False, "--json", help="JSON output"),
):
    """Propose an AST-based edit."""
    from sin_code_bundle.ast_edit import SINASTEdit

    ast = SINASTEdit()
    if not ast.is_available():
        typer.echo(
            "ERROR: tree-sitter not installed. Run: pip install tree-sitter tree-sitter-languages",
            err=True,
        )
        raise typer.Exit(code=1)
    result = ast.edit(file, old, new, verify_with_poc=not no_poc)
    if apply and result.success:
        ast.resolve(file, result.proposed_changes)
    out = result.to_dict()
    if json_out:
        typer.echo(json.dumps(out, indent=2))
    else:
        if result.success:
            typer.echo(
                f"Edit proposed: {len(result.proposed_changes)} changes, POC verified={result.poc_verified}"
            )
            if apply:
                typer.echo("Applied.")
        else:
            typer.echo(f"ERROR: {result.error}", err=True)
            raise typer.Exit(code=1)


@ast_app.command("status")
def ast_status():
    """Check if AST edit is available."""
    from sin_code_bundle.ast_edit import SINASTEdit

    ast = SINASTEdit()
    if ast.is_available():
        typer.echo(f"AST edit available. Languages: {', '.join(ast.SUPPORTED_LANGS)}")
    else:
        typer.echo("AST edit NOT available. Run: pip install tree-sitter tree-sitter-languages")
        raise typer.Exit(code=1)
