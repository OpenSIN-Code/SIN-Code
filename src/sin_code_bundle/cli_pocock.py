# SPDX-License-Identifier: MIT
"""Pocock sub-commands — extracted from cli.py."""
from __future__ import annotations

import json
import subprocess
from pathlib import Path

import typer

from sin_code_bundle.cli_app import pocock_app


@pocock_app.command("grill-me")
def pocock_grill_me(
    goal: str = typer.Argument(..., help="Development goal / feature description"),
    output: str = typer.Option("PRD.md", "--output", "-o", help="Output path for PRD.md"),
    non_interactive: bool = typer.Option(
        False, "--non-interactive", help="Non-interactive mode (CI/CD)"
    ),
    answers: str = typer.Option(None, "--answers", help="JSON answers for non-interactive mode"),
    json_out: bool = typer.Option(False, "--json", help="Output JSON"),
):
    """Socratic alignment tool - asks clarifying questions before coding."""
    from sin_code_bundle.tools.pocock.grill_me import GrillMe

    grill = GrillMe(goal)
    if non_interactive:
        if not answers:
            typer.echo("❌ --non-interactive requires --answers JSON", err=True)
            raise typer.Exit(code=1)
        import json

        answers_dict = json.loads(answers)
        grill.run_non_interactive(answers_dict)
    else:
        grill.run_interactive()

    grill.generate_prd(output)

    if json_out:
        typer.echo(grill.to_json())
    else:
        typer.echo(f"🎉 PRD generated: {output}")


@pocock_app.command("tdd-enforcer")
def pocock_tdd_enforcer(
    test_cmd: str = typer.Argument(..., help="Test command (e.g., 'pytest tests/')"),
    file: str = typer.Argument(..., help="File to edit"),
    lock_dir: str = typer.Option(None, "--lock-dir", help="Directory for lock files"),
    reset: bool = typer.Option(False, "--reset", help="Reset TDD state for this file"),
    check: bool = typer.Option(False, "--check", help="Only check lock status"),
    json_out: bool = typer.Option(False, "--json", help="Output JSON"),
):
    """TDD gatekeeper - enforces Red-Green-Refactor cycle before editing."""
    from sin_code_bundle.tools.pocock.tdd_enforcer import TDDEnforcer

    enforcer = TDDEnforcer(test_cmd, file, lock_dir)

    if reset:
        enforcer.reset()
        raise typer.Exit()

    if check:
        result = {
            "is_locked": enforcer.is_locked(),
            "phase": enforcer._get_current_phase(),
            "file": file,
            "lock_file": enforcer.lock_file,
        }
        if json_out:
            typer.echo(json.dumps(result, indent=2))
        else:
            status = "🔒 Locked" if result["is_locked"] else "🔓 Unlocked"
            typer.echo(f"{status} - Phase: {result['phase']}")
        raise typer.Exit()

    result = enforcer.enforce()

    if json_out:
        typer.echo(json.dumps(result, indent=2, ensure_ascii=False))
    else:
        typer.echo(f"\n{'=' * 60}")
        typer.echo(f"Result: {result['status'].upper()}")
        typer.echo(f"Phase: {result['phase'].upper()}")
        typer.echo(f"{'=' * 60}")

    if result["status"] == "blocked":
        raise typer.Exit(code=1)


@pocock_app.command("dag-kanban")
def pocock_dag_kanban(
    prd: str = typer.Option("PRD.md", "--prd", help="Path to PRD.md"),
    json_out: bool = typer.Option(False, "--json", help="Output JSON"),
    docker: bool = typer.Option(False, "--docker", help="Export Docker Compose"),
    output: str = typer.Option(
        "docker-compose.dag.yml", "--output", help="Docker Compose output path"
    ),
):
    """DAG-based Kanban - parses PRD and creates task execution graph."""
    from sin_code_bundle.tools.pocock.dag_kanban import DAGKanban

    runner = DAGKanban(prd)
    runner.run()

    if json_out:
        typer.echo(runner.to_json())

    if docker:
        try:
            import yaml  # noqa: F401

            runner.export_docker_compose(output)
        except ImportError:
            typer.echo("⚠️  PyYAML not installed. Run: pip install pyyaml", err=True)
            raise typer.Exit(code=1)


@pocock_app.command("cleanup")
def pocock_cleanup():
    """Run post-flight cleanup hook (system cleanup after task runs)."""
    script_path = (
        Path(__file__).parent.parent.parent / "scripts" / "pocock" / "opencode-cleanup-hook.sh"
    )
    if not script_path.exists():
        typer.echo("❌ Cleanup script not found. Is the bundle installed correctly?", err=True)
        raise typer.Exit(code=1)

    result = subprocess.run(["bash", str(script_path)], capture_output=True, text=True)
    typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@pocock_app.command("safe-start")
def pocock_safe_start():
    """Start OpenCode with safe environment injection (Zod patch + env substitution)."""
    script_path = (
        Path(__file__).parent.parent.parent / "scripts" / "pocock" / "opencode-safe-start.sh"
    )
    if not script_path.exists():
        typer.echo("❌ Safe-start script not found. Is the bundle installed correctly?", err=True)
        raise typer.Exit(code=1)

    # Forward remaining args to the script
    import sys

    args = sys.argv[sys.argv.index("safe-start") + 1 :]
    result = subprocess.run(["bash", str(script_path), *args])
    raise typer.Exit(code=result.returncode)
