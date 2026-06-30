# SPDX-License-Identifier: MIT
"""VFS sub-commands — extracted from cli.py."""
from __future__ import annotations

import json
from pathlib import Path

import typer

from sin_code_bundle.cli_app import vfs_app


@vfs_app.command("resolve")
def vfs_resolve(
    uri: str = typer.Argument(..., help="URI to resolve (e.g., sckg://module/auth/dependencies)"),
    repo: str = typer.Option(".", "--repo", help="Repo root"),
    json_out: bool = typer.Option(False, "--json", help="JSON output"),
):
    """Resolve a SIN URI scheme to structured content."""
    from sin_code_bundle.vfs import SINVirtualFS

    vfs = SINVirtualFS(Path(repo))
    result = vfs.resolve(uri)
    typer.echo(json.dumps(result, indent=2))


@vfs_app.command("schemes")
def vfs_schemes():
    """List all available URI schemes."""
    from sin_code_bundle.vfs import URI_SCHEMES

    typer.echo("Available URI schemes:")
    for scheme, desc in URI_SCHEMES.items():
        typer.echo(f"  {scheme}://  {desc}")


@vfs_app.command("status")
def vfs_status():
    """Check which SIN subsystems are available for VFS resolution."""
    from sin_code_bundle.vfs import URI_SCHEMES

    typer.echo("VFS backend status:")
    module_map = {
        "sckg": "sin_code_sckg",
        "poc": "sin_code_poc",
        "ibd": "sin_code_ibd",
        "adw": "sin_code_adw",
        "efsm": "sin_code_efsm",
        "oracle": "sin_code_oracle",
    }
    for scheme in URI_SCHEMES:
        if scheme == "conflict":
            typer.echo(f"  {scheme:8s}  OK (git-based)")
            continue
        try:
            __import__(module_map[scheme])
            typer.echo(f"  {scheme:8s}  OK")
        except ImportError:
            typer.echo(f"  {scheme:8s}  NOT INSTALLED")
