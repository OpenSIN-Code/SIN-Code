# SPDX-License-Identifier: MIT
"""Hashline anchor patching sub-commands — extracted from cli.py."""

from __future__ import annotations

import json
from pathlib import Path

import typer

from sin_code_bundle.cli_app import hashline_app


@hashline_app.command("patch")
def hashline_patch(
    file: Path = typer.Argument(..., help="File to patch"),
    old: str = typer.Option(..., "--old", help="Old content to replace"),
    new: str = typer.Option(..., "--new", help="New content"),
    intent: str = typer.Option("", "--intent", help="Intent description"),
    apply: bool = typer.Option(False, "--apply", help="Apply the patch immediately"),
    json_out: bool = typer.Option(False, "--json", help="JSON output"),
):
    """Create a hashline-anchored patch (and optionally apply it)."""
    from sin_code_bundle.hashline import SINHashlinePatch

    patcher = SINHashlinePatch()
    patch = patcher.create_semantic_patch(file, old, new, intent or None)
    if patch is None:
        typer.echo(f"ERROR: Could not find anchor for old content in {file}", err=True)
        raise typer.Exit(code=1)
    if apply:
        success, msg = patcher.apply_semantic_patch(patch)
        result = {"patch": patch, "applied": success, "message": msg}
    else:
        result = {"patch": patch, "applied": False, "message": "Use --apply to write"}
    if json_out:
        typer.echo(json.dumps(result, indent=2))
    else:
        typer.echo(f"Patch: anchor_line={patch['anchor_line']}, hash={patch['anchor_hash'][:8]}")
        typer.echo(f"Status: {result['message']}")


@hashline_app.command("validate")
def hashline_validate(
    file: Path = typer.Argument(..., help="File to validate against"),
    patch_json: str = typer.Option(..., "--patch", help="Patch JSON (or @file)"),
):
    """Validate a patch can still be applied (anchor not stale)."""
    from sin_code_bundle.hashline import HashlineAnchor

    if patch_json.startswith("@"):
        with open(patch_json[1:]) as f:
            patch = json.load(f)
    else:
        patch = json.loads(patch_json)
    content = file.read_text()
    anchor = HashlineAnchor(content)
    is_valid, msg = anchor.validate_patch(patch)
    typer.echo(f"Valid: {is_valid} - {msg}")
    raise typer.Exit(code=0 if is_valid else 1)
