# SPDX-License-Identifier: MIT
"""Forwarding helpers and shared constants — extracted from cli.py.

Utility functions that are NOT decorated with @app.command() but are used
by command functions in cli.py, cli_sin_code.py, and cli_misc.py.
"""
from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import typer

# ── SIN-Code Go Tools registry ──────────────────────────────────────────────
# The repo key is kept for legacy fallback (standalone binary install).
_SIN_CODE_TOOLS = {
    # Core analysis (7)
    "discover": "SIN-Code-Discover-Tool",
    "execute": "SIN-Code-Execute-Tool",
    "map": "SIN-Code-Map-Tool",
    "grasp": "SIN-Code-Grasp-Tool",
    "scout": "SIN-Code-Scout-Tool",
    "harvest": "SIN-Code-Harvest-Tool",
    "orchestrate": "SIN-Code-Orchestrate-Tool",
    # Advanced tools (6) — previously thin wrappers, now native subcommands
    "ibd": "SIN-Code-IBD-Tool",
    "poc": "SIN-Code-PoC-Tool",
    "sckg": "SIN-Code-SCKG-Tool",
    "adw": "SIN-Code-ADW-Tool",
    "oracle": "SIN-Code-Oracle-Tool",
    "efm": "SIN-Code-EFM-Tool",
}

_EXCLUDE = {"venv", ".venv", "node_modules", ".git", "__pycache__"}

# Thin binary wrappers for new SIN-Code tools (v0.10.0)
_NEW_TOOL_BINARIES = {
    "ibd": ("SIN-Code-Intent-Based-Diffing", "ibd"),
    "poc": ("SIN-Code-Proof-of-Correctness", "poc"),
    "sckg": ("SIN-Code-Semantic-Codebase-Knowledge-Graphs", "sckg"),
    "adw": ("SIN-Code-Architectural-Debt-Watchdogs", "adw"),
    "oracle": ("SIN-Code-Verification-Oracle", "oracle"),
    "efm": ("SIN-Code-EFM-Tool", "efm"),
    "forge": ("SIN-Code-Forge-Tool", "forge"),
}

# CEO Audit skill paths
_CEO_AUDIT_SKILL_PATH = Path.home() / ".config" / "opencode" / "skills" / "ceo-audit"
_CEO_AUDIT_SCRIPT = _CEO_AUDIT_SKILL_PATH / "scripts" / "audit.sh"

# Catalog used by the non-TTY fallback in `tui`.
_TU_CATALOG = [
    {"title": "sin code", "desc": "Unified coding workflow hub"},
    {"title": "sin code full <path>", "desc": "Run preflight -> codocs -> debt -> sckg"},
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


def _sin_code_tool_path(name: str) -> Path | None:
    """Return the path to a SIN-Code Go binary if it exists."""
    home_bin = Path.home() / ".local" / "bin" / name
    if home_bin.exists():
        return home_bin
    # Also check PATH
    from shutil import which

    w = which(name)
    return Path(w) if w else None


def _require(module: str, hint: str):
    """Importiert ein Subsystem oder bricht mit klarer Meldung ab."""
    import importlib

    try:
        return importlib.import_module(module)
    except ImportError:
        typer.echo(f"[SIN-BUNDLE] Subsystem '{module}' not installed. Install with: {hint}")
        raise typer.Exit(code=1)


def _normalize_root_flag(args: list[str]) -> list[str]:
    """Normalize positional path arg to --root flag for commands that need it.

    `sin debt <path>` -> `sin debt --root <path>`
    `sin debt --root <path>` -> unchanged
    """
    if not args:
        return []
    # If first arg is a path (not a flag), wrap it with --root
    if not args[0].startswith("-"):
        return ["--root", args[0]] + args[1:]
    return args


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
    # Forward everything after the subcommand name
    args = sys.argv[sys.argv.index(name) + 1 :]
    result = subprocess.run([binary, *args])
    raise typer.Exit(code=result.returncode)


def _forward_security_subcommand(subcommand: str) -> None:
    """Forward `subcommand <rest of argv>` to the `sin-security` Go binary.

    Unlike `_forward_to_binary`, this helper is meant to be called from a Typer
    subcommand (e.g. `sin security secrets /path`). It extracts everything in
    sys.argv after the first occurrence of `subcommand` and prepends it to the
    binary invocation, so the binary sees the same shape it would if invoked
    directly (e.g. `sin-security secrets /path`).
    """
    import sys

    binary = shutil.which("sin-security")
    if not binary:
        typer.echo(
            "[SIN-BUNDLE] 'sin-security' binary not found in PATH. "
            "Build it from ~/SIN-Code-Security-Bundle:\n"
            "  go build -o ~/.local/bin/sin-security ./cmd/sin-security",
            err=True,
        )
        raise typer.Exit(code=1)
    args = sys.argv[sys.argv.index(subcommand) + 1 :] if subcommand in sys.argv else []
    result = subprocess.run([binary, subcommand, *args])
    raise typer.Exit(code=result.returncode)


def _build_cli_runner(agent: str):
    from sin_code_bundle.bench import CommandRunner

    def build_cmd(task, sin_enabled: bool) -> list[str]:
        prompt = task.problem_statement
        if agent == "opencode":
            return ["opencode", "run", "-m", prompt]
        if agent == "codex":
            return ["codex", "exec", "--skip-git-repo-check", prompt]
        if agent == "hermes":
            return ["hermes", "run", "--prompt", prompt]
        raise ValueError(agent)

    return CommandRunner(build_cmd=build_cmd, timeout_s=1800)


__all__ = [
    "_SIN_CODE_TOOLS",
    "_sin_code_tool_path",
    "_EXCLUDE",
    "_require",
    "_normalize_root_flag",
    "_NEW_TOOL_BINARIES",
    "_forward_to_binary",
    "_forward_security_subcommand",
    "_build_cli_runner",
    "_CEO_AUDIT_SKILL_PATH",
    "_CEO_AUDIT_SCRIPT",
    "_TU_CATALOG",
]
