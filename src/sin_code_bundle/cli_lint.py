# SPDX-License-Identifier: MIT
"""Lint sub-commands — extracted from cli.py."""
from __future__ import annotations

import subprocess

import typer

from sin_code_bundle.cli_app import lint_app


@lint_app.command("run")
def lint_run(
    path: str = typer.Argument(".", help="Path to lint."),
    tool: str = typer.Option(
        "auto",
        help="Linter to use: auto, ruff, flake8, mypy, pylint, eslint, golangci-lint, shellcheck.",
    ),
    fix: bool = typer.Option(False, help="Auto-fix issues where supported."),
):
    """Run a linter on the given path."""
    import shutil

    linters = {
        "ruff": ("ruff", ["check", path] + (["--fix"] if fix else [])),
        "flake8": ("flake8", [path]),
        "mypy": ("mypy", [path]),
        "pylint": ("pylint", [path]),
        "eslint": ("eslint", [path] + (["--fix"] if fix else [])),
        "golangci-lint": ("golangci-lint", ["run", path]),
        "shellcheck": ("shellcheck", [path]),
    }

    if tool == "auto":
        for name, (binary, _) in linters.items():
            if shutil.which(binary):
                tool = name
                break
        else:
            typer.echo(
                "[SIN-BUNDLE] No supported linter found on PATH. Install one: ruff, flake8, mypy, pylint, eslint, golangci-lint, shellcheck",
                err=True,
            )
            raise typer.Exit(code=1)

    if tool not in linters:
        typer.echo(f"[SIN-BUNDLE] Unknown linter '{tool}'.", err=True)
        raise typer.Exit(code=1)

    binary, args = linters[tool]
    if not shutil.which(binary):
        typer.echo(f"[SIN-BUNDLE] '{binary}' not found on PATH. Install it first.", err=True)
        raise typer.Exit(code=1)

    result = subprocess.run([binary, *args], capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@lint_app.command("check")
def lint_check(
    path: str = typer.Argument(".", help="Path to check."),
):
    """Check which linters are available and what they would report."""
    import shutil

    available = []
    for name, binary in [
        ("ruff", "ruff"),
        ("flake8", "flake8"),
        ("mypy", "mypy"),
        ("pylint", "pylint"),
        ("eslint", "eslint"),
        ("golangci-lint", "golangci-lint"),
        ("shellcheck", "shellcheck"),
    ]:
        if shutil.which(binary):
            available.append(name)

    if not available:
        typer.echo("[SIN-BUNDLE] No linters found on PATH.")
    else:
        typer.echo(f"[SIN-BUNDLE] Available linters: {', '.join(available)}")

    for name in available:
        typer.echo(f"\n--- {name} ---")
        if name == "ruff":
            result = subprocess.run(["ruff", "check", path], capture_output=True, text=True)
        elif name == "flake8":
            result = subprocess.run(["flake8", path], capture_output=True, text=True)
        elif name == "mypy":
            result = subprocess.run(["mypy", path], capture_output=True, text=True)
        else:
            continue

        if result.stdout:
            typer.echo(result.stdout)
        if result.stderr:
            typer.echo(result.stderr, err=True)
