# SPDX-License-Identifier: MIT
"""CEO Audit sub-commands — extracted from cli.py."""
from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import typer

from sin_code_bundle.cli_app import ceo_audit_app

_CEO_AUDIT_SKILL_PATH = Path.home() / ".config" / "opencode" / "skills" / "ceo-audit"
_CEO_AUDIT_SCRIPT = _CEO_AUDIT_SKILL_PATH / "scripts" / "audit.sh"

_SIN_CODE_TOOLS = {
    "discover": "SIN-Code-Discover-Tool",
    "execute": "SIN-Code-Execute-Tool",
    "map": "SIN-Code-Map-Tool",
    "grasp": "SIN-Code-Grasp-Tool",
    "scout": "SIN-Code-Scout-Tool",
    "harvest": "SIN-Code-Harvest-Tool",
    "orchestrate": "SIN-Code-Orchestration-Tool",
    "ibd": "SIN-Code-IBD-Tool",
    "poc": "SIN-Code-PoC-Tool",
    "sckg": "SIN-Code-SCKG-Tool",
    "adw": "SIN-Code-ADW-Tool",
    "oracle": "SIN-Code-Oracle-Tool",
    "efm": "SIN-Code-EFM-Tool",
}


@ceo_audit_app.command("run")
def ceo_audit_run(
    repo: str = typer.Argument(".", help="Path to the repository to audit"),
    profile: str = typer.Option("FULL", "--profile", help="FULL | SECURITY | RELEASE | QUICK"),
    grade: str = typer.Option("", "--grade", help="CI grade gate: A | B | C"),
    output: str = typer.Option("", "--output", help="Output directory (default: ~/ceo-audits/)"),
    json_out: bool = typer.Option(False, "--json", help="Also write JSON sidecar"),
    no_color: bool = typer.Option(False, "--no-color", help="Disable ANSI colors"),
):
    """Run a 47-gate, 8-axis SOTA audit on a repository.

    Requires the ceo-audit skill to be installed (run `sin ceo-audit install`).
    """
    if not _CEO_AUDIT_SCRIPT.exists():
        typer.echo(
            f"[CEO-AUDIT] Skill not installed at {_CEO_AUDIT_SKILL_PATH}.\n"
            f"  Install: sin ceo-audit install",
            err=True,
        )
        raise typer.Exit(code=4)

    args = [str(_CEO_AUDIT_SCRIPT), f"--profile={profile}"]
    if grade:
        args.append(f"--grade={grade}")
    if output:
        args.append(f"--output={output}")
    if json_out:
        args.append("--json")
    if no_color:
        args.append("--no-color")
    args.append(repo)

    result = subprocess.run(args)
    raise typer.Exit(code=result.returncode)


@ceo_audit_app.command("install")
def ceo_audit_install(
    force: bool = typer.Option(False, "--force", help="Overwrite existing files"),
):
    """Install the ceo-audit skill to ~/.config/opencode/skills/ceo-audit/.

    Idempotent: safe to run multiple times. Use --force to overwrite.
    """
    skill_source = Path(__file__).parent.parent.parent.parent / "skills" / "ceo-audit"
    skill_target = _CEO_AUDIT_SKILL_PATH

    if not skill_source.exists():
        skill_source = Path("/Users/jeremy/dev/SIN-Code-Bundle/skills/ceo-audit")
    if not skill_source.exists():
        typer.echo(
            f"[CEO-AUDIT] Cannot find ceo-audit skill source. Looked in:\n  {skill_source}",
            err=True,
        )
        raise typer.Exit(code=1)

    skill_target.parent.mkdir(parents=True, exist_ok=True)
    if skill_target.exists() and not force:
        typer.echo(f"[CEO-AUDIT] Skill already installed at {skill_target}")
        typer.echo("  Use --force to overwrite.")
        raise typer.Exit(code=0)

    shutil.copytree(skill_source, skill_target, dirs_exist_ok=True)
    for script in (skill_target / "scripts").glob("*.sh"):
        script.chmod(0o755)
    if (skill_target / "hooks" / "post_audit.py").exists():
        (skill_target / "hooks" / "post_audit.py").chmod(0o755)
    typer.echo(f"[CEO-AUDIT] Installed to {skill_target}")
    typer.echo("  Run: sin ceo-audit run /path/to/repo")


@ceo_audit_app.command("status")
def ceo_audit_status():
    """Show whether the ceo-audit skill is installed and ready."""
    installed = _CEO_AUDIT_SCRIPT.exists()
    typer.echo(f"CEO Audit skill installed: {'yes' if installed else 'no'}")
    if installed:
        typer.echo(f"  Path: {_CEO_AUDIT_SKILL_PATH}")
        from shutil import which

        missing = [t for t in _SIN_CODE_TOOLS if not which(t)]
        if missing:
            typer.echo(f"  Missing SIN-Code tools: {', '.join(missing)}")
            typer.echo("  Install: bash ~/.local/share/SIN-Code/install.sh")
        else:
            typer.echo("  All 7 SIN-Code tools available")
    else:
        typer.echo("  Install: sin ceo-audit install")
