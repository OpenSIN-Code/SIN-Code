# SPDX-License-Identifier: MIT
"""`sin agent` CLI subcommands."""

from __future__ import annotations

import asyncio
import json
import os
import sys
import time
from pathlib import Path

from .builtin_tools import register_builtin_tools
from .checkpoint import CheckpointStore
from .delegate import DelegationContext, make_delegate_tool
from .loop import AgentLoop
from .memory_bridge import MemoryBridge
from .policy_sandbox import PolicySandbox
from .router import ToolRouter
from .telemetry import Telemetry
from .types import AgentTask
from .verifier import Verifier


def _build_loop(repo_root: str, *, echo: bool,
                budget_s: float = 1800.0) -> AgentLoop:
    telemetry = Telemetry(echo=echo)
    router = register_builtin_tools(ToolRouter())
    sandbox = PolicySandbox.load(
        repo_root, dry_run=bool(os.environ.get("SIN_POLICY_DRY_RUN")))
    sandbox.wrap(router)

    ctx = DelegationContext(
        max_depth=int(os.environ.get("SIN_MAX_DELEGATION_DEPTH", "3")),
        budget_deadline=time.monotonic() + budget_s,
    )
    router.register("sin_delegate",
                    make_delegate_tool(ctx, telemetry,
                                       policy_wrap=sandbox.wrap))

    verifier = Verifier(
        repo_root, telemetry,
        lint_cmd=os.environ.get("SIN_LINT_CMD", "ruff check ."),
        test_cmd=os.environ.get("SIN_TEST_CMD", "pytest -x -q"),
        arch_cmd=os.environ.get("SIN_ARCH_CMD") or None,
    )
    return AgentLoop(router, verifier, telemetry=telemetry,
                     memory=MemoryBridge())


def register_agent_commands(app) -> None:
    import typer

    agent = typer.Typer(name="agent", help="Autonomous agent engine")
    app.add_typer(agent)

    @agent.command("run")
    def run(
        goal: str = typer.Option(..., "--goal"),
        plan_file: Path = typer.Option(..., "--plan"),
        repo: Path = typer.Option(Path.cwd(), "--repo"),
        parallel: int = typer.Option(4, "--parallel"),
        budget: float = typer.Option(1800.0, "--budget"),
        repair_rounds: int = typer.Option(3, "--repair-rounds"),
        quiet: bool = typer.Option(False, "--quiet"),
    ) -> None:
        specs = json.loads(plan_file.read_text(encoding="utf-8"))
        if isinstance(specs, dict):
            specs = specs.get("steps", [])
        task = AgentTask(
            goal=goal, repo_root=str(repo), max_parallelism=parallel,
            budget_seconds=budget, max_repair_rounds=repair_rounds,
        )
        loop = _build_loop(str(repo), echo=not quiet)
        report = asyncio.run(loop.run(task, specs))
        typer.echo(json.dumps(report, indent=2, ensure_ascii=False))
        raise typer.Exit(0 if report["outcome"] == "success" else 1)

    @agent.command("resume")
    def resume(
        goal: str = typer.Option(..., "--goal"),
        plan_file: Path = typer.Option(..., "--plan"),
        task_id: str = typer.Option(..., "--task-id"),
        repo: Path = typer.Option(Path.cwd(), "--repo"),
        parallel: int = typer.Option(4, "--parallel"),
        budget: float = typer.Option(1800.0, "--budget"),
    ) -> None:
        specs = json.loads(plan_file.read_text(encoding="utf-8"))
        if isinstance(specs, dict):
            specs = specs.get("steps", [])
        task = AgentTask(goal=goal, repo_root=str(repo),
                         max_parallelism=parallel, budget_seconds=budget)
        task.task_id = task_id
        loop = _build_loop(str(repo), echo=True)
        checkpoints = CheckpointStore(task_id, str(repo))
        state = checkpoints.load_resume_state()
        if not state.resumable:
            typer.echo(f"cannot resume: {state.reason}", err=True)
            raise typer.Exit(1)
        skipped = CheckpointStore.apply_to_plan(
            loop.planner.build(task, specs), state)
        typer.echo(f"resuming — skipped {len(skipped)} completed steps: "
                   f"{skipped}", err=True)
        report = asyncio.run(loop.run(task, specs))
        typer.echo(json.dumps(report, indent=2, ensure_ascii=False))
        raise typer.Exit(0 if report["outcome"] == "success" else 1)

    @agent.command("recall")
    def recall(
        goal: str = typer.Option(..., "--goal"),
        limit: int = typer.Option(5, "--limit"),
    ) -> None:
        typer.echo(json.dumps(
            MemoryBridge().recall_similar(goal, limit=limit),
            indent=2, ensure_ascii=False))

    @agent.command("stats")
    def stats() -> None:
        log = Path(os.environ.get("SIN_AGENT_LOG", "")
                   or Path.home() / ".sin" / "agent-events.jsonl")
        if not log.exists():
            typer.echo("no agent runs recorded yet")
            raise typer.Exit(0)
        lines = log.read_text(encoding="utf-8").splitlines()[-50:]
        typer.echo("\n".join(lines))

    @agent.command("synth")
    def synth(
        goal: str = typer.Option(..., "--goal"),
        repo: Path = typer.Option(Path.cwd(), "--repo"),
        out: Path = typer.Option(Path("plan.json"), "--out"),
        no_critique: bool = typer.Option(False, "--no-critique"),
    ) -> None:
        from .synthesizer import PlanSynthesizer

        llm_cmd = os.environ.get("SIN_LLM_CMD")
        if not llm_cmd:
            typer.echo("error: SIN_LLM_CMD not set", err=True)
            raise typer.Exit(2)

        async def complete(prompt: str) -> str:
            proc = await asyncio.create_subprocess_shell(
                llm_cmd,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
            )
            out_b, _ = await proc.communicate(prompt.encode())
            return out_b.decode(errors="replace")

        task = AgentTask(goal=goal, repo_root=str(repo))
        synthesizer = PlanSynthesizer(complete, critique=not no_critique)
        specs = asyncio.run(synthesizer.synthesize(task))
        out.write_text(
            json.dumps({"steps": specs}, indent=2, ensure_ascii=False),
            encoding="utf-8",
        )
        typer.echo(f"plan with {len(specs)} steps written to {out}")

    @agent.command("watch")
    def watch_cmd(
        log: Path | None = typer.Option(None, "--log"),
        refresh: float = typer.Option(0.5, "--refresh"),
        once: bool = typer.Option(False, "--once"),
    ) -> None:
        from .watch import watch
        watch(str(log) if log else None, refresh_s=refresh, once=once)

    @agent.command("insights")
    def insights(
        log: Path | None = typer.Option(None, "--log"),
        as_prompt: bool = typer.Option(False, "--prompt"),
    ) -> None:
        from .insights import TelemetryAnalyzer
        analyzer = TelemetryAnalyzer(str(log) if log else None)
        results = analyzer.analyze()
        if as_prompt:
            typer.echo(analyzer.render_for_prompt(results))
        else:
            typer.echo(json.dumps([i.to_dict() for i in results],
                                  indent=2, ensure_ascii=False))
        critical = any(i.severity == "critical" for i in results)
        raise typer.Exit(1 if critical else 0)

    @agent.command("policy-check")
    def policy_check(
        tool: str = typer.Option(..., "--tool"),
        args_json: str = typer.Option("{}", "--args"),
        repo: Path = typer.Option(Path.cwd(), "--repo"),
    ) -> None:
        sandbox = PolicySandbox.load(str(repo))
        allowed, reason = sandbox.decide(tool, json.loads(args_json))
        typer.echo(json.dumps({"tool": tool, "allowed": allowed,
                               "reason": reason}))
        raise typer.Exit(0 if allowed else 1)

    @agent.command("trace")
    def trace_cmd(
        trace_id: str | None = typer.Option(None, "--trace-id"),
        log: Path | None = typer.Option(None, "--log"),
        chrome: Path | None = typer.Option(None, "--chrome"),
    ) -> None:
        from .tracing import TraceAssembler
        assembler = TraceAssembler(str(log) if log else None)
        if chrome is not None:
            chrome.write_text(assembler.to_chrome_trace(trace_id),
                              encoding="utf-8")
            typer.echo(f"chrome trace written to {chrome} "
                       "(open via chrome://tracing or ui.perfetto.dev)")
            return
        roots = assembler.assemble(trace_id)
        if not roots:
            typer.echo("no spans found")
            raise typer.Exit(1)
        typer.echo(TraceAssembler.render_tree(
            roots, color=sys.stdout.isatty()))

    @agent.command("distill")
    def distill_cmd(
        since_days: float = typer.Option(7.0, "--since-days"),
        no_llm: bool = typer.Option(False, "--no-llm"),
    ) -> None:
        from .distiller import KnowledgeDistiller
        complete = None
        llm_cmd = os.environ.get("SIN_LLM_CMD")
        if llm_cmd and not no_llm:
            async def complete(prompt: str) -> str:
                proc = await asyncio.create_subprocess_shell(
                    llm_cmd, stdin=asyncio.subprocess.PIPE,
                    stdout=asyncio.subprocess.PIPE)
                out_b, _ = await proc.communicate(prompt.encode())
                return out_b.decode(errors="replace")
        distiller = KnowledgeDistiller(complete=complete)
        lessons = distiller.harvest_lessons(since_s=since_days * 86400)
        report = asyncio.run(distiller.distill(lessons))
        typer.echo(json.dumps({
            "harvested_lessons": len(lessons), **report,
            "active_rules": [r.rule for r in distiller.active_rules()],
        }, indent=2, ensure_ascii=False))

    @agent.command("rules")
    def rules_cmd() -> None:
        from .distiller import KnowledgeDistiller
        active = KnowledgeDistiller().active_rules()
        if not active:
            typer.echo("no active standing rules yet — run `sin agent distill`")
            return
        for r in active:
            typer.echo(f"[{r.score:5.1f}] ({r.evidence_count}x) {r.rule}")
