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


if __name__ == "__main__":
    app()
