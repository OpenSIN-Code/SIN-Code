# SPDX-License-Identifier: MIT
"""CEO Audit CLI backed by the canonical source or bundled wheel resource."""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import typer

from sin_code_bundle.cli_app import ceo_audit_app

_SKILL_RELATIVE = Path("skills/code-skills/skill-code-ceo-audit")
_USER_SKILL_RELATIVE = Path(".config/opencode/skills/ceo-audit")

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


def _installed_user_skill() -> Path:
    return Path.home() / _USER_SKILL_RELATIVE


def _user_skill_path() -> Path:
    """Return the install target, allowing an explicit test/operator override."""
    override = os.environ.get("SIN_CEO_AUDIT_SKILL_PATH")
    return Path(override).expanduser() if override else _installed_user_skill()


def _source_checkout_skill() -> Path:
    return Path(__file__).resolve().parents[2] / _SKILL_RELATIVE


def _bundled_skill() -> Path:
    return Path(__file__).resolve().parent / "resources" / "ceo-audit"


def _distribution_skill_source() -> Path | None:
    """Return the immutable source used for run/install operations."""
    for candidate in (_source_checkout_skill(), _bundled_skill()):
        if (candidate / "scripts" / "audit.sh").is_file():
            return candidate
    return None


def _active_skill() -> Path | None:
    """Resolve the newest trustworthy audit engine in deterministic order.

    An explicit override is authoritative. Otherwise, immutable resources from
    the current checkout/distribution take precedence over a potentially stale
    user-installed OpenCode copy.
    """
    explicit = os.environ.get("SIN_CEO_AUDIT_SKILL_PATH")
    candidates = []
    if explicit:
        candidates.append(Path(explicit).expanduser())
    candidates.extend((_source_checkout_skill(), _bundled_skill(), _installed_user_skill()))
    for candidate in candidates:
        if (candidate / "scripts" / "audit.sh").is_file():
            return candidate
    return None


@ceo_audit_app.command("run")
def ceo_audit_run(
    repo: str = typer.Argument(".", help="Path to the repository to audit"),
    profile: str = typer.Option("FULL", "--profile", help="FULL | SECURITY | RELEASE | QUICK"),
    grade: str = typer.Option("", "--grade", help="CI grade gate: A | B | C"),
    output: str = typer.Option("", "--output", help="Output directory (default: ~/ceo-audits/)"),
    json_out: bool = typer.Option(False, "--json", help="Also write JSON sidecar"),
    no_color: bool = typer.Option(False, "--no-color", help="Disable ANSI colors"),
) -> None:
    """Run the canonical CEO Audit from source, user install, or wheel."""
    skill = _active_skill()
    bash = shutil.which("bash")
    if skill is None or bash is None:
        reason = "CEO Audit resource not found" if skill is None else "bash is required"
        typer.echo(
            f"[CEO-AUDIT] {reason}. Reinstall with: pip install --force-reinstall sin-code",
            err=True,
        )
        raise typer.Exit(code=4)

    args = [bash, str(skill / "scripts" / "audit.sh"), f"--profile={profile}"]
    if grade:
        args.append(f"--grade={grade}")
    if output:
        args.append(f"--output={output}")
    if json_out:
        args.append("--json")
    if no_color:
        args.append("--no-color")
    args.append(repo)

    result = subprocess.run(args, check=False)
    raise typer.Exit(code=result.returncode)


@ceo_audit_app.command("install")
def ceo_audit_install(
    force: bool = typer.Option(False, "--force", help="Overwrite existing files"),
) -> None:
    """Install the bundled canonical skill into the OpenCode runtime path."""
    skill_source = _distribution_skill_source()
    skill_target = _user_skill_path()
    if skill_source is None:
        typer.echo(
            "[CEO-AUDIT] Bundled skill resource missing. Reinstall with: pip install --force-reinstall sin-code",
            err=True,
        )
        raise typer.Exit(code=1)

    skill_target.parent.mkdir(parents=True, exist_ok=True)
    if skill_target.exists() and not force:
        typer.echo(f"[CEO-AUDIT] Skill already installed at {skill_target}")
        typer.echo("  Use --force to overwrite.")
        raise typer.Exit(code=0)

    shutil.copytree(skill_source, skill_target, dirs_exist_ok=True)
    for script in (skill_target / "scripts").rglob("*"):
        if script.is_file() and (script.suffix == ".sh" or script.parent.name == "compat-bin"):
            script.chmod(0o755)
    hook = skill_target / "hooks" / "post_audit.py"
    if hook.exists():
        hook.chmod(0o755)
    typer.echo(f"[CEO-AUDIT] Installed to {skill_target}")
    typer.echo("  Run: sin ceo-audit run /path/to/repo")


@ceo_audit_app.command("status")
def ceo_audit_status() -> None:
    """Show the active CEO Audit resource and optional tool coverage."""
    skill = _active_skill()
    typer.echo(f"CEO Audit available: {'yes' if skill else 'no'}")
    if skill is None:
        typer.echo("  Reinstall: pip install --force-reinstall sin-code")
        return

    typer.echo(f"  Active source: {skill}")
    missing = [tool for tool in _SIN_CODE_TOOLS if not shutil.which(tool)]
    if missing:
        typer.echo(f"  Optional tools missing: {', '.join(missing)}")
        typer.echo("  The audit reports skipped gates and reduces assurance accordingly.")
    else:
        typer.echo("  All optional SIN-Code tools available")
