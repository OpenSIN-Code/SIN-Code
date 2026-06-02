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
codocs_app = typer.Typer(help="CoDocs - co-located docs standard (.doc.md companions).")
app.add_typer(codocs_app, name="codocs")

# SIN-Code Go Tools (new generation)
sin_code_app = typer.Typer(
    help="SIN-Code Go Tools - discovery, execution, mapping, grasping, scouting, harvesting, orchestration."
)
app.add_typer(sin_code_app, name="sin-code")

# Available SIN-Code Go binaries
_SIN_CODE_TOOLS = {
    "discover": "SIN-Code-Discover-Tool",
    "execute": "SIN-Code-Execute-Tool",
    "map": "SIN-Code-Map-Tool",
    "grasp": "SIN-Code-Grasp-Tool",
    "scout": "SIN-Code-Scout-Tool",
    "harvest": "SIN-Code-Harvest-Tool",
    "orchestrate": "SIN-Code-Orchestrate-Tool",
}


def _sin_code_tool_path(name: str) -> Path | None:
    """Return the path to a SIN-Code Go binary if it exists."""
    home_bin = Path.home() / ".local" / "bin" / name
    if home_bin.exists():
        return home_bin
    # Also check PATH
    from shutil import which
    w = which(name)
    return Path(w) if w else None

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
    # CoDocs ships inside the bundle itself, so it is always available.
    report["CoDocs (co-located docs)"] = True
    # SIN-Code Go tools
    for tool_name, repo_name in _SIN_CODE_TOOLS.items():
        path = _sin_code_tool_path(tool_name)
        report[f"sin-{tool_name} ({repo_name})"] = path is not None
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
@codocs_app.command("check")
def codocs_check(
    root: str = typer.Argument(".", help="Repository root to scan"),
    json_out: bool = typer.Option(False, "--json", help="Emit machine-readable JSON"),
):
    """Verify every `# Docs: x.doc.md` reference points to an existing file."""
    from sin_code_bundle import codocs

    broken = codocs.find_broken(root, exclude=set(_EXCLUDE))
    if json_out:
        typer.echo(json.dumps([ref.to_dict() for ref in broken], indent=2))
    else:
        if not broken:
            typer.echo("[CODOCS] OK - no broken .doc.md references.")
        else:
            for ref in broken:
                typer.echo(f"[CODOCS] MISSING: {ref.source} -> {ref.doc}")
            typer.echo(f"[CODOCS] {len(broken)} broken reference(s).")
    if broken:
        raise typer.Exit(code=1)


@codocs_app.command("check-inline")
def codocs_check_inline(
    root: str = typer.Argument(".", help="Repository root to scan"),
    json_out: bool = typer.Option(False, "--json", help="Emit machine-readable JSON"),
):
    """Check that code files have proper inline docs (Purpose header, etc.)."""
    from sin_code_bundle import codocs

    issues = codocs.check_inline_docs(root, exclude=set(_EXCLUDE))
    if json_out:
        typer.echo(codocs._check_inline_docs_json(root, exclude=set(_EXCLUDE)))
    else:
        if not issues:
            typer.echo("[CODOCS] OK - all files have Purpose header.")
        else:
            for issue in issues:
                typer.echo(f"[CODOCS] {issue.kind}: {issue.path} - {issue.detail}")
            typer.echo(f"[CODOCS] {len(issues)} inline doc issue(s).")
    if issues:
        raise typer.Exit(code=1)


@codocs_app.command("list")
def codocs_list(root: str = typer.Argument(".", help="Repository root to scan")):
    """List all discovered CoDocs references and whether they resolve."""
    from sin_code_bundle import codocs

    refs = codocs.scan(root, exclude=set(_EXCLUDE))
    if not refs:
        typer.echo("[CODOCS] No `Docs:` references found.")
        return
    for ref in refs:
        mark = "ok" if ref.exists else "MISSING"
        typer.echo(f"[{mark}] {ref.source} -> {ref.doc}")


@codocs_app.command("install-skill")
def codocs_install_skill(
    agent: str = typer.Option(
        "all", help="Which agent skill dir to install into: hermes | opencode | all"
    ),
):
    """Install the CoDocs skill into the local agent skill directory."""
    import shutil

    skill_src = Path(__file__).parent / "data" / "codocs" / "SKILL.md"
    if not skill_src.is_file():
        # Fallback to the repo-level skills/ dir (editable installs).
        skill_src = (
            Path(__file__).resolve().parents[2] / "skills" / "sin-codocs" / "SKILL.md"
        )
    if not skill_src.is_file():
        typer.echo("[CODOCS] Skill file not found in package.", err=True)
        raise typer.Exit(code=1)

    targets = {
        "hermes": Path.home() / ".hermes" / "skills" / "sin-codocs",
        "opencode": Path.home() / ".config" / "opencode" / "skills" / "sin-codocs",
    }
    chosen = targets.keys() if agent == "all" else [agent]
    for name in chosen:
        if name not in targets:
            typer.echo(f"[CODOCS] Unknown agent: {name}", err=True)
            raise typer.Exit(code=1)
        dest_dir = targets[name]
        dest_dir.mkdir(parents=True, exist_ok=True)
        shutil.copy2(skill_src, dest_dir / "SKILL.md")
        typer.echo(f"[CODOCS] Installed skill -> {dest_dir / 'SKILL.md'}")
@app.command(name="mcp-config")
def mcp_config(
    client: str = typer.Argument(..., help="Target CLI: opencode | codex | hermes"),
    full: bool = typer.Option(
        False, "--full", help="Generate config for all 15 individual tools"
    ),
    write: bool = typer.Option(
        False, "--write", help="Merge into the client's config file instead of stdout."
    ),
    path: Path = typer.Option(
        None, "--path", help="Override the config file path used with --write."
    ),
    stdout: bool = typer.Option(
        False, "--stdout", help="Write to stdout (default)."
    ),
):
    """Generate a ready-to-use MCP client configuration."""
    from . import mcp_config as gen

    client_norm = client.lower()
    if client_norm not in gen.SUPPORTED_CLIENTS:
        typer.echo(
            f"[SIN-BUNDLE] Unknown client '{client}'. "
            f"Supported: {', '.join(gen.SUPPORTED_CLIENTS)}",
            err=True,
        )
        raise typer.Exit(code=1)

    if write:
        target = path or gen.default_path(client_norm)
        try:
            if full:
                msg = gen.merge_full_into_file(client_norm, Path(target))
            else:
                msg = gen.merge_into_file(client_norm, Path(target))
        except ValueError as exc:
            typer.echo(f"[SIN-BUNDLE] {exc}", err=True)
            raise typer.Exit(code=1)
        typer.echo(f"[SIN-BUNDLE] {msg}")
    else:
        if full:
            typer.echo(gen.generate_full(client_norm))
        else:
            typer.echo(gen.generate(client_norm))


@app.command(name="agents-md")
def agents_md(
    path: Path = typer.Option(
        Path("AGENTS.md"), "--path", help="Target AGENTS.md path."
    ),
):
    """Create or idempotently update an AGENTS.md describing SIN tool usage."""
    from . import agents_md as gen

    msg = gen.upsert(Path(path))
    typer.echo(f"[SIN-BUNDLE] {msg}")


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
    # CoDocs is built into the bundle, so it is always exposed.
    from sin_code_bundle import codocs

    @mcp.tool()
    def codocs_check(root: str = ".") -> str:
        """Find broken co-located `.doc.md` references in a repository."""
        broken = codocs.find_broken(root, exclude=set(_EXCLUDE))
        return json.dumps(
            {
                "broken": [ref.to_dict() for ref in broken],
                "count": len(broken),
                "ok": not broken,
            }
        )

    # SIN-Brain memory tools (BR-1)
    try:
        from sin_brain import BrainCortex

        @mcp.tool()
        def recall(query: str, scope: str = "all", limit: int = 5) -> str:
            """Recall relevant memories from SIN-Brain."""
            try:
                cortex = BrainCortex(storage_path=".sin/brain.db")
                memories = cortex.recall(query, scope=scope, limit=limit)
                return json.dumps({"memories": [m.to_dict() for m in memories]})
            except ImportError:
                return json.dumps({"error": "SIN-Brain not installed"})

        @mcp.tool()
        def remember(content: str, kind: str = "observation", tier: str = "episodic", confidence: float = 1.0) -> str:
            """Store a memory in SIN-Brain."""
            try:
                cortex = BrainCortex(storage_path=".sin/brain.db")
                memory_id = cortex.remember(content, kind=kind, tier=tier, confidence=confidence)
                return json.dumps({"memory_id": memory_id, "status": "stored"})
            except ImportError:
                return json.dumps({"error": "SIN-Brain not installed"})

        @mcp.tool()
        def forget(memory_id: str) -> str:
            """Remove a memory from SIN-Brain."""
            try:
                cortex = BrainCortex(storage_path=".sin/brain.db")
                cortex.forget(memory_id)
                return json.dumps({"memory_id": memory_id, "status": "forgotten"})
            except ImportError:
                return json.dumps({"error": "SIN-Brain not installed"})

        @mcp.tool()
        def pin(memory_id: str) -> str:
            """Pin a memory to core tier for quick recall."""
            try:
                cortex = BrainCortex(storage_path=".sin/brain.db")
                cortex.pin(memory_id)
                return json.dumps({"memory_id": memory_id, "status": "pinned"})
            except ImportError:
                return json.dumps({"error": "SIN-Brain not installed"})

        @mcp.tool()
        def link_evidence(source_id: str, target_id: str, relation: str = "related") -> str:
            """Create evidence link between two memories."""
            try:
                cortex = BrainCortex(storage_path=".sin/brain.db")
                cortex.link_evidence(source_id, target_id, relation=relation)
                return json.dumps({"source_id": source_id, "target_id": target_id, "relation": relation, "status": "linked"})
            except ImportError:
                return json.dumps({"error": "SIN-Brain not installed"})
    except ImportError:
        pass

    typer.echo("[SIN-BUNDLE] MCP server starting (stdio).", err=True)
    mcp.run()


if __name__ == "__main__":
    app()

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


# --------------------------------------------------------------------------- #
# SIN-Code Go Tools commands
# --------------------------------------------------------------------------- #
@sin_code_app.command("run")
def sin_code_run(
    tool: str = typer.Argument(..., help="Tool name: discover, execute, map, grasp, scout, harvest, orchestrate"),
    args: list[str] = typer.Argument(default_factory=list, help="Arguments to pass to the tool"),
):
    """Run a SIN-Code Go tool with the given arguments."""
    if tool not in _SIN_CODE_TOOLS:
        typer.echo(f"[SIN-CODE] Unknown tool: {tool}. Available: {', '.join(_SIN_CODE_TOOLS.keys())}", err=True)
        raise typer.Exit(code=1)
    
    path = _sin_code_tool_path(tool)
    if not path:
        typer.echo(f"[SIN-CODE] Tool '{tool}' not found. Install: go install github.com/OpenSIN-Code/{_SIN_CODE_TOOLS[tool]}/cmd/{tool}@latest", err=True)
        raise typer.Exit(code=1)
    
    import subprocess
    result = subprocess.run([str(path)] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sin_code_app.command("agents-md")
def sin_code_agents_md(
    output: Path = typer.Option(Path("AGENTS.md"), help="Output file path"),
):
    """Generate AGENTS.md with SIN-Code Tool Suite rules for all agents."""
    content = '''# 🛠️ SIN-Code Tool Suite — IMMER UNSERE EIGENEN TOOLS NUTZEN (PRIORITY -10.0 - ABSOLUTE OBERSTE REGEL)

## ⚠️ ABSOLUTE REGEL: Niemals opencode-eigene Tools nutzen, wenn unsere SIN-Code Tools verfügbar sind!

**UNSERE 7 SIN-Code Tools sind BESSER als die opencode-eigenen Alternativen.** Jeder Agent MUSS unsere Tools nutzen, niemals die opencode-internen.

### Wann welches Tool?

| Aufgabe | SIN-Code Tool (NUTZEN!) | Opencode Alternative (NICHT nutzen!) | Warum unsere besser |
|---------|------------------------|-----------------------------------|-------------------|
| **Dateien suchen, Projekt-Struktur erkunden** | `sin-discover` | `opencode` interne Dateisuche | Pattern-Matching, Relevanz-Scoring, Dependency-Map, Gruppierung |
| **Befehle ausführen, Kommandos laufen lassen** | `sin-execute` | `opencode` interne Shell-Ausführung | Secret-Redaction, Safety-Checks, Timeout-Handling, Error-Analyse |
| **Architektur analysieren, Module mappen** | `sin-map` | `opencode` interne Code-Analyse | Module-Level, Entry-Points, Hot-Paths, Dependency-Graph, Orphan-Detection |
| **Einzelne Datei verstehen, Code analysieren** | `sin-grasp` | `opencode` interne Code-Analyse | Struktur, Dependencies, Usage, Context, Related-Files |
| **Code durchsuchen, Patterns finden** | `sin-scout` | `opencode` interne Suche | Regex, Semantic, Symbol, Usage-Search, Dead-Code-Detection |
| **URLs abrufen, APIs konsumieren** | `sin-harvest` | `opencode` interne HTTP-Requests | Caching, Struktur-Extraktion, Change-Detection, Auth-Management |
| **Tasks managen, Planung, Rollback** | `sin-orchestrate` | `opencode` interne Task-Planung | Dependencies, Parallel-Execution, Blocker-Detection, Rollback-Plan |

### Tool-Verweisung & Skills/MCP

| Tool | MCP Name | GitHub Repo | Skill | Installiert |
|------|----------|-------------|-------|-------------|
| `sin-discover` | `sin-discover` | `OpenSIN-Code/SIN-Code-Discover-Tool` | `sin-discover` | ✅ `~/.local/bin/discover` |
| `sin-execute` | `sin-execute` | `OpenSIN-Code/SIN-Code-Execute-Tool` | `sin-execute` | ✅ `~/.local/bin/execute` |
| `sin-map` | `sin-map` | `OpenSIN-Code/SIN-Code-Map-Tool` | `sin-map` | ✅ `~/.local/bin/map` |
| `sin-grasp` | `sin-grasp` | `OpenSIN-Code/SIN-Code-Grasp-Tool` | `sin-grasp` | ✅ `~/.local/bin/grasp` |
| `sin-scout` | `sin-scout` | `OpenSIN-Code/SIN-Code-Scout-Tool` | `sin-scout` | ✅ `~/.local/bin/scout` |
| `sin-harvest` | `sin-harvest` | `OpenSIN-Code/SIN-Code-Harvest-Tool` | `sin-harvest` | ✅ `~/.local/bin/harvest` |
| `sin-orchestrate` | `sin-orchestrate` | `OpenSIN-Code/SIN-Code-Orchestrate-Tool` | `sin-orchestrate` | ✅ `~/.local/bin/orchestrate` |

### Anwendungsbeispiele

**1. Neues Projekt erkunden:**
```bash
# NIEMALS opencode-interne Dateisuche nutzen!
/Users/jeremy/.local/bin/discover -path /Users/jeremy/dev/NEUES-PROJEKT -pattern "**/*.py" -sort_by relevance -format json
# Ergebnis: Alle Python-Dateien absteigend nach Relevanz sortiert, mit Dependencies und Related-Files
```

**2. Befehle sicher ausführen:**
```bash
# NIEMALS opencode-interne Shell-Ausführung nutzen!
/Users/jeremy/.local/bin/execute -command "npm test" -timeout 60 -format json
# Ergebnis: Safety-Check, Secret-Redaction, Error-Analyse, Timeout-Handling
```

**3. Architektur verstehen:**
```bash
# NIEMALS opencode-interne Code-Analyse nutzen!
/Users/jeremy/.local/bin/map -path /Users/jeremy/dev/PROJEKT -action map -format json
# Ergebnis: Module, Entry-Points, Hot-Paths, Dependency-Graph, Orphan-Detection, Complexity
```

**4. Code durchsuchen:**
```bash
# NIEMALS opencode-interne Suche nutzen!
/Users/jeremy/.local/bin/scout -query "func.*main" -path /Users/jeremy/dev/PROJEKT -search_type regex -format json
# Ergebnis: Alle Main-Funktionen, mit Usage-Count, Architecture-Info, Context
```

**5. API/URL abrufen:**
```bash
# NIEMALS opencode-interne HTTP-Requests nutzen!
/Users/jeremy/.local/bin/harvest -url "https://api.example.com/data" -format json
# Ergebnis: Status, Body, Headers, Struktur-Extraktion, Caching
```

**6. Tasks planen & tracken:**
```bash
# NIEMALS opencode-interne Task-Planung nutzen!
/Users/jeremy/.local/bin/orchestrate -action add -title "Feature implementieren" -tags "urgent" -format json
# Ergebnis: Task-ID, Dependencies, Parallel-Execution-Plan, Rollback-Plan
```

### Regeln

- **SOFORTIGER PERMANENTER BAN** bei Nutzung von opencode-internen Tools statt unserer SIN-Code Tools
- **VOR jedem Coding-Task:** Erst mit `sin-discover` das Projekt erkunden, dann mit `sin-grasp` die relevante Datei verstehen
- **VOR jedem Refactoring:** Erst mit `sin-map` die Architektur analysieren, dann mit `sin-scout` alle Referenzen finden
- **VOR jedem Befehl:** Immer `sin-execute` mit Safety-Check und Secret-Redaction nutzen
- **VOR jedem API-Call:** Immer `sin-harvest` mit Caching und Struktur-Extraktion nutzen
- **Für jede Planung:** Immer `sin-orchestrate` mit Dependencies und Rollback-Plan nutzen

### Versionen (aktuell)

| Tool | Version | Status |
|------|---------|--------|
| sin-discover | v0.2.5-fixes | ✅ Stable |
| sin-execute | v0.2.4-fixes | ✅ Stable |
| sin-map | v0.2.5-fixes | ✅ Stable |
| sin-grasp | v0.2.4-fixes | ✅ Stable |
| sin-scout | v0.1.5-fixes | ✅ Stable |
| sin-harvest | v0.1.4-fixes | ✅ Stable |
| sin-orchestrate | v0.1.6-fixes | ✅ Stable |

---

'''
    output.write_text(content, encoding="utf-8")
    typer.echo(f"[SIN-CODE] Generated {output}")


if __name__ == "__main__":
    app()
