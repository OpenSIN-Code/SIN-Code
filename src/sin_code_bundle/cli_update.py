# SPDX-License-Identifier: MIT
"""Update sub-commands — extracted from cli.py.

Also includes the top-level ``forge`` and ``tui`` commands which were
co-located with the update group in the original cli.py.
"""
from __future__ import annotations

import json
import shutil
import subprocess

import typer

from sin_code_bundle.cli_app import app, update_app


# ── Thin binary wrappers for new SIN-Code tools (v0.10.0) ──────────────────
_NEW_TOOL_BINARIES = {
    "ibd": ("SIN-Code-Intent-Based-Diffing", "ibd"),
    "poc": ("SIN-Code-Proof-of-Correctness", "poc"),
    "sckg": ("SIN-Code-Semantic-Codebase-Knowledge-Graphs", "sckg"),
    "adw": ("SIN-Code-Architectural-Debt-Watchdogs", "adw"),
    "oracle": ("SIN-Code-Verification-Oracle", "oracle"),
    "efm": ("SIN-Code-EFM-Tool", "efm"),
    "forge": ("SIN-Code-Forge-Tool", "forge"),
}


def _forward_to_binary(name: str, repo_hint: str) -> None:
    """Forward remaining CLI args to the binary *name* if it exists on PATH."""
    import sys

    binary = shutil.which(name)
    if not binary:
        typer.echo(
            f"[SIN-BUNDLE] '{name}' binary not found in PATH. "
            f"Install: pip install -e ~/{repo_hint}",
            err=True,
        )
        raise typer.Exit(code=1)
    args = sys.argv[sys.argv.index(name) + 1 :]
    result = subprocess.run([binary, *args])
    raise typer.Exit(code=result.returncode)


@app.command()
def forge():
    """SIN-Code Forge — intelligent code generation & editing (thin wrapper around the `forge` binary)."""
    _forward_to_binary("forge", _NEW_TOOL_BINARIES["forge"][0])


@app.command()
def tui(
    fallback: bool = typer.Option(
        False,
        "--fallback",
        help="Skip the TUI and show a plain menu (used when no TTY is available).",
    ),
) -> None:
    """Launch the SIN-Code TUI (Bubbletea) — interactive menu over every `sin` subcommand.

    The TUI is a separate Go binary (sin-tui) that the Python CLI shells out to.
    Build it once with:

        go build -o ~/.local/bin/sin-tui ./cmd/sin-tui

    If the binary is missing, this command prints a short installation hint and
    exits 1 instead of crashing, so `sin tui` is always safe to call.
    """
    import sys

    if fallback or not sys.stdout.isatty():
        # Plain-text menu fallback for non-TTY environments (CI, logs, pipes).
        typer.echo("sin tui — interactive mode (fallback, no TTY detected)\n")
        for c in _TU_CATALOG:
            typer.echo(f"  {c['title']:<22}  {c['desc']}")
        typer.echo("\nRun `sin <subcommand> --help` for details.")
        return

    binary = shutil.which("sin-tui")
    if not binary:
        typer.echo(
            "[SIN-BUNDLE] 'sin-tui' binary not found in PATH.\n"
            "Build it from this repo:\n"
            "  go build -o ~/.local/bin/sin-tui ./cmd/sin-tui\n"
            "Or download a prebuilt binary from the SIN-Code release page.",
            err=True,
        )
        raise typer.Exit(code=1)

    # Hand off the terminal to the Go binary (it uses alt-screen + mouse).
    result = subprocess.run([binary, *sys.argv[sys.argv.index("tui") + 1 :]])
    raise typer.Exit(code=result.returncode)


# Catalog used by the non-TTY fallback in `tui`. Keep in sync with
# internal/tui/commands.go (the Go side is the source of truth at runtime).
_TU_CATALOG = [
    {"title": "sin code", "desc": "Unified coding workflow hub"},
    {"title": "sin code full <path>", "desc": "Run preflight → codocs → debt → sckg"},
    {"title": "sin sckg", "desc": "Semantic codebase knowledge graph"},
    {"title": "sin ibd", "desc": "Intent-based diffing"},
    {"title": "sin poc", "desc": "Proof-of-correctness"},
    {"title": "sin adw", "desc": "Architectural debt watchdog"},
    {"title": "sin oracle", "desc": "Independent verification oracle"},
    {"title": "sin sin-code run map <path>", "desc": "Architecture map"},
    {"title": "sin sin-code run scout <q>", "desc": "Code search"},
    {"title": "sin status", "desc": "Subsystem status"},
    {"title": "sin doctor", "desc": "Diagnose environment"},
    {"title": "sin bootstrap <path>", "desc": "Initialize subsystems"},
    {"title": "sin serve", "desc": "Expose tools as MCP server"},
    {"title": "sin brain", "desc": "Behavioral memory"},
    {"title": "sin context-bridge <q>", "desc": "Unified context query"},
    {"title": "sin update", "desc": "Upgrade pipx package + rebuild Go tools"},
    {"title": "sin config", "desc": "Read/write the layered config (TOML + opencode + env)"},
    {"title": "sin security secrets", "desc": "Scan for hardcoded secrets"},
    {"title": "sin security sast", "desc": "Static application security testing"},
    {"title": "sin security sca", "desc": "Software composition analysis (deps)"},
    {"title": "sin security sbom", "desc": "Generate SBOM (SPDX + CycloneDX)"},
    {"title": "sin security container", "desc": "Scan container images"},
    {"title": "sin security iac", "desc": "Scan IaC (Terraform, etc.)"},
    {"title": "sin security license", "desc": "License compliance check"},
    {"title": "sin security dast", "desc": "Dynamic application security testing"},
    {"title": "sin security full <path>", "desc": "Run all 8 security tools"},
]


# ── v1.4.0 — update commands ────────────────────────────────────────────────
# See update.doc.md for per-module design notes.

@update_app.callback()
def _update_callback() -> None:
    """Update the bundle (pipx) and/or the Go toolchain under ~/dev/."""


@update_app.command("run")
def update_run(
    core: bool = typer.Option(
        False, "--core", help="Only update the Python package (pipx upgrade)."
    ),
    go: bool = typer.Option(
        False, "--go", help="Only rebuild Go tools under ~/dev/SIN-Code-*-Tool/."
    ),
    check: bool = typer.Option(
        False, "--check", help="Dry run — print the plan but do not modify anything."
    ),
    json_out: bool = typer.Option(False, "--json", help="Output JSON instead of a table."),
) -> None:
    """Upgrade pipx package and rebuild every Go tool binary in place."""
    from sin_code_bundle import update as upd

    results = upd.run_update(core=core, go=go, check=check)
    if json_out:
        typer.echo(json.dumps([r.__dict__ for r in results], indent=2, default=str))
    else:
        typer.echo(upd.render_table(results))
    failed = [r for r in results if r.status == "failed"]
    if failed and not check:
        raise typer.Exit(code=1)
