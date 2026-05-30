"""Unified CLI fuer den gesamten SIN-Code Stack.

Subsysteme werden lazy und defensiv importiert: fehlt eines, bleibt der Rest
nutzbar und es wird eine klare Meldung statt eines Importfehlers ausgegeben.
"""
from __future__ import annotations

import json
from pathlib import Path

import typer

app = typer.Typer(help="SIN-Code Bundle - Unified SOTA Agent-Engineering Stack")

_EXCLUDE = ["venv", ".venv", "node_modules", ".git", "__pycache__"]


def _require(module: str, hint: str):
    """Importiert ein Subsystem oder bricht mit klarer Meldung ab."""
    import importlib

    try:
        return importlib.import_module(module)
    except ImportError:
        typer.echo(
            f"[SIN-BUNDLE] Subsystem '{module}' not installed. "
            f"Install with: {hint}"
        )
        raise typer.Exit(code=1)


@app.command()
def status():
    """Zeigt, welche Subsysteme installiert sind."""
    import importlib.util

    subsystems = {
        "sin_code_sckg": "SCKG (knowledge graph)",
        "sin_code_ibd": "IBD (intent diff)",
        "sin_code_poc": "POC (proof of correctness)",
        "sin_code_efsm": "EFSM (mock orchestration)",
        "sin_code_adw": "ADW (debt watchdog)",
        "sin_code_oracle": "Oracle (verification)",
        "sin_code_orchestration": "Orchestration (multi-agent workflow)",
        "sin_code_review_interface": "Review-Interface (semantic review UI)",
    }
    report = {}
    for mod, desc in subsystems.items():
        report[desc] = importlib.util.find_spec(mod) is not None
    typer.echo(json.dumps(report, indent=2))


@app.command()
def bootstrap(repo: str = typer.Argument(".", help="Repository root")):
    """Initialize available subsystems for a repository."""
    typer.echo(f"[SIN-BUNDLE] Bootstrapping {repo}...")
    sin_dir = Path(repo) / ".sin"
    sin_dir.mkdir(parents=True, exist_ok=True)

    # 1. Knowledge graph (optional)
    try:
        from sin_code_sckg.graph import KnowledgeGraph

        kg = KnowledgeGraph(storage_path=str(sin_dir / "knowledge.graph"))
        stats = kg.build_from_repo(repo, exclude=_EXCLUDE)
        typer.echo(f"[SIN-BUNDLE] SCKG built: {json.dumps(stats)}")
    except ImportError:
        typer.echo("[SIN-BUNDLE] SCKG not installed, skipping graph.")

    # 2. Baseline complexity (optional)
    try:
        from sin_code_adw.complexity import ComplexityAnalyzer
        from sin_code_adw.cost_tracker import CostTracker

        analyzer = ComplexityAnalyzer()
        reports = analyzer.analyze(repo, exclude=set(_EXCLUDE))
        baseline = analyzer.debt_score(reports)
        (sin_dir / "baseline.json").write_text(json.dumps(baseline, indent=2))
        CostTracker(log_path=str(sin_dir / "costs.jsonl"))
        typer.echo(f"[SIN-BUNDLE] ADW baseline: {json.dumps(baseline)}")
    except ImportError:
        typer.echo("[SIN-BUNDLE] ADW not installed, skipping baseline.")

    typer.echo("[SIN-BUNDLE] Bootstrap complete.")


@app.command()
def review(file_a: Path, file_b: Path):
    """Semantic review of a change (IBD)."""
    _require("sin_code_ibd", "pip install -e ../SIN-Code-Intent-Based-Diffing")
    from sin_code_ibd import ASTDiff, IntentSummarizer, RiskScorer

    changes = ASTDiff().diff_files(str(file_a), str(file_b))
    intents = IntentSummarizer().summarize(changes)
    risk = RiskScorer().score(changes)
    typer.echo(
        json.dumps(
            {"intents": [i.__dict__ for i in intents], "risk": risk}, indent=2
        )
    )


@app.command()
def debt(root: str = "."):
    """Show current architectural debt."""
    _require("sin_code_adw", "pip install -e ../SIN-Code-Architectural-Debt-Watchdogs")
    from sin_code_adw.complexity import ComplexityAnalyzer

    analyzer = ComplexityAnalyzer()
    reports = analyzer.analyze(root, exclude=set(_EXCLUDE))
    typer.echo(json.dumps(analyzer.debt_score(reports), indent=2))


@app.command()
def verify(test_command: str, root: str = "."):
    """Independent execution-based verification (Oracle)."""
    _require("sin_code_oracle", "pip install -e ../SIN-Code-Verification-Oracle")
    from sin_code_oracle.oracle import VerificationOracle

    oracle = VerificationOracle(workspace=root)
    verdict = oracle.verify(test_command=test_command, run_diagnostics=False)
    typer.echo(json.dumps(verdict.to_dict(), indent=2))


# --------------------------------------------------------------------------- #
# sin init  — one-command onboarding for any coder agent
# --------------------------------------------------------------------------- #
@app.command()
def init(
    agent: str = typer.Argument(
        ..., help="Target agent CLI: opencode | codex | hermes | all"
    ),
    scope: str = typer.Option(
        "local", help="'local' (this repo) or 'global' (user home)."
    ),
    with_agents_md: bool = typer.Option(
        True,
        "--agents-md/--no-agents-md",
        help="Also write AGENTS.md workflow doctrine.",
    ),
    force: bool = typer.Option(
        False, "--force", help="Overwrite an existing AGENTS.md."
    ),
    dry_run: bool = typer.Option(
        False, "--dry-run", help="Print what would be written, change nothing."
    ),
):
    """Wire the SIN-Code MCP server into a coding agent's config.

    Generates the agent-specific MCP config (idempotent merge) and, by default,
    an AGENTS.md workflow doctrine so the agent uses the tools best-practice.
    """
    from sin_code_bundle.generators import (
        SUPPORTED_AGENTS,
        write_agent_config,
        write_agents_md,
    )

    if scope not in ("local", "global"):
        typer.echo("[SIN-BUNDLE] --scope must be 'local' or 'global'.", err=True)
        raise typer.Exit(code=2)

    agents_to_init = list(SUPPORTED_AGENTS) if agent == "all" else [agent]
    for ag in agents_to_init:
        try:
            path, content = write_agent_config(ag, scope, dry_run=dry_run)  # type: ignore[arg-type]
        except ValueError as exc:
            typer.echo(f"[SIN-BUNDLE] {exc}", err=True)
            raise typer.Exit(code=2)
        verb = "Would write" if dry_run else "Wrote"
        typer.echo(f"[SIN-BUNDLE] {verb} {ag} config -> {path}")
        if dry_run:
            typer.echo(content)

    if with_agents_md:
        md_path, written = write_agents_md(dry_run=dry_run, force=force)
        if dry_run:
            typer.echo(f"[SIN-BUNDLE] Would write AGENTS.md -> {md_path}")
        elif written:
            typer.echo(f"[SIN-BUNDLE] Wrote AGENTS.md -> {md_path}")
        else:
            typer.echo(
                f"[SIN-BUNDLE] AGENTS.md already exists at {md_path} "
                "(use --force to overwrite)."
            )

    typer.echo("[SIN-BUNDLE] init complete. Restart your agent to load the MCP server.")


# --------------------------------------------------------------------------- #
# sin agents-md  — write the workflow doctrine standalone
# --------------------------------------------------------------------------- #
@app.command(name="agents-md")
def agents_md_cmd(
    root: str = typer.Argument(".", help="Repository root."),
    force: bool = typer.Option(False, "--force", help="Overwrite if it exists."),
    dry_run: bool = typer.Option(False, "--dry-run", help="Preview only."),
):
    """Generate the AGENTS.md engineering doctrine for this repository."""
    from sin_code_bundle.generators import render_agents_md, write_agents_md

    if dry_run:
        typer.echo(render_agents_md())
        return
    path, written = write_agents_md(Path(root), force=force)
    if written:
        typer.echo(f"[SIN-BUNDLE] Wrote {path}")
    else:
        typer.echo(
            f"[SIN-BUNDLE] {path} already exists (use --force to overwrite)."
        )


@app.command()
def serve():
    """Expose available tools as a unified MCP server (stdio)."""
    try:
        from mcp.server.fastmcp import FastMCP
    except ImportError:
        typer.echo("[SIN-BUNDLE] mcp package required: pip install 'sin-code-bundle[mcp]'", err=True)
        raise typer.Exit(code=1)

    mcp = FastMCP("sin-code-bundle")

    try:
        from sin_code_sckg.graph import KnowledgeGraph

        @mcp.tool()
        def impact(symbol_fqid: str) -> str:
            """Blast-radius impact analysis for a symbol."""
            kg = KnowledgeGraph(storage_path="./.sin/knowledge.graph")
            return json.dumps(kg.impact_analysis(symbol_fqid))
    except ImportError:
        pass

    try:
        from sin_code_ibd import ASTDiff, IntentSummarizer, RiskScorer

        @mcp.tool()
        def semantic_diff(file_a: str, file_b: str) -> str:
            """Semantic intent diff between two files."""
            changes = ASTDiff().diff_files(file_a, file_b)
            intents = IntentSummarizer().summarize(changes)
            risk = RiskScorer().score(changes)
            return json.dumps(
                {"intents": [i.__dict__ for i in intents], "risk": risk}
            )
    except ImportError:
        pass

    try:
        from sin_code_adw.complexity import ComplexityAnalyzer

        @mcp.tool()
        def architectural_debt() -> str:
            """Current architectural debt score."""
            analyzer = ComplexityAnalyzer()
            reports = analyzer.analyze(".", exclude=set(_EXCLUDE))
            return json.dumps(analyzer.debt_score(reports))
    except ImportError:
        pass


    try:
        from sin_code_oracle import VerificationOracle

        @mcp.tool()
        def verify_tests(code: str, language: str = "python") -> str:
            """Verify agent-generated code (security/performance/correctness)."""
            oracle = VerificationOracle()
            report = oracle.verify(code, language=language)
            return report.to_json()
    except ImportError:
        pass

    try:
        from sin_code_poc import ProofGenerator

        @mcp.tool()
        def prove(function_code: str, properties: str = "") -> str:
            """Generate and verify proofs of correctness."""
            gen = ProofGenerator()
            proof = gen.generate(function_code, properties=properties)
            return json.dumps({"proof": proof})
    except ImportError:
        pass

    try:
        from sin_code_efsm import EphemeralMockServer

        @mcp.tool()
        def mock_env(action: str = "up", port: int = 8888) -> str:
            """Manage ephemeral full-stack mock environment."""
            server = EphemeralMockServer(port=port)
            if action == "up":
                server.start()
                return json.dumps({"status": "up", "port": port})
            elif action == "down":
                server.stop()
                return json.dumps({"status": "down"})
            else:
                return json.dumps({"error": f"unknown action: {action}"})
    except ImportError:
        pass

    try:
        from sin_code_orchestration import Orchestrator, TaskSpec, Role

        @mcp.tool()
        def orchestrate(task_id: str, role: str, input_data: str) -> str:
            """Submit a task to the multi-agent orchestrator."""
            orch = Orchestrator()
            spec = TaskSpec(
                task_id=task_id,
                description=f"Task via MCP: {task_id}",
                role=Role(role),
                input_data=json.loads(input_data),
            )
            entry = orch.submit_task(spec)
            return json.dumps({"entry_id": entry.id, "status": entry.status.value})

        @mcp.tool()
        def task_status(entry_id: str) -> str:
            """Get status of an orchestrated task."""
            orch = Orchestrator()
            status = orch.status()
            return json.dumps(status)
    except ImportError:
        pass

    try:
        from sin_code_ibd import ASTDiff, IntentSummarizer, RiskScorer

        @mcp.tool()
        def semantic_review(file_a: str, file_b: str) -> str:
            """Comprehensive semantic review: intent + risk in one call."""
            changes = ASTDiff().diff_files(file_a, file_b)
            intents = IntentSummarizer().summarize(changes)
            risk = RiskScorer().score(changes)
            return json.dumps({
                "intents": [i.__dict__ for i in intents],
                "risk": risk,
                "recommendation": "Approve" if risk["risk"] == "low" else "Review Manually"
            })
    except ImportError:
        pass

    typer.echo("[SIN-BUNDLE] MCP server starting (stdio).", err=True)
    mcp.run()


# --------------------------------------------------------------------------- #
# sin bench  — SWE-bench A/B harness
# --------------------------------------------------------------------------- #
@app.command()
def bench(
    tasks: str | None = typer.Option(
        None, "--tasks", help="Path to a JSONL task file. Omit to use SWE-bench Lite."
    ),
    limit: int = typer.Option(20, help="Max number of tasks to run per arm."),
    runner: str = typer.Option(
        "dry", help="Agent runner: 'dry' | 'opencode' | 'codex' | 'hermes'."
    ),
    arms: str = typer.Option(
        "control,sin", help="Comma-separated arms to run."
    ),
    out: str | None = typer.Option(
        None, "--out", help="Write the full JSON report to this path."
    ),
):
    """Run the SIN-Code A/B benchmark and report the resolved-rate delta."""
    from sin_code_bundle.bench import (
        CommandRunner,
        DryRunRunner,
        format_report,
        load_swebench_lite,
        load_tasks_jsonl,
        run_benchmark,
    )

    if tasks:
        task_list = load_tasks_jsonl(Path(tasks), limit=limit)
    else:
        try:
            task_list = load_swebench_lite(limit=limit)
        except RuntimeError as exc:
            typer.echo(f"[SIN-BUNDLE] {exc}", err=True)
            raise typer.Exit(code=2)

    if not task_list:
        typer.echo("[SIN-BUNDLE] No tasks loaded.", err=True)
        raise typer.Exit(code=2)

    if runner == "dry":
        agent_runner = DryRunRunner()
    elif runner in ("opencode", "codex", "hermes"):
        agent_runner = _build_cli_runner(runner)
    else:
        typer.echo(f"[SIN-BUNDLE] Unknown runner '{runner}'.", err=True)
        raise typer.Exit(code=2)

    arm_tuple = tuple(a.strip() for a in arms.split(",") if a.strip())

    typer.echo(
        f"[SIN-BUNDLE] Running {len(task_list)} task(s) x {len(arm_tuple)} arm(s) "
        f"with '{runner}' runner..."
    )
    report = run_benchmark(task_list, agent_runner, arms=arm_tuple)  # type: ignore[arg-type]
    typer.echo(format_report(report))

    if out:
        Path(out).write_text(report.to_json(), encoding="utf-8")
        typer.echo(f"[SIN-BUNDLE] Wrote full report -> {out}")


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


# --------------------------------------------------------------------------- #
# sin skills  — compile portable skills into an agent's native format
# --------------------------------------------------------------------------- #
@app.command()
def skills(
    target: str = typer.Argument(..., help="opencode | codex | claude | all"),
    source: str = typer.Option("skills", help="Source skills directory."),
    dry_run: bool = typer.Option(False, "--dry-run", help="Preview only."),
):
    """Compile portable SIN skills into an agent's native command/skill format."""
    from sin_code_bundle.skills import SUPPORTED_TARGETS, compile_skills

    valid = SUPPORTED_TARGETS
    targets = list(valid) if target == "all" else [target]  # type: ignore[list-item]
    for t in targets:
        if t not in valid:
            typer.echo(f"[SIN-BUNDLE] Unknown target '{t}'.", err=True)
            raise typer.Exit(code=2)
        paths = compile_skills(t, Path(source), dry_run=dry_run)  # type: ignore[arg-type]
        verb = "Would write" if dry_run else "Wrote"
        for p in paths:
            typer.echo(f"[SIN-BUNDLE] {verb} {t} skill -> {p}")
        if not paths:
            typer.echo(f"[SIN-BUNDLE] No skills found in '{source}'.")


# --------------------------------------------------------------------------- #
# sin policy  — inspect / initialize the policy and audit log
# --------------------------------------------------------------------------- #
@app.command()
def policy(
    action: str = typer.Argument("show", help="show | init | verify"),
    root: str = typer.Option(".", help="Project root."),
):
    """Inspect or initialize the SIN policy and audit log."""
    from sin_code_bundle.policy import DEFAULT_POLICY, AuditLog, Policy

    root_path = Path(root)
    if action == "init":
        path = root_path / ".sin" / "policy.yaml"
        path.parent.mkdir(parents=True, exist_ok=True)
        if path.exists():
            typer.echo(f"[SIN-BUNDLE] {path} already exists.")
            return
        try:
            import yaml as _yaml

            path.write_text(
                _yaml.safe_dump(
                    {"auto_approve": False, "rules": dict(DEFAULT_POLICY)},
                    sort_keys=False,
                ),
                encoding="utf-8",
            )
        except ImportError:
            # Manual fallback if pyyaml missing
            path.write_text(
                "auto_approve: false\nrules:\n"
                + "".join(f"  {k}: {v}\n" for k, v in DEFAULT_POLICY.items()),
                encoding="utf-8",
            )
        typer.echo(f"[SIN-BUNDLE] Wrote default policy -> {path}")
        return

    if action == "verify":
        ok = AuditLog(root_path).verify_chain()
        typer.echo(f"[SIN-BUNDLE] Audit chain {'intact' if ok else 'TAMPERED'}.")
        raise typer.Exit(code=0 if ok else 1)

    p = Policy.load(root_path)
    typer.echo("[SIN-BUNDLE] Effective policy:")
    for risk, decision in p.rules.items():
        typer.echo(f"  {risk:<8} -> {decision}")
    typer.echo(f"  auto_approve = {p.auto_approve}")


# --------------------------------------------------------------------------- #
# sin doctor  — environment diagnostics
# --------------------------------------------------------------------------- #
@app.command()
def doctor(root: str = typer.Option(".", help="Project root.")):
    """Diagnose the environment: detected languages, LSP servers, audit chain."""
    from sin_code_bundle.lsp_bootstrap import server_status
    from sin_code_bundle.policy import AuditLog

    rows = server_status(Path(root))
    typer.echo("[SIN-BUNDLE] Language servers (for accurate impact analysis):")
    if not rows:
        typer.echo("  (no supported source files detected)")
    for r in rows:
        mark = "OK " if r["installed"] else "-- "
        typer.echo(
            f"  {mark}{r['language']:<11} {r['files']:>5} files  server={r['server']}"
        )
        if not r["installed"]:
            typer.echo(f"       install: {r['install_hint']}")

    ok = AuditLog(Path(root)).verify_chain()
    typer.echo(f"[SIN-BUNDLE] Audit chain: {'intact' if ok else 'TAMPERED'}")


if __name__ == "__main__":
    app()
