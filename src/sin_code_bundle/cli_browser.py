# SPDX-License-Identifier: MIT
"""Browser sub-commands — extracted from cli.py."""
from __future__ import annotations

import json as _json
import shutil
import subprocess
from collections import defaultdict
from pathlib import Path

import typer

from sin_code_bundle.cli_app import browser_app


@browser_app.command("list")
def browser_list(
    filter: str = typer.Option(
        "", "--filter", help="Substring filter (e.g. 'click', 'screenshot')"
    ),
    json_out: bool = typer.Option(False, "--json", help="Output full JSON instead of summary"),
):
    """List all 106 sin-browser-tools. Always run this first to discover the surface."""
    if not shutil.which("sin-browser"):
        typer.echo(
            "[BROWSER] sin-browser not installed. Install: https://github.com/OpenSIN-Code/SIN-Browser-Tools",
            err=True,
        )
        raise typer.Exit(code=1)
    result = subprocess.run(["sin-browser", "skills"], capture_output=True, text=True, timeout=30)
    if result.returncode != 0:
        typer.echo(f"[BROWSER] sin-browser failed: {result.stderr}", err=True)
        raise typer.Exit(code=1)

    data = _json.loads(result.stdout)
    actions = data.get("actions", {})
    if filter:
        actions = {
            k: v
            for k, v in actions.items()
            if filter.lower() in k.lower() or filter.lower() in v.get("description", "").lower()
        }
    if json_out:
        typer.echo(_json.dumps(actions, indent=2))
    else:
        by_cat: dict[str, list] = defaultdict(list)
        for name, tool in actions.items():
            by_cat[tool.get("category", "other")].append((name, tool.get("description", "")))
        typer.echo(f"\n  sin-browser-tools -- {len(actions)} tools\n")
        for cat in sorted(by_cat):
            typer.echo(f"[{cat}] ({len(by_cat[cat])})")
            for name, desc in sorted(by_cat[cat]):
                desc_short = desc[:55] + "..." if len(desc) > 55 else desc
                typer.echo(f"  - {name:35s} {desc_short}")
            typer.echo("")


@browser_app.command("help")
def browser_help():
    """Show sin-browser help."""
    if not shutil.which("sin-browser"):
        typer.echo("[BROWSER] sin-browser not installed", err=True)
        raise typer.Exit(code=1)
    subprocess.run(["sin-browser", "help"])


@browser_app.command("install-skill")
def browser_install_skill():
    """Install the sin-browser-tools skill to ~/.config/opencode/skills/."""
    skill_source = Path(__file__).parent.parent.parent.parent / "skills" / "sin-browser-tools"
    skill_target = Path.home() / ".config" / "opencode" / "skills" / "sin-browser-tools"
    if not skill_source.exists():
        skill_source = Path("/Users/jeremy/dev/Infra-SIN-OpenCode-Stack/skills/sin-browser-tools")
    if not skill_source.exists():
        typer.echo("[BROWSER] Cannot find skill source", err=True)
        raise typer.Exit(code=1)
    skill_target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copytree(skill_source, skill_target, dirs_exist_ok=True)
    for script in (skill_target / "scripts").glob("*.py"):
        script.chmod(0o755)
    typer.echo(f"[BROWSER] Installed skill to {skill_target}")


@browser_app.command("status")
def browser_status():
    """Show sin-browser status."""
    if not shutil.which("sin-browser"):
        typer.echo("sin-browser installed: no")
        typer.echo("  Install: https://github.com/OpenSIN-Code/SIN-Browser-Tools")
        raise typer.Exit(code=1)
    result = subprocess.run(["sin-browser", "skills"], capture_output=True, text=True, timeout=10)
    if result.returncode != 0:
        typer.echo("sin-browser installed: yes (but broken)")
        typer.echo(f"  Error: {result.stderr[:200]}")
        raise typer.Exit(code=1)

    try:
        data = _json.loads(result.stdout)
        count = data.get("count", 0)
    except Exception:
        count = "?"
    typer.echo(f"sin-browser installed: yes ({count} tools available)")
    skill = Path.home() / ".config" / "opencode" / "skills" / "sin-browser-tools" / "SKILL.md"
    typer.echo(f"  Skill installed: {'yes' if skill.exists() else 'no'}")
    typer.echo("  See: sin browser list")
