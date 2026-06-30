# SPDX-License-Identifier: MIT
"""RTK sub-commands — extracted from cli.py."""
from __future__ import annotations

import json

import typer

from sin_code_bundle.cli_app import rtk_app


@rtk_app.command("doctor")
def rtk_doctor():
    """Check whether the RTK binary is installed."""
    from sin_code_bundle import rtk

    typer.echo(json.dumps(rtk.doctor(), indent=2))


@rtk_app.command("setup")
def rtk_setup(
    agents: str = typer.Option(
        "opencode,codex,hermes",
        help="Comma-separated agents to wire (opencode,codex,hermes).",
    ),
):
    """Run `rtk init` for each coder agent (token-saving command interception)."""
    from sin_code_bundle import rtk

    chosen = [a.strip() for a in agents.split(",") if a.strip()]
    try:
        done = rtk.setup_agents(chosen)
    except rtk.RtkError as exc:
        typer.echo(f"[RTK] {exc}", err=True)
        raise typer.Exit(code=1)
    for agent, cmd in done.items():
        typer.echo(f"[RTK] wired {agent} via `{cmd}`")
    typer.echo("[RTK] Agents now route shell commands through RTK (60-90% fewer tokens).")


@rtk_app.command("gain")
def rtk_gain():
    """Show RTK token-savings statistics (JSON)."""
    from sin_code_bundle import rtk

    try:
        typer.echo(json.dumps(rtk.gain(), indent=2))
    except rtk.RtkError as exc:
        typer.echo(f"[RTK] {exc}", err=True)
        raise typer.Exit(code=1)
