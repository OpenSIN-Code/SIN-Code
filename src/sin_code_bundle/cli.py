"""Unified CLI fuer den gesamten SIN-Code Stack.

Subsysteme werden lazy und defensiv importiert: fehlt eines, bleibt der Rest
nutzbar und es wird eine klare Meldung statt eines Importfehlers ausgegeben.
"""
from __future__ import annotations

import json
from pathlib import Path

import typer

app = typer.Typer(help="SIN-Code Bundle - Unified SOTA Agent-Engineering Stack")

gitnexus_app = typer.Typer(
    help="GitNexus bridge - mandatory graph context for coder agents."
)
app.add_typer(gitnexus_app, name="gitnexus")

markitdown_app = typer.Typer(
    help="MarkItDown bridge - document->Markdown context for coder agents."
)
app.add_typer(markitdown_app, name="markitdown")

rtk_app = typer.Typer(
    help="RTK bridge - token-saving command proxy for coder agents."
)
app.add_typer(rtk_app, name="rtk")

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

    # External upstream tools (not Python subsystems): report their runtime
    # availability so it is obvious when an agent would be missing context.
    from sin_code_bundle import gitnexus, markitdown, rtk

    report["GitNexus (graph context, external)"] = gitnexus.detect_env().available
    report["MarkItDown (doc->markdown, external)"] = (
        markitdown.detect_env().mcp_available
    )
    report["RTK (token-saving proxy, external)"] = rtk.detect_env().available
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


@gitnexus_app.command("doctor")
def gitnexus_doctor(root: str = typer.Argument(".", help="Repository root")):
    """Check Node/npx + GitNexus index health."""
    from sin_code_bundle import gitnexus

    typer.echo(json.dumps(gitnexus.doctor(root), indent=2))


@gitnexus_app.command("setup")
def gitnexus_setup(
    agents: str = typer.Option(
        "opencode,codex,hermes",
        help="Comma-separated agents to wire (opencode,codex,hermes).",
    ),
):
    """Wire the GitNexus MCP server into each coder agent's config."""
    from sin_code_bundle import gitnexus

    chosen = [a.strip() for a in agents.split(",") if a.strip()]
    try:
        written = gitnexus.setup_agents(chosen)
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)
    for agent, path in written.items():
        typer.echo(f"[GITNEXUS] wired {agent} -> {path}")
    typer.echo("[GITNEXUS] Agents now have mandatory graph context via MCP.")


@gitnexus_app.command("index")
def gitnexus_index(
    root: str = typer.Argument(".", help="Repository root"),
    force: bool = typer.Option(False, "--force", help="Rebuild even if fresh."),
):
    """Build or refresh the GitNexus index for a repository."""
    from sin_code_bundle import gitnexus

    try:
        if force:
            gitnexus.analyze(root)
            state = gitnexus.index_state(root)
        else:
            state = gitnexus.ensure_index(root, auto=True)
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)
    typer.echo(json.dumps(state.to_dict(), indent=2))


@gitnexus_app.command("status")
def gitnexus_status(root: str = typer.Argument(".", help="Repository root")):
    """Show the on-disk index state without invoking GitNexus."""
    from sin_code_bundle import gitnexus

    typer.echo(json.dumps(gitnexus.index_state(root).to_dict(), indent=2))


@gitnexus_app.command("context")
def gitnexus_context(
    symbol: str = typer.Argument(..., help="Symbol / FQID to inspect"),
    root: str = typer.Option(".", help="Repository root"),
):
    """Structural context for a symbol from the graph."""
    from sin_code_bundle import gitnexus

    try:
        gitnexus.ensure_index(root, auto=True)
        typer.echo(gitnexus.context(symbol, root=root))
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)


@gitnexus_app.command("impact")
def gitnexus_impact(
    symbol: str = typer.Argument(..., help="Symbol / FQID to analyze"),
    root: str = typer.Option(".", help="Repository root"),
):
    """Blast-radius impact analysis for a symbol."""
    from sin_code_bundle import gitnexus

    try:
        gitnexus.ensure_index(root, auto=True)
        typer.echo(gitnexus.impact(symbol, root=root))
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)


@gitnexus_app.command("ai-context")
def gitnexus_ai_context(
    task: str = typer.Argument(..., help="Task description to scope context to"),
    root: str = typer.Option(".", help="Repository root"),
):
    """Task-scoped, graph-aware context bundle for an agent."""
    from sin_code_bundle import gitnexus

    try:
        gitnexus.ensure_index(root, auto=True)
        typer.echo(gitnexus.ai_context(task, root=root))
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[GITNEXUS] {exc}", err=True)
        raise typer.Exit(code=1)


# --------------------------------------------------------------------------- #
# MarkItDown (document -> Markdown) bridge commands
# --------------------------------------------------------------------------- #
@markitdown_app.command("doctor")
def markitdown_doctor():
    """Check MarkItDown MCP/CLI availability."""
    from sin_code_bundle import markitdown

    typer.echo(json.dumps(markitdown.doctor(), indent=2))


@markitdown_app.command("setup")
def markitdown_setup(
    agents: str = typer.Option(
        "opencode,codex,hermes",
        help="Comma-separated agents to wire (opencode,codex,hermes).",
    ),
):
    """Wire the MarkItDown MCP server into each coder agent's config."""
    from sin_code_bundle import markitdown

    chosen = [a.strip() for a in agents.split(",") if a.strip()]
    try:
        written = markitdown.setup_agents(chosen)
    except markitdown.MarkItDownError as exc:
        typer.echo(f"[MARKITDOWN] {exc}", err=True)
        raise typer.Exit(code=1)
    for agent, path in written.items():
        typer.echo(f"[MARKITDOWN] wired {agent} -> {path}")
    typer.echo("[MARKITDOWN] Agents can now convert documents to Markdown via MCP.")


@markitdown_app.command("convert")
def markitdown_convert(
    path: Path = typer.Argument(..., help="Document to convert to Markdown"),
):
    """Convert a document (PDF/Office/image/...) to Markdown via the CLI."""
    from sin_code_bundle import markitdown

    try:
        typer.echo(markitdown.convert(str(path)))
    except markitdown.MarkItDownError as exc:
        typer.echo(f"[MARKITDOWN] {exc}", err=True)
        raise typer.Exit(code=1)


# --------------------------------------------------------------------------- #
# RTK (token-saving command proxy) bridge commands
# --------------------------------------------------------------------------- #
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


@app.command()
def preflight(
    root: str = typer.Argument(".", help="Repository root"),
    no_auto: bool = typer.Option(
        False, "--no-auto", help="Do not auto-index; only report."
    ),
):
    """Ensure agents are not coding blind: guarantee a fresh GitNexus index.

    Run this before any agent task. By default a missing or stale index is
    rebuilt automatically; with --no-auto it only reports state.
    """
    from sin_code_bundle import gitnexus

    try:
        state = gitnexus.ensure_index(root, auto=not no_auto)
    except gitnexus.GitNexusError as exc:
        typer.echo(f"[PREFLIGHT] BLOCKED: {exc}", err=True)
        raise typer.Exit(code=1)

    if not state.exists:
        typer.echo(
            "[PREFLIGHT] No GitNexus index and auto-index disabled. "
            "Run `sin gitnexus index` before coding.",
            err=True,
        )
        raise typer.Exit(code=1)
    if state.stale:
        typer.echo(
            f"[PREFLIGHT] WARNING: index is stale (age {state.age_seconds:.0f}s).",
            err=True,
        )
    typer.echo("[PREFLIGHT] OK - GitNexus graph context is ready.")
    typer.echo(json.dumps(state.to_dict(), indent=2))


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

    # GitNexus graph context (external npm tool). Always exposed so agents can
    # pull structural context / impact through the same MCP endpoint.
    try:
        from sin_code_bundle import gitnexus

        @mcp.tool()
        def gitnexus_context(symbol: str, root: str = ".") -> str:
            """Structural graph context for a symbol (auto-indexes if needed)."""
            gitnexus.ensure_index(root, auto=True)
            return gitnexus.context(symbol, root=root)

        @mcp.tool()
        def gitnexus_impact(symbol: str, root: str = ".") -> str:
            """Blast-radius impact analysis for a symbol (auto-indexes if needed)."""
            gitnexus.ensure_index(root, auto=True)
            return gitnexus.impact(symbol, root=root)

        @mcp.tool()
        def gitnexus_ai_context(task: str, root: str = ".") -> str:
            """Task-scoped, graph-aware context bundle (auto-indexes if needed)."""
            gitnexus.ensure_index(root, auto=True)
            return gitnexus.ai_context(task, root=root)
    except ImportError:
        pass

    # MarkItDown document conversion (external pip tool). Lets agents turn
    # PDFs / office docs / images into Markdown through the same MCP endpoint.
    try:
        from sin_code_bundle import markitdown

        @mcp.tool()
        def markitdown_convert(path: str) -> str:
            """Convert a document (PDF/DOCX/PPTX/XLSX/image/...) to Markdown."""
            return markitdown.convert(path)
    except ImportError:
        pass

    typer.echo("[SIN-BUNDLE] MCP server starting (stdio).", err=True)
    mcp.run()


if __name__ == "__main__":
    app()
