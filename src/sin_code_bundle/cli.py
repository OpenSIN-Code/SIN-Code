"""Unified CLI für den gesamten SIN-Code Stack."""
import json
import typer
from pathlib import Path

app = typer.Typer(help="SIN-Code Bundle - Unified SOTA Agent-Engineering Stack")


@app.command()
def bootstrap(repo: str = "."):
    """Initialize all 5 subsystems for a repository."""
    typer.echo(f"[SIN-BUNDLE] Bootstrapping {repo}...")
    # 1. Build knowledge graph
    from sin_code_sckg.graph import KnowledgeGraph
    kg = KnowledgeGraph(storage_path=f"{repo}/.sin/knowledge.graph")
    stats = kg.build_from_repo(repo, exclude=["venv", "node_modules", ".git"])
    typer.echo(f"[SIN-BUNDLE] SCKG built: {json.dumps(stats)}")

    # 2. Baseline complexity
    from sin_code_adw.complexity import ComplexityAnalyzer
    analyzer = ComplexityAnalyzer()
    reports = analyzer.analyze(repo, exclude={"venv", "node_modules", ".git"})
    baseline = analyzer.debt_score(reports)
    Path(f"{repo}/.sin/baseline.json").write_text(json.dumps(baseline, indent=2))
    typer.echo(f"[SIN-BUNDLE] ADW baseline: {json.dumps(baseline)}")

    # 3. Init cost tracker
    from sin_code_adw.cost_tracker import CostTracker
    CostTracker(log_path=f"{repo}/.sin/costs.jsonl")
    typer.echo("[SIN-BUNDLE] Cost tracker initialized")

    typer.echo(f"[SIN-BUNDLE] ✓ Bootstrap complete. Run `sin serve` to expose via MCP.")


@app.command()
def review(file_a: Path, file_b: Path):
    """Semantic review of a change (IBD + SCKG impact)."""
    from sin_code_ibd import ASTDiff, IntentSummarizer, RiskScorer
    ad = ASTDiff()
    changes = ad.diff_files(str(file_a), str(file_b))
    intents = IntentSummarizer().summarize(changes)
    risk = RiskScorer().score(changes)
    typer.echo(json.dumps({
        "intents": [i.__dict__ for i in intents],
        "risk": risk,
    }, indent=2))


@app.command()
def verify(module: Path, function: str):
    """Proof-of-correctness for a function."""
    from sin_code_poc.cli import verify as poc_verify
    poc_verify(module, function)


@app.command()
def mock(name: str, apis: list[str] = typer.Option([], "--api"), test_cmd: str = "pytest"):
    """Spin up ephemeral mocks and run tests."""
    from sin_code_efsm.cli import setup
    setup(name, apis, False, test_cmd)


@app.command()
def debt(root: str = "."):
    """Show current architectural debt."""
    from sin_code_adw.cli import scan
    scan(root)


@app.command()
def serve(port: int = 9000):
    """Expose all tools as a unified MCP server."""
    try:
        from mcp.server.fastmcp import FastMCP
    except ImportError:
        typer.echo("[SIN-BUNDLE] mcp package required")
        return
    mcp = FastMCP("sin-code-bundle")

    # Register tools from all subsystems
    from sin_code_sckg.graph import KnowledgeGraph
    from sin_code_ibd import ASTDiff, IntentSummarizer, RiskScorer
    from sin_code_adw.complexity import ComplexityAnalyzer

    @mcp.tool()
    def impact(symbol_fqid: str) -> str:
        kg = KnowledgeGraph(storage_path="./.sin/knowledge.graph")
        return json.dumps(kg.impact_analysis(symbol_fqid))

    @mcp.tool()
    def semantic_diff(file_a: str, file_b: str) -> str:
        ad = ASTDiff()
        changes = ad.diff_files(file_a, file_b)
        intents = IntentSummarizer().summarize(changes)
        risk = RiskScorer().score(changes)
        return json.dumps({"intents": [i.__dict__ for i in intents], "risk": risk})

    @mcp.tool()
    def architectural_debt() -> str:
        a = ComplexityAnalyzer()
        r = a.analyze(".", exclude={"venv", "node_modules", ".git"})
        return json.dumps(a.debt_score(r))

    typer.echo(f"[SIN-BUNDLE] MCP server on port {port}")
    mcp.run()


if __name__ == "__main__":
    app()
