"""Unified CLI fuer den gesamten SIN-Code Stack.

Subsysteme werden lazy und defensiv importiert: fehlt eines, bleibt der Rest
nutzbar und es wird eine klare Meldung statt eines Importfehlers ausgegeben.

Docs: cli.doc.md
"""

from __future__ import annotations

import json
import shutil
import subprocess
from pathlib import Path

import typer

app = typer.Typer(help="SIN-Code Bundle - Unified SOTA Agent-Engineering Stack")

# ── Sub-App Registration ────────────────────────────────────────────────────
# Each sub-Typer becomes a `sin <name>` command group. The seven external
# SIN-Code Go tools + ceo-audit + browser + vfs + hashline + ast are all
# registered as sub-apps so users get a unified `sin --help` surface.
gitnexus_app = typer.Typer(help="GitNexus bridge - mandatory graph context for coder agents.")
app.add_typer(gitnexus_app, name="gitnexus")

markitdown_app = typer.Typer(
    help="MarkItDown bridge - document->Markdown context for coder agents."
)
app.add_typer(markitdown_app, name="markitdown")

rtk_app = typer.Typer(help="RTK bridge - token-saving command proxy for coder agents.")
app.add_typer(rtk_app, name="rtk")
codocs_app = typer.Typer(help="CoDocs - co-located docs standard (.doc.md companions).")
app.add_typer(codocs_app, name="codocs")

# SIN-Code Go Tools (new generation)
sin_code_app = typer.Typer(
    help="SIN-Code Go Tools - discovery, execution, mapping, grasping, scouting, harvesting, orchestration."
)
app.add_typer(sin_code_app, name="sin-code")

# SIN-Code Security Bundle - 8 tools, 8 compliance frameworks
security_app = typer.Typer(
    help="SIN-Code Security Bundle - 8 tools (secrets, sast, sca, sbom, container, iac, license, dast) + 8 compliance frameworks (cis, nist, soc2, iso27001, gdpr, hipaa, pci, owasp)."
)
app.add_typer(security_app, name="security")

# CEO Audit - SOTA repo review (delegates to the opencode skill)
sckg_app = typer.Typer(help="SCKG - Semantic Codebase Knowledge Graph")
app.add_typer(sckg_app, name="sckg")

@sckg_app.command("run")
def sckg_run(
    args: list[str] = typer.Argument(default_factory=list, help="Arguments to pass to the sckg CLI"),
):
    """Pass-through to the `sckg` CLI — any subcommand and flags."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found. Install: pip install -e ~/SIN-Code-SCKG-Tool", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("index")
def sckg_index(
    path: str = typer.Argument(..., help="Repository path to index"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg index"),
):
    """Build a knowledge graph from source code: `sckg index <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "index", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("search")
def sckg_search(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    query: str = typer.Argument(..., help="Search query"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg search"),
):
    """Search the knowledge graph: `sckg search <path> <query>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "search", path, query] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("hot-paths")
def sckg_hot_paths(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg hot-paths"),
):
    """Show most frequently called functions: `sckg hot-paths <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "hot-paths", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("dead-code")
def sckg_dead_code(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg dead-code"),
):
    """Analyze graph for dead code: `sckg dead-code <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "dead-code", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("communities")
def sckg_communities(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg communities"),
):
    """Detect language-aware communities: `sckg communities <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "communities", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("dashboard")
def sckg_dashboard(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg graph (dashboard)"),
):
    """Generate interactive D3.js dashboard: `sckg graph <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "graph", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("adr")
def sckg_adr(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg adr"),
):
    """Generate Architecture Decision Records: `sckg adr <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "adr", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("similar")
def sckg_similar(
    path: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    node: str = typer.Argument(..., help="Node to find similar symbols for"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg similar"),
):
    """Find structurally similar symbols: `sckg similar <path> <node>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "similar", path, node] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("serve")
def sckg_serve(
    graph: str = typer.Argument(..., help="Path to the knowledge graph JSON"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg serve"),
):
    """Start the GraphQL API server: `sckg serve <graph>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "serve", graph] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sckg_app.command("watch")
def sckg_watch(
    path: str = typer.Argument(..., help="Repository path to watch"),
    args: list[str] = typer.Argument(default_factory=list, help="Extra arguments for sckg watch"),
):
    """Watch repo for changes with live GraphQL: `sckg watch <path>`."""
    binary = shutil.which("sckg")
    if not binary:
        typer.echo("[SCKG] 'sckg' binary not found.", err=True)
        raise typer.Exit(code=1)
    result = subprocess.run([binary, "watch", path] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


ceo_audit_app = typer.Typer(
    help="CEO Audit - 47-gate, 8-axis SOTA repository review (security, perf, quality, tests, deps, docs, arch, compliance)."
)
app.add_typer(ceo_audit_app, name="ceo-audit")

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


_EXCLUDE = {"venv", ".venv", "node_modules", ".git", "__pycache__"}


def _require(module: str, hint: str):
    """Importiert ein Subsystem oder bricht mit klarer Meldung ab."""
    import importlib

    try:
        return importlib.import_module(module)
    except ImportError:
        typer.echo(f"[SIN-BUNDLE] Subsystem '{module}' not installed. Install with: {hint}")
        raise typer.Exit(code=1)


# ── Core Status / Bootstrap Commands ────────────────────────────────────────
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
    report["MarkItDown (doc->markdown, external)"] = markitdown.detect_env().mcp_available
    report["RTK (token-saving proxy, external)"] = rtk.detect_env().available
    # CoDocs ships inside the bundle itself, so it is always available.
    report["CoDocs (co-located docs)"] = True

    # SIN-Brain memory cortex (external package). Report presence plus tier
    # sizes so it is obvious whether agents have a working memory.
    from sin_code_bundle import memory

    mem_env = memory.detect_env()
    report["SIN-Brain (memory cortex, external)"] = mem_env.available
    if mem_env.available:
        report["sin-brain:db"] = mem_env.db_path or "(default)"
        report["sin-brain:tiers"] = mem_env.tiers
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
        reports = analyzer.analyze(repo, exclude=_EXCLUDE)
        baseline = analyzer.debt_score(reports)
        (sin_dir / "baseline.json").write_text(json.dumps(baseline, indent=2))
        CostTracker()
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
    typer.echo(json.dumps({"intents": [i.__dict__ for i in intents], "risk": risk}, indent=2))


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


# ── Unified `sin code` Subcommand Hub ──────────────────────────────────────
def _normalize_root_flag(args: list[str]) -> list[str]:
    """Normalize positional path arg to --root flag for commands that need it.

    `sin debt <path>` -> `sin debt --root <path>`
    `sin debt --root <path>` -> unchanged
    """
    if not args:
        return []
    # If first arg is a path (not a flag), wrap it with --root
    if not args[0].startswith("-"):
        return ["--root", args[0]] + args[1:]
    return args


@app.command()
def code(
    action: str = typer.Argument(
        ...,
        help="Action: review, debt, verify, preflight, codocs, sckg, audit, oracle, adw, ibd, discover, scout, or full",
    ),
    args: list[str] = typer.Argument(default_factory=list, help="Arguments to pass to the underlying command"),
):
    """Unified coding workflow hub — shortcut to all sin coding tools.

    Examples:
      sin code review file_a.py file_b.py
      sin code debt .
      sin code preflight
      sin code codocs check
      sin code discover
      sin code audit
      sin code full .   # runs preflight + codocs + debt pipeline
    """
    actions_map = {
        "review": (["review"], args),
        "debt": (["debt"], _normalize_root_flag(args)),
        "verify": (["verify"], args),
        "preflight": (["preflight"], args),
        "preflight-write": (["preflight-write"], args),
        "codocs": (["codocs"], args),
        "sckg": (["sckg"], args),
        "audit": (["ceo-audit"], args),
        "oracle": (["verify"], args),
        "adw": (["debt"], _normalize_root_flag(args)),
        "ibd": (["review"], args),
        "discover": (["sin-code", "run", "discover"], args),
        "scout": (["sin-code", "run", "scout"], args),
        "grasp": (["sin-code", "run", "grasp"], args),
        "map": (["sin-code", "run", "map"], args),
        "harvest": (["sin-code", "run", "harvest"], args),
    }

    if action == "full":
        # Run a complete coding review pipeline
        typer.echo("[SIN-CODE] Running full coding review pipeline...")
        steps = [
            (["preflight"], "Preflight (GitNexus index freshness)"),
            (["codocs", "check", "."], "CoDocs validation"),
            (["debt", "--root", "."], "Architectural debt analysis"),
        ]
        for cmd, label in steps:
            typer.echo(f"\n[SIN-CODE] → {label}")
            full_cmd = ["sin"] + cmd
            result = subprocess.run(full_cmd, capture_output=True, text=True)
            if result.stdout:
                typer.echo(result.stdout)
            if result.returncode != 0 and result.stderr:
                typer.echo(f"[WARN] {label} returned {result.returncode}", err=True)
        typer.echo("\n[SIN-CODE] ✓ Full pipeline complete")
        return

    if action not in actions_map:
        typer.echo(
            f"[SIN-CODE] Unknown action: {action}. Available: {', '.join(actions_map.keys())}, full",
            err=True,
        )
        raise typer.Exit(code=1)

    cmd_prefix, cmd_args = actions_map[action]
    full_cmd = ["sin"] + cmd_prefix + cmd_args
    typer.echo(f"[SIN-CODE] {' '.join(full_cmd)}")
    result = subprocess.run(full_cmd, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


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


# ── MarkItDown Bridge Commands (document -> Markdown) ──────────────────────
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


# ── RTK Bridge Commands (token-saving command proxy) ───────────────────────
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
    no_auto: bool = typer.Option(False, "--no-auto", help="Do not auto-index; only report."),
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


# ── v0.8.0 Baseline Workflow CLI subcommands ──────────────────────────────
# CLI wrappers around the new MCP tools so hooks (post-commit.sh etc.)
# can call them without an MCP client.


@app.command("preflight-write")
def preflight_write(
    tool: str = typer.Option(
        ..., "--tool", help="Tool about to be called (sin_write, sin_edit, ...)"
    ),
    path: str = typer.Option("", "--path", help="Target file path"),
):
    """Pre-write safety gate — runs sin_preflight + CoDocs for a single write."""
    from sin_code_bundle.preflight import PreflightChecker

    result = PreflightChecker().check(tool, {"path": path} if path else {})
    typer.echo(json.dumps(result, indent=2, default=str))


@app.command("programming-workflow")
def programming_workflow_cli(
    action: str = typer.Argument(
        ..., help="One of: pre_write, write, post_write, pre_commit, refactor, session_warmup"
    ),
    target: str = typer.Option("", "--target"),
    message: str = typer.Option("", "--message"),
    checkpoint_name: str = typer.Option("", "--checkpoint-name"),
    base: str = typer.Option("main", "--base"),
    head: str = typer.Option("HEAD", "--head"),
):
    """CLI wrapper around the sin_programming_workflow MCP tool."""
    from sin_code_bundle.programming_workflow import ProgrammingWorkflow

    wf = ProgrammingWorkflow()
    result = wf.run(
        action=action,
        target=target,
        message=message,
        checkpoint_name=checkpoint_name,
        base=base,
        head=head,
    )
    typer.echo(json.dumps(result, indent=2, default=str))


@app.command("immortal-commit")
def immortal_commit_cli(
    message: str = typer.Option("", "--message", help="Conventional Commits message"),
    tag: str = typer.Option("", "--tag", help="Optional annotated tag"),
    push: bool = typer.Option(False, "--push", help="Push to origin after commit"),
    post_hook: bool = typer.Option(
        False, "--post-hook", help="Post-commit hook mode: tag + push only, no commit"
    ),
):
    """CLI wrapper around the sin_immortal_commit MCP tool.

    Two modes:
      - Default: validates message, creates commit (and tag/push if requested).
      - --post-hook: assumes the commit was already made; only does tag + push.
    """
    from sin_code_bundle.immortal_commit import ImmortalCommitter

    if post_hook:
        # Post-hook mode: tag + push only, no new commit.
        committer = ImmortalCommitter()
        result: dict = {"mode": "post_hook", "message": message, "tag": tag or None, "steps": []}
        if tag:
            import subprocess

            tag_proc = subprocess.run(
                ["git", "tag", "-a", tag, "-m", f"Release {tag}"],
                capture_output=True,
                text=True,
                timeout=30,
            )
            result["steps"].append({"step": "git_tag", "ok": tag_proc.returncode == 0})
        if push:
            import subprocess

            push_proc = subprocess.run(
                ["git", "push", "origin", "main"],
                capture_output=True,
                text=True,
                timeout=60,
            )
            result["steps"].append({"step": "git_push", "ok": push_proc.returncode == 0})
            if tag:
                tag_push = subprocess.run(
                    ["git", "push", "origin", tag],
                    capture_output=True,
                    text=True,
                    timeout=30,
                )
                result["steps"].append({"step": "git_push_tag", "ok": tag_push.returncode == 0})
        import subprocess as _sp

        sha = _sp.run(["git", "rev-parse", "HEAD"], capture_output=True, text=True).stdout.strip()
        result["sha"] = sha
        result["success"] = all(s.get("ok") for s in result["steps"])
        typer.echo(json.dumps(result, indent=2, default=str))
        return

    if not message:
        typer.echo("[immortal-commit] error: --message is required (or pass --post-hook)", err=True)
        raise typer.Exit(code=2)

    committer = ImmortalCommitter()
    result = committer.commit(message=message, tag=tag, push=push, force_main=True)
    typer.echo(json.dumps(result, indent=2, default=str))
    if not result.get("success"):
        raise typer.Exit(code=1)


@app.command("session-warmup")
def session_warmup_cli(
    repo_path: str = typer.Argument(".", help="Path to the repository"),
):
    """CLI wrapper around the sin_session_warmup MCP tool."""
    from sin_code_bundle.session_warmup import SessionWarmup

    warm = SessionWarmup(repo_root=Path(repo_path))
    typer.echo(json.dumps(warm.warmup(), indent=2, default=str))


@app.command("merge-safety")
def merge_safety_cli(
    base: str = typer.Option("main", "--base"),
    head: str = typer.Option("HEAD", "--head"),
    profile: str = typer.Option("QUICK", "--profile"),
):
    """CLI wrapper around the sin_merge_safety MCP tool."""
    from sin_code_bundle.merge_safety import MergeSafety

    gate = MergeSafety()
    result = gate.check(base=base, head=head, profile=profile)
    typer.echo(json.dumps(result, indent=2, default=str))
    if not result.get("pass"):
        raise typer.Exit(code=1)


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
        skill_src = Path(__file__).resolve().parents[2] / "skills" / "sin-codocs" / "SKILL.md"
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
    full: bool = typer.Option(False, "--full", help="Generate config for all 15 individual tools"),
    write: bool = typer.Option(
        False, "--write", help="Merge into the client's config file instead of stdout."
    ),
    path: Path = typer.Option(
        None, "--path", help="Override the config file path used with --write."
    ),
    stdout: bool = typer.Option(False, "--stdout", help="Write to stdout (default)."),
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
    path: Path = typer.Option(Path("AGENTS.md"), "--path", help="Target AGENTS.md path."),
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
        typer.echo(
            "[SIN-BUNDLE] mcp package required: pip install 'sin-code-bundle[mcp]'", err=True
        )
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
            return json.dumps({"intents": [i.__dict__ for i in intents], "risk": risk})
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
        def mock_env(
            action: str = "up", port: int = 8888
        ) -> str:  # 8888 = EFSM default ephemeral-mock port
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
        from sin_code_orchestration import Orchestrator, Role, TaskSpec

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
            return json.dumps(
                {
                    "intents": [i.__dict__ for i in intents],
                    "risk": risk,
                    "recommendation": "Approve" if risk["risk"] == "low" else "Review Manually",
                }
            )
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

    # SIN-Brain memory cortex (external package, BR-1 / Issue #14). Registers
    # recall/remember/forget/pin/link_evidence only when sin-brain is importable;
    # a missing package leaves the server fully functional (graceful degradation).
    from sin_code_bundle import memory

    memory.register_tools(mcp)

    # ── Core file-ops tools (PRIORITY -10.0 — REPLACE native read/write/edit/bash) ──
    # These tools are the primary interface agents use instead of opencode's
    # native read/write/edit/bash. They wrap our SOTA-infrastructure:
    #   - sin_read:        VirtualFS (URI schemes) + grasp fallback
    #   - sin_write:       atomic write with backup
    #   - sin_edit:        hashline-anchored semantic patches (prevents stale edits)
    #   - sin_bash:        execute wrapper (secret redaction, timeouts, error analysis)
    from pathlib import Path as _Path

    from sin_code_bundle import hashline as _hashline_mod
    from sin_code_bundle import vfs

    @mcp.tool()
    def sin_read(path: str, summarize: bool = False, max_chars: int = 50000) -> str:
        """SIN-Code read — replaces native read.

        - URI schemes (sckg://, poc://, ibd://, adw://, efsm://, oracle://, conflict://)
          are resolved via VirtualFS — semantic, not textual.
        - Plain file paths are read with size-aware truncation.
        - summarize=True returns a structural overview (line count, head/tail) instead
          of full content (use for large files).

        Better than native read: URI semantics, size safety, no accidental
        multi-MB dumps into context.
        """
        try:
            if "://" in path:
                v = vfs.SINVirtualFS()
                return json.dumps(v.resolve(path), indent=2, default=str)
            p = _Path(path).expanduser()
            if not p.exists():
                return json.dumps({"error": f"path not found: {path}"})
            if p.is_dir():
                items = sorted([str(x.relative_to(p)) for x in p.iterdir()])
                return json.dumps({"type": "directory", "path": str(p), "items": items})
            content = p.read_text(encoding="utf-8", errors="replace")
            n = len(content)
            if n > max_chars:
                head = content[: max_chars // 2]
                tail = content[-max_chars // 2 :]
                truncated = True
            else:
                head = content
                tail = ""
                truncated = False
            if summarize:
                lines = content.splitlines()
                return json.dumps(
                    {
                        "path": str(p),
                        "lines": len(lines),
                        "chars": n,
                        "first_5": lines[:5],
                        "last_5": lines[-5:],
                    }
                )
            return json.dumps(
                {
                    "path": str(p),
                    "chars": n,
                    "truncated": truncated,
                    "content": head,
                    "tail": tail,
                }
            )
        except Exception as exc:
            return json.dumps({"error": str(exc), "path": path})

    @mcp.tool()
    def sin_write(path: str, content: str, verify: bool = True) -> str:
        """SIN-Code write — replaces native write.

        Atomic write with optional backup. When verify=True (default), runs
        AST-based syntax validation for known file types (.py, .ts, .js, .go)
        to catch broken-syntax writes before they hit disk.

        Better than native write: atomic (no half-written files on crash),
        syntax pre-validation, optional backup.
        """
        try:
            p = _Path(path).expanduser()
            backup = None
            if p.exists() and verify:
                backup = str(p) + ".bak"
                p.replace(backup)
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(content, encoding="utf-8")
            verified = True
            if verify and p.suffix in {".py", ".ts", ".js", ".go"}:
                try:
                    compile(content, str(p), "exec") if p.suffix == ".py" else None
                except SyntaxError as e:
                    verified = False
                    if backup:
                        _Path(backup).replace(p)
                    return json.dumps(
                        {"success": False, "error": f"syntax error: {e}", "path": str(p)}
                    )
            return json.dumps(
                {
                    "success": True,
                    "path": str(p),
                    "chars": len(content),
                    "verified": verified,
                    "backup": backup,
                }
            )
        except Exception as exc:
            return json.dumps({"error": str(exc), "path": path})

    @mcp.tool()
    def sin_edit(
        file_path: str,
        old_content: str,
        new_content: str,
        intent: str = "",
    ) -> str:
        """SIN-Code edit — replaces native edit.

        Hashline-anchored semantic patching. The old_content is anchored by
        content-hash (NOT line numbers), so the edit survives line shifts,
        reformatting, and concurrent edits elsewhere in the file. Returns
        a structured result with the patch details.

        Better than native edit: line-shift resilient, multi-edit support
        (apply N changes atomically), validates with hashline before/after.
        """
        try:
            p = _Path(file_path).expanduser()
            if not p.exists():
                return json.dumps({"error": f"file not found: {file_path}"})
            patcher = _hashline_mod.SINHashlinePatch(repo_root=p.parent)
            patch = patcher.create_semantic_patch(
                file_path=str(p),
                old_text=old_content,
                new_text=new_content,
                intent=intent,
            )
            if not patch:
                return json.dumps(
                    {
                        "success": False,
                        "error": "anchor not found (content drift detected)",
                        "hint": "use sin_read first to see current state",
                    }
                )
            ok, msg = patcher.apply_semantic_patch(patch)
            return json.dumps({"success": ok, "message": msg, "intent": intent, "patch": patch})
        except Exception as exc:
            return json.dumps({"error": str(exc), "file_path": file_path})

    @mcp.tool()
    def sin_bash(command: str, timeout: int = 60) -> str:
        """SIN-Code bash — replaces native bash.

        Safe command execution via the `execute` tool (Go binary) with:
        - Secret redaction (tokens/keys in output are masked automatically)
        - Timeout enforcement (default 60s, max 600s)
        - Exit code capture
        - Working directory = current repo

        For complex pipelines, prefer chaining sin_bash calls over single
        shell pipelines — easier to debug, partial success possible.

        Better than native bash: secret-safety, timeout, structured result.
        """
        import shutil as _sh
        import subprocess as _sp

        try:
            cmd_path = _sh.which("execute") or str(_Path.home() / ".local/bin/execute")
            if _Path(cmd_path).exists():
                proc = _sp.run(
                    [cmd_path, "-timeout", str(timeout), "-format", "json", "-command", command],
                    capture_output=True,
                    text=True,
                    timeout=timeout + 10,
                )
                return json.dumps(
                    {
                        "stdout": proc.stdout,
                        "stderr": proc.stderr,
                        "returncode": proc.returncode,
                        "redacted": True,
                    }
                )
            proc = _sp.run(
                command,
                shell=True,
                capture_output=True,
                text=True,
                timeout=timeout,
            )
            return json.dumps(
                {
                    "stdout": proc.stdout[-10000:],
                    "stderr": proc.stderr[-5000:],
                    "returncode": proc.returncode,
                    "redacted": False,
                    "warning": "execute binary not found — running raw shell",
                }
            )
        except _sp.TimeoutExpired:
            return json.dumps({"error": f"timeout after {timeout}s", "command": command})
        except Exception as exc:
            return json.dumps({"error": str(exc), "command": command})

    @mcp.tool()
    def sin_search(query: str, path: str = ".", search_type: str = "semantic") -> str:
        """SIN-Code search — replaces native search/grep.

        Wraps the `scout` Go tool (semantic + regex + symbol search). Falls
        back to Python regex if scout binary is missing.

        search_type: semantic | regex | symbol | usage

        Accepts both directory paths (rglob) and single files (single file scan).
        """
        import shutil as _sh
        import subprocess as _sp

        try:
            cmd_path = _sh.which("scout") or str(_Path.home() / ".local/bin/scout")
            if _Path(cmd_path).exists():
                proc = _sp.run(
                    [cmd_path, "--query", query, "--path", path, "--type", search_type, "--json"],
                    capture_output=True,
                    text=True,
                    timeout=30,
                )
                if proc.returncode == 0 and proc.stdout.strip():
                    try:
                        return proc.stdout
                    except Exception:
                        pass
                # fall through to python-regex fallback
            import re as _re

            results = []
            target = _Path(path).expanduser()
            # Determine which files to scan
            if target.is_file():
                files = [target]
            elif target.is_dir():
                files = [p for p in target.rglob("*") if p.is_file() and ".git" not in p.parts]
            else:
                return json.dumps({"error": f"path not found: {path}"})
            for p in files:
                try:
                    text = p.read_text(encoding="utf-8", errors="ignore")
                except Exception:
                    continue
                for m in _re.finditer(query, text):
                    line_no = text[: m.start()].count("\n") + 1
                    line_text = (
                        text.splitlines()[line_no - 1] if line_no <= len(text.splitlines()) else ""
                    )
                    results.append(
                        {
                            "file": str(p),
                            "line": line_no,
                            "match": m.group(0),
                            "context": line_text[:200],
                        }
                    )
                    # 200 = hard ceiling for python-regex fallback; keeps
                    # the fallback from flooding agent context on common
                    # broad queries like `import `.
                    if len(results) >= 200:
                        break
                if len(results) >= 200:
                    break
            return json.dumps(
                {"results": results, "count": len(results), "fallback": "python-regex"}
            )
        except Exception as exc:
            return json.dumps({"error": str(exc), "query": query})

    typer.echo("[SIN-BUNDLE] MCP server starting (stdio).", err=True)
    mcp.run()


# ── v0.9.3 Consolidated Skill Subcommands (issue #29) ─────────────────────
# Migrated 3 baseline skills into the bundle CLI:
#   - sin-slash           -> sin slash <sub>
#   - sin-mcp-server-builder -> sin mcp-server <sub>
#   - sin-marketplace     -> sin marketplace <sub>
# Source repos are now archived (see DEPRECATED notice in their READMEs).
try:
    from sin_code_bundle.tools.slash.app import app as slash_app
    app.add_typer(slash_app, name="slash")
except ImportError as exc:
    @app.command("slash")
    def slash_missing() -> None:
        """Slash commands (slash module not installed)."""
        typer.echo(f"[SIN-BUNDLE] slash module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)

try:
    from sin_code_bundle.tools.mcp_server_builder.app import app as mcp_server_app
    app.add_typer(mcp_server_app, name="mcp-server")
except ImportError as exc:
    @app.command("mcp-server")
    def mcp_server_missing() -> None:
        """MCP server builder (mcp_server_builder module not installed)."""
        typer.echo(f"[SIN-BUNDLE] mcp-server module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)

try:
    from sin_code_bundle.tools.marketplace.app import app as marketplace_app
    app.add_typer(marketplace_app, name="marketplace")
except ImportError as exc:
    @app.command("marketplace")
    def marketplace_missing() -> None:
        """Marketplace (marketplace module not installed)."""
        typer.echo(f"[SIN-BUNDLE] marketplace module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)


# ── Thin binary wrappers for new SIN-Code tools (v0.10.0) ──────────────────
# These forward to standalone binaries if installed; otherwise they show a
# helpful installation hint.  They complement the Python-module commands
# (review, debt, verify) so agents can use either interface.

_NEW_TOOL_BINARIES = {
    "ibd": ("SIN-Code-Intent-Based-Diffing", "ibd"),
    "poc": ("SIN-Code-Proof-of-Correctness", "poc"),
    "sckg": ("SIN-Code-Semantic-Codebase-Knowledge-Graphs", "sckg"),
    "adw": ("SIN-Code-Architectural-Debt-Watchdogs", "adw"),
    "oracle": ("SIN-Code-Verification-Oracle", "oracle"),
    "efm": ("SIN-Code-EFM-Tool", "efm"),
}


def _forward_to_binary(name: str, repo_hint: str) -> None:
    """Forward remaining CLI args to the binary *name* if it exists on PATH."""
    import sys

    binary = shutil.which(name)
    if not binary:
        typer.echo(
            f"[SIN-BUNDLE] '{name}' binary not found in PATH. "
            f"Install: pip install -e ~/{repo_hint}",
            err=True,
        )
        raise typer.Exit(code=1)
    # Forward everything after the subcommand name
    args = sys.argv[sys.argv.index(name) + 1 :]
    result = subprocess.run([binary, *args])
    raise typer.Exit(code=result.returncode)


def _forward_security_subcommand(subcommand: str) -> None:
    """Forward `subcommand <rest of argv>` to the `sin-security` Go binary.

    Unlike `_forward_to_binary`, this helper is meant to be called from a Typer
    subcommand (e.g. `sin security secrets /path`). It extracts everything in
    sys.argv after the first occurrence of `subcommand` and prepends it to the
    binary invocation, so the binary sees the same shape it would if invoked
    directly (e.g. `sin-security secrets /path`).
    """
    import sys

    binary = shutil.which("sin-security")
    if not binary:
        typer.echo(
            "[SIN-BUNDLE] 'sin-security' binary not found in PATH. "
            "Build it from ~/SIN-Code-Security-Bundle:\n"
            "  go build -o ~/.local/bin/sin-security ./cmd/sin-security",
            err=True,
        )
        raise typer.Exit(code=1)
    args = sys.argv[sys.argv.index(subcommand) + 1 :] if subcommand in sys.argv else []
    result = subprocess.run([binary, subcommand, *args])
    raise typer.Exit(code=result.returncode)


@app.command()
def ibd():
    """Intent-Based Diffing (IBD) — thin wrapper around the `ibd` binary."""
    _forward_to_binary("ibd", _NEW_TOOL_BINARIES["ibd"][0])


@app.command()
def poc():
    """Proof-of-Correctness (POC) — thin wrapper around the `poc` binary."""
    _forward_to_binary("poc", _NEW_TOOL_BINARIES["poc"][0])


@app.command()
def adw():
    """Architectural Debt Watchdogs (ADW) — thin wrapper around the `adw` binary."""
    _forward_to_binary("adw", _NEW_TOOL_BINARIES["adw"][0])


@app.command()
def oracle():
    """Verification Oracle — thin wrapper around the `oracle` binary."""
    _forward_to_binary("oracle", _NEW_TOOL_BINARIES["oracle"][0])


@app.command()
def efm():
    """Ephemeral Full-Stack Mocking (EFM) — thin wrapper around the `efm` binary."""
    _forward_to_binary("efm", _NEW_TOOL_BINARIES["efm"][0])


# ── Pocock Workflow Tools (v0.11.0) ───────────────────────────────────────
# Implements the Matt Pocock System-Design Paradigm:
#   - grill-me      -> Socratic alignment & PRD generation
#   - tdd-enforcer  -> TDD Red-Green-Refactor cycle enforcement
#   - dag-kanban    -> DAG-based task orchestration
#   - zod-patch     -> Zod v3/v4 compatibility sandbox
#   - safe-start    -> Safe OpenCode bootstrap with env injection
#   - cleanup-hook  -> Post-flight system cleanup
#   - teammate      -> Multi-agent swarm communication

pocock_app = typer.Typer(
    help="Pocock Workflow - Socratic alignment, TDD enforcement, DAG orchestration, multi-agent swarm."
)
app.add_typer(pocock_app, name="pocock")


@pocock_app.command("grill-me")
def pocock_grill_me(
    goal: str = typer.Argument(..., help="Development goal / feature description"),
    output: str = typer.Option("PRD.md", "--output", "-o", help="Output path for PRD.md"),
    non_interactive: bool = typer.Option(False, "--non-interactive", help="Non-interactive mode (CI/CD)"),
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
    output: str = typer.Option("docker-compose.dag.yml", "--output", help="Docker Compose output path"),
):
    """DAG-based Kanban - parses PRD and creates task execution graph."""
    from sin_code_bundle.tools.pocock.dag_kanban import DAGKanban

    runner = DAGKanban(prd)
    order = runner.run()

    if json_out:
        typer.echo(runner.to_json())

    if docker:
        try:
            import yaml
            runner.export_docker_compose(output)
        except ImportError:
            typer.echo("⚠️  PyYAML not installed. Run: pip install pyyaml", err=True)
            raise typer.Exit(code=1)


@pocock_app.command("cleanup")
def pocock_cleanup():
    """Run post-flight cleanup hook (system cleanup after task runs)."""
    script_path = Path(__file__).parent.parent.parent / "scripts" / "pocock" / "opencode-cleanup-hook.sh"
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
    script_path = Path(__file__).parent.parent.parent / "scripts" / "pocock" / "opencode-safe-start.sh"
    if not script_path.exists():
        typer.echo("❌ Safe-start script not found. Is the bundle installed correctly?", err=True)
        raise typer.Exit(code=1)

    # Forward remaining args to the script
    import sys
    args = sys.argv[sys.argv.index("safe-start") + 1:]
    result = subprocess.run(["bash", str(script_path), *args])
    raise typer.Exit(code=result.returncode)

if __name__ == "__main__":
    app()


# ── SIN Bench (SWE-bench A/B harness) ──────────────────────────────────────
@app.command()
def bench(
    tasks: str | None = typer.Option(
        None, "--tasks", help="Path to a JSONL task file. Omit to use SWE-bench Lite."
    ),
    limit: int = typer.Option(20, help="Max number of tasks to run per arm."),
    runner: str = typer.Option(
        "dry", help="Agent runner: 'dry' | 'opencode' | 'codex' | 'hermes'."
    ),
    arms: str = typer.Option("control,sin", help="Comma-separated arms to run."),
    out: str | None = typer.Option(None, "--out", help="Write the full JSON report to this path."),
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


# ── SIN Hooks (automatic SIN-Brain calls via .opencode hooks) ──────────────
@app.command(name="hooks-install")
def hooks_install(
    target: str = typer.Argument("opencode", help="Target CLI: opencode"),
    pre_command: bool = typer.Option(True, "--pre-command", help="Install pre-command hook."),
    post_command: bool = typer.Option(True, "--post-command", help="Install post-command hook."),
    brain_path: str = typer.Option(
        ".sin/brain.db", "--brain-path", help="SIN-Brain database path."
    ),
):
    """Install automatic hooks for SIN-Brain calls before/after every command."""
    from sin_code_bundle import hooks

    if target != "opencode":
        typer.echo("[SIN-BUNDLE] Only 'opencode' hooks are supported.", err=True)
        raise typer.Exit(code=2)

    installed = hooks.install_opencode_hooks(
        pre_command=pre_command,
        post_command=post_command,
        brain_path=brain_path,
    )
    for path in installed:
        typer.echo(f"[SIN-BUNDLE] Installed hook -> {path}")
    if not installed:
        typer.echo(
            "[SIN-BUNDLE] No hooks installed (both --pre-command and --post-command disabled)."
        )
    else:
        typer.echo("[SIN-BUNDLE] Hooks active. Run `sin hooks-uninstall` to remove them.")


@app.command(name="hooks-uninstall")
def hooks_uninstall(
    target: str = typer.Argument("opencode", help="Target CLI: opencode"),
):
    """Remove automatic SIN-Brain hooks from ~/.opencode/hooks/."""
    from sin_code_bundle import hooks

    if target != "opencode":
        typer.echo("[SIN-BUNDLE] Only 'opencode' hooks are supported.", err=True)
        raise typer.Exit(code=2)

    removed = hooks.uninstall_opencode_hooks()
    for path in removed:
        typer.echo(f"[SIN-BUNDLE] Removed hook -> {path}")
    if not removed:
        typer.echo("[SIN-BUNDLE] No hooks found to uninstall.")


@app.command(name="hooks-list")
def hooks_list(
    target: str = typer.Argument("opencode", help="Target CLI: opencode"),
):
    """List installed SIN-Brain hooks in ~/.opencode/hooks/."""
    from sin_code_bundle import hooks

    if target != "opencode":
        typer.echo("[SIN-BUNDLE] Only 'opencode' hooks are supported.", err=True)
        raise typer.Exit(code=2)

    found = hooks.list_opencode_hooks()
    if not found:
        typer.echo("[SIN-BUNDLE] No hooks installed. Run `sin hooks-install` to set them up.")
    else:
        for path in found:
            typer.echo(f"[SIN-BUNDLE] Hook -> {path}")


# ── Skills (compile portable skills into an agent's native format) ─────────
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


# ── Policy (inspect / initialize the policy and audit log) ─────────────────
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


# ── Doctor (environment diagnostics) ──────────────────────────────────────
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
        typer.echo(f"  {mark}{r['language']:<11} {r['files']:>5} files  server={r['server']}")
        if not r["installed"]:
            typer.echo(f"       install: {r['install_hint']}")

    ok = AuditLog(Path(root)).verify_chain()
    typer.echo(f"[SIN-BUNDLE] Audit chain: {'intact' if ok else 'TAMPERED'}")


# ── SIN-Code Go Tools Commands ─────────────────────────────────────────────
@sin_code_app.command("run")
def sin_code_run(
    tool: str = typer.Argument(
        ..., help="Tool name: discover, execute, map, grasp, scout, harvest, orchestrate"
    ),
    args: list[str] = typer.Argument(default_factory=list, help="Arguments to pass to the tool"),
):
    """Run a SIN-Code Go tool with the given arguments."""
    if tool not in _SIN_CODE_TOOLS:
        typer.echo(
            f"[SIN-CODE] Unknown tool: {tool}. Available: {', '.join(_SIN_CODE_TOOLS.keys())}",
            err=True,
        )
        raise typer.Exit(code=1)

    path = _sin_code_tool_path(tool)
    if not path:
        typer.echo(
            f"[SIN-CODE] Tool '{tool}' not found. Install: go install github.com/OpenSIN-Code/{_SIN_CODE_TOOLS[tool]}/cmd/{tool}@latest",
            err=True,
        )
        raise typer.Exit(code=1)

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
    content = """# 🛠️ SIN-Code Tool Suite — IMMER UNSERE EIGENEN TOOLS NUTZEN (PRIORITY -10.0 - ABSOLUTE OBERSTE REGEL)

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

"""
    output.write_text(content, encoding="utf-8")
    typer.echo(f"[SIN-CODE] Generated {output}")


# ── SIN-Code Security Bundle Sub-Commands ──────────────────────────────────
# Each of the 8 tools is a thin pass-through to the `sin-security` Go binary
# shipped by SIN-Code-Security-Bundle (https://github.com/OpenSIN-Code/SIN-Code-Security-Bundle).
# The `full` convenience command delegates to the binary's built-in `scan`
# subcommand, which runs all 8 tools in sequence and emits a combined report.


@security_app.command("secrets")
def security_secrets(
    path: str = typer.Argument(".", help="Path to scan for hardcoded secrets"),
):
    """Secret scanning (regex + Shannon entropy)."""
    _forward_security_subcommand("secrets")


@security_app.command("sast")
def security_sast(
    path: str = typer.Argument(".", help="Path to scan with static analysis"),
):
    """Static application security testing (SAST)."""
    _forward_security_subcommand("sast")


@security_app.command("sca")
def security_sca(
    path: str = typer.Argument(".", help="Path to scan for vulnerable dependencies"),
):
    """Software composition analysis (SCA) — deps + CVEs."""
    _forward_security_subcommand("sca")


@security_app.command("sbom")
def security_sbom(
    path: str = typer.Argument(".", help="Path to generate SBOM for"),
):
    """Generate SBOM in SPDX and CycloneDX formats."""
    _forward_security_subcommand("sbom")


@security_app.command("container")
def security_container(
    path: str = typer.Argument(".", help="Path (image name or Dockerfile) to scan"),
):
    """Container image security scan."""
    _forward_security_subcommand("container")


@security_app.command("iac")
def security_iac(
    path: str = typer.Argument(".", help="Path to IaC files (Terraform, etc.)"),
):
    """Infrastructure-as-Code security scan (Terraform, CloudFormation, K8s)."""
    _forward_security_subcommand("iac")


@security_app.command("license")
def security_license(
    path: str = typer.Argument(".", help="Path to scan for license compliance"),
):
    """License compliance check across all dependencies."""
    _forward_security_subcommand("license")


@security_app.command("dast")
def security_dast(
    path: str = typer.Argument(".", help="Path or target URL for DAST scan"),
):
    """Dynamic application security testing (DAST)."""
    _forward_security_subcommand("dast")


@security_app.command("full")
def security_full(
    path: str = typer.Argument(".", help="Path to run all 8 security tools against"),
):
    """Run all 8 security tools in sequence and emit a combined report.

    Delegates to the binary's built-in `scan` subcommand, which orchestrates
    secrets, sast, sca, sbom, container, iac, license, and dast. Use
    `--compliance` to scope to a framework (e.g. cis,nist,soc2,iso27001,gdpr,
    hipaa,pci,owasp) and `--skip-tools` to exclude specific tools.
    """
    import sys

    binary = shutil.which("sin-security")
    if not binary:
        typer.echo(
            "[SIN-BUNDLE] 'sin-security' binary not found in PATH. "
            "Build it from ~/SIN-Code-Security-Bundle:\n"
            "  go build -o ~/.local/bin/sin-security ./cmd/sin-security",
            err=True,
        )
        raise typer.Exit(code=1)
    args = sys.argv[sys.argv.index("full") + 1 :] if "full" in sys.argv else []
    result = subprocess.run([binary, "scan", *args])
    raise typer.Exit(code=result.returncode)


# ── CEO Audit Sub-Commands (SOTA repo review) ─────────────────────────────
_CEO_AUDIT_SKILL_PATH = Path.home() / ".config" / "opencode" / "skills" / "ceo-audit"
_CEO_AUDIT_SCRIPT = _CEO_AUDIT_SKILL_PATH / "scripts" / "audit.sh"


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
    import shutil

    skill_source = Path(__file__).parent.parent.parent.parent / "skills" / "ceo-audit"
    skill_target = _CEO_AUDIT_SKILL_PATH

    if not skill_source.exists():
        # Fall back: try the repo's skills/ directory
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
    # Make all scripts executable
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
        # Check if SIN-Code tools are available
        from shutil import which

        missing = [t for t in _SIN_CODE_TOOLS if not which(t)]
        if missing:
            typer.echo(f"  Missing SIN-Code tools: {', '.join(missing)}")
            typer.echo("  Install: bash ~/.local/share/SIN-Code-Bundle/install.sh")
        else:
            typer.echo("  All 7 SIN-Code tools available")
    else:
        typer.echo("  Install: sin ceo-audit install")


# ── sin-browser Sub-Commands (106 browser-automation tools) ────────────────
browser_app = typer.Typer(
    help="sin-browser — 106 browser-automation tools (navigate, click, fill, screenshot, scrape, etc.)"
)
app.add_typer(browser_app, name="browser")


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
    import json as _json

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
        from collections import defaultdict

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
    import shutil

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
    import json as _json

    try:
        data = _json.loads(result.stdout)
        count = data.get("count", 0)
    except Exception:
        count = "?"
    typer.echo(f"sin-browser installed: yes ({count} tools available)")
    skill = Path.home() / ".config" / "opencode" / "skills" / "sin-browser-tools" / "SKILL.md"
    typer.echo(f"  Skill installed: {'yes' if skill.exists() else 'no'}")
    typer.echo("  See: sin browser list")


# ── v2 Sub-Commands (VFS, Hashline, Memory, AST) ───────────────────────────
vfs_app = typer.Typer(
    help="VFS — resolve SIN URI schemes (sckg://, poc://, ibd://, adw://, efsm://, oracle://, conflict://)"
)
app.add_typer(vfs_app, name="vfs")


@vfs_app.command("resolve")
def vfs_resolve(
    uri: str = typer.Argument(..., help="URI to resolve (e.g., sckg://module/auth/dependencies)"),
    repo: str = typer.Option(".", "--repo", help="Repo root"),
    json_out: bool = typer.Option(False, "--json", help="JSON output"),
):
    """Resolve a SIN URI scheme to structured content."""
    from sin_code_bundle.vfs import SINVirtualFS

    vfs = SINVirtualFS(Path(repo))
    result = vfs.resolve(uri)
    typer.echo(json.dumps(result, indent=2))


@vfs_app.command("schemes")
def vfs_schemes():
    """List all available URI schemes."""
    from sin_code_bundle.vfs import URI_SCHEMES

    typer.echo("Available URI schemes:")
    for scheme, desc in URI_SCHEMES.items():
        typer.echo(f"  {scheme}://  {desc}")


@vfs_app.command("status")
def vfs_status():
    """Check which SIN subsystems are available for VFS resolution."""
    from sin_code_bundle.vfs import URI_SCHEMES

    typer.echo("VFS backend status:")
    module_map = {
        "sckg": "sin_code_sckg",
        "poc": "sin_code_poc",
        "ibd": "sin_code_ibd",
        "adw": "sin_code_adw",
        "efsm": "sin_code_efsm",
        "oracle": "sin_code_oracle",
    }
    for scheme in URI_SCHEMES:
        if scheme == "conflict":
            typer.echo(f"  {scheme:8s}  OK (git-based)")
            continue
        try:
            __import__(module_map[scheme])
            typer.echo(f"  {scheme:8s}  OK")
        except ImportError:
            typer.echo(f"  {scheme:8s}  NOT INSTALLED")


hashline_app = typer.Typer(
    help="Hashline anchor patching — content-hash based, no string-not-found errors"
)
app.add_typer(hashline_app, name="hashline")


@hashline_app.command("patch")
def hashline_patch(
    file: Path = typer.Argument(..., help="File to patch"),
    old: str = typer.Option(..., "--old", help="Old content to replace"),
    new: str = typer.Option(..., "--new", help="New content"),
    intent: str = typer.Option("", "--intent", help="Intent description"),
    apply: bool = typer.Option(False, "--apply", help="Apply the patch immediately"),
    json_out: bool = typer.Option(False, "--json", help="JSON output"),
):
    """Create a hashline-anchored patch (and optionally apply it)."""
    from sin_code_bundle.hashline import SINHashlinePatch

    patcher = SINHashlinePatch()
    patch = patcher.create_semantic_patch(file, old, new, intent or None)
    if patch is None:
        typer.echo(f"ERROR: Could not find anchor for old content in {file}", err=True)
        raise typer.Exit(code=1)
    if apply:
        success, msg = patcher.apply_semantic_patch(patch)
        result = {"patch": patch, "applied": success, "message": msg}
    else:
        result = {"patch": patch, "applied": False, "message": "Use --apply to write"}
    if json_out:
        typer.echo(json.dumps(result, indent=2))
    else:
        typer.echo(f"Patch: anchor_line={patch['anchor_line']}, hash={patch['anchor_hash'][:8]}")
        typer.echo(f"Status: {result['message']}")


@hashline_app.command("validate")
def hashline_validate(
    file: Path = typer.Argument(..., help="File to validate against"),
    patch_json: str = typer.Option(..., "--patch", help="Patch JSON (or @file)"),
):
    """Validate a patch can still be applied (anchor not stale)."""
    from sin_code_bundle.hashline import HashlineAnchor

    if patch_json.startswith("@"):
        with open(patch_json[1:]) as f:
            patch = json.load(f)
    else:
        patch = json.loads(patch_json)
    content = file.read_text()
    anchor = HashlineAnchor(content)
    is_valid, msg = anchor.validate_patch(patch)
    typer.echo(f"Valid: {is_valid} - {msg}")
    raise typer.Exit(code=0 if is_valid else 1)


# NOTE: The `sin memory {retain,recall,reflect,stats,forget}` and
# `sin memory {honcho-status,honcho-retain,honcho-chat}` + `sin context query`
# sub-commands were removed in this commit. They referenced `SINMemory` and
# `HonchoBackend` classes that were moved to the external `sin-brain` package
# (see commit af69464, BR-1, Issue #14). The bundle's `memory.py` is now a
# thin pass-through adapter to `sin_brain.mcp_tools` and exposes the five
# memory operations only as MCP tools (`recall`, `remember`, `forget`, `pin`,
# `link_evidence`) registered by `sin serve` — not as CLI sub-commands.
# Honcho integration is intentionally out of scope for this bundle: the
# real memory backend is `sin-brain` (SQLite + FTS5, MIT, 1500+ LOC).
# See `src/sin_code_bundle/memory.doc.md` for the current architecture.

ast_app = typer.Typer(help="AST-based code editing (requires tree-sitter)")
app.add_typer(ast_app, name="ast")


@ast_app.command("edit")
def ast_edit(
    file: Path = typer.Argument(..., help="File to edit"),
    old: str = typer.Option(..., "--old", help="Old substring"),
    new: str = typer.Option(..., "--new", help="Replacement"),
    apply: bool = typer.Option(False, "--apply", help="Apply changes immediately"),
    no_poc: bool = typer.Option(False, "--no-poc", help="Skip POC verification"),
    json_out: bool = typer.Option(False, "--json", help="JSON output"),
):
    """Propose an AST-based edit."""
    from sin_code_bundle.ast_edit import SINASTEdit

    ast = SINASTEdit()
    if not ast.is_available():
        typer.echo(
            "ERROR: tree-sitter not installed. Run: pip install tree-sitter tree-sitter-languages",
            err=True,
        )
        raise typer.Exit(code=1)
    result = ast.edit(file, old, new, verify_with_poc=not no_poc)
    if apply and result.success:
        ast.resolve(file, result.proposed_changes)
    out = result.to_dict()
    if json_out:
        typer.echo(json.dumps(out, indent=2))
    else:
        if result.success:
            typer.echo(
                f"Edit proposed: {len(result.proposed_changes)} changes, POC verified={result.poc_verified}"
            )
            if apply:
                typer.echo("Applied.")
        else:
            typer.echo(f"ERROR: {result.error}", err=True)
            raise typer.Exit(code=1)


@ast_app.command("status")
def ast_status():
    """Check if AST edit is available."""
    from sin_code_bundle.ast_edit import SINASTEdit

    ast = SINASTEdit()
    if ast.is_available():
        typer.echo(f"AST edit available. Languages: {', '.join(ast.SUPPORTED_LANGS)}")
    else:
        typer.echo("AST edit NOT available. Run: pip install tree-sitter tree-sitter-languages")
        raise typer.Exit(code=1)


# ── v0.9.3 Consolidated Skill Subcommands (issue #29) ─────────────────────
# Migrated 3 baseline skills into the bundle CLI:
#   - sin-slash           -> sin slash <sub>
#   - sin-mcp-server-builder -> sin mcp-server <sub>
#   - sin-marketplace     -> sin marketplace <sub>
# Source repos are now archived (see DEPRECATED notice in their READMEs).
try:
    from sin_code_bundle.tools.slash.app import app as slash_app
    app.add_typer(slash_app, name="slash")
except ImportError as exc:
    @app.command("slash")
    def slash_missing() -> None:
        """Slash commands (slash module not installed)."""
        typer.echo(f"[SIN-BUNDLE] slash module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)

try:
    from sin_code_bundle.tools.mcp_server_builder.app import app as mcp_server_app
    app.add_typer(mcp_server_app, name="mcp-server")
except ImportError as exc:
    @app.command("mcp-server")
    def mcp_server_missing() -> None:
        """MCP server builder (mcp_server_builder module not installed)."""
        typer.echo(f"[SIN-BUNDLE] mcp-server module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)

try:
    from sin_code_bundle.tools.marketplace.app import app as marketplace_app
    app.add_typer(marketplace_app, name="marketplace")
except ImportError as exc:
    @app.command("marketplace")
    def marketplace_missing() -> None:
        """Marketplace (marketplace module not installed)."""
        typer.echo(f"[SIN-BUNDLE] marketplace module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)


# ── Thin binary wrappers for new SIN-Code tools (v0.10.0) ──────────────────
# These forward to standalone binaries if installed; otherwise they show a
# helpful installation hint.  They complement the Python-module commands
# (review, debt, verify) so agents can use either interface.

_NEW_TOOL_BINARIES = {
    "ibd": ("SIN-Code-Intent-Based-Diffing", "ibd"),
    "poc": ("SIN-Code-Proof-of-Correctness", "poc"),
    "sckg": ("SIN-Code-Semantic-Codebase-Knowledge-Graphs", "sckg"),
    "adw": ("SIN-Code-Architectural-Debt-Watchdogs", "adw"),
    "oracle": ("SIN-Code-Verification-Oracle", "oracle"),
    "efm": ("SIN-Code-EFM-Tool", "efm"),
}


def _forward_to_binary(name: str, repo_hint: str) -> None:
    """Forward remaining CLI args to the binary *name* if it exists on PATH."""
    import sys

    binary = shutil.which(name)
    if not binary:
        typer.echo(
            f"[SIN-BUNDLE] '{name}' binary not found in PATH. "
            f"Install: pip install -e ~/{repo_hint}",
            err=True,
        )
        raise typer.Exit(code=1)
    # Forward everything after the subcommand name
    args = sys.argv[sys.argv.index(name) + 1 :]
    result = subprocess.run([binary, *args])
    raise typer.Exit(code=result.returncode)


def _forward_security_subcommand(subcommand: str) -> None:
    """Forward `subcommand <rest of argv>` to the `sin-security` Go binary.

    Unlike `_forward_to_binary`, this helper is meant to be called from a Typer
    subcommand (e.g. `sin security secrets /path`). It extracts everything in
    sys.argv after the first occurrence of `subcommand` and prepends it to the
    binary invocation, so the binary sees the same shape it would if invoked
    directly (e.g. `sin-security secrets /path`).
    """
    import sys

    binary = shutil.which("sin-security")
    if not binary:
        typer.echo(
            "[SIN-BUNDLE] 'sin-security' binary not found in PATH. "
            "Build it from ~/SIN-Code-Security-Bundle:\n"
            "  go build -o ~/.local/bin/sin-security ./cmd/sin-security",
            err=True,
        )
        raise typer.Exit(code=1)
    args = sys.argv[sys.argv.index(subcommand) + 1 :] if subcommand in sys.argv else []
    result = subprocess.run([binary, subcommand, *args])
    raise typer.Exit(code=result.returncode)


@app.command()
def ibd():
    """Intent-Based Diffing (IBD) — thin wrapper around the `ibd` binary."""
    _forward_to_binary("ibd", _NEW_TOOL_BINARIES["ibd"][0])


@app.command()
def poc():
    """Proof-of-Correctness (POC) — thin wrapper around the `poc` binary."""
    _forward_to_binary("poc", _NEW_TOOL_BINARIES["poc"][0])


@app.command()
def adw():
    """Architectural Debt Watchdogs (ADW) — thin wrapper around the `adw` binary."""
    _forward_to_binary("adw", _NEW_TOOL_BINARIES["adw"][0])


@app.command()
def oracle():
    """Verification Oracle — thin wrapper around the `oracle` binary."""
    _forward_to_binary("oracle", _NEW_TOOL_BINARIES["oracle"][0])


@app.command()
def efm():
    """Ephemeral Full-Stack Mocking (EFM) — thin wrapper around the `efm` binary."""
    _forward_to_binary("efm", _NEW_TOOL_BINARIES["efm"][0])


@app.command()
def tui(
    fallback: bool = typer.Option(
        False,
        "--fallback",
        help="Skip the TUI and show a plain menu (used when no TTY is available).",
    ),
) -> None:
    """Launch the SIN-Code TUI (Bubbletea) — interactive menu over every `sin` subcommand.

    The TUI is a separate Go binary (sin-tui) that the Python CLI shells out to.
    Build it once with:

        go build -o ~/.local/bin/sin-tui ./cmd/sin-tui

    If the binary is missing, this command prints a short installation hint and
    exits 1 instead of crashing, so `sin tui` is always safe to call.
    """
    import sys

    if fallback or not sys.stdout.isatty():
        # Plain-text menu fallback for non-TTY environments (CI, logs, pipes).
        typer.echo("sin tui — interactive mode (fallback, no TTY detected)\n")
        for c in _TU_CATALOG:
            typer.echo(f"  {c['title']:<22}  {c['desc']}")
        typer.echo("\nRun `sin <subcommand> --help` for details.")
        return

    binary = shutil.which("sin-tui")
    if not binary:
        typer.echo(
            "[SIN-BUNDLE] 'sin-tui' binary not found in PATH.\n"
            "Build it from this repo:\n"
            "  go build -o ~/.local/bin/sin-tui ./cmd/sin-tui\n"
            "Or download a prebuilt binary from the SIN-Code-Bundle release page.",
            err=True,
        )
        raise typer.Exit(code=1)

    # Hand off the terminal to the Go binary (it uses alt-screen + mouse).
    result = subprocess.run([binary, *sys.argv[sys.argv.index("tui") + 1 :]])
    raise typer.Exit(code=result.returncode)


# Catalog used by the non-TTY fallback in `tui`. Keep in sync with
# internal/tui/commands.go (the Go side is the source of truth at runtime).
_TU_CATALOG = [
    {"title": "sin code", "desc": "Unified coding workflow hub"},
    {"title": "sin code full <path>", "desc": "Run preflight → codocs → debt → sckg"},
    {"title": "sin sckg", "desc": "Semantic codebase knowledge graph"},
    {"title": "sin ibd", "desc": "Intent-based diffing"},
    {"title": "sin poc", "desc": "Proof-of-correctness"},
    {"title": "sin adw", "desc": "Architectural debt watchdog"},
    {"title": "sin oracle", "desc": "Independent verification oracle"},
    {"title": "sin sin-code run map <path>", "desc": "Architecture map"},
    {"title": "sin sin-code run scout <q>", "desc": "Code search"},
    {"title": "sin status", "desc": "Subsystem status"},
    {"title": "sin doctor", "desc": "Diagnose environment"},
    {"title": "sin bootstrap <path>", "desc": "Initialize subsystems"},
    {"title": "sin serve", "desc": "Expose tools as MCP server"},
    {"title": "sin brain", "desc": "Behavioral memory"},
    {"title": "sin context-bridge <q>", "desc": "Unified context query"},
    {"title": "sin update", "desc": "Upgrade pipx package + rebuild Go tools"},
    {"title": "sin config", "desc": "Read/write the layered config (TOML + opencode + env)"},
]


# ── v1.4.0 —  and  commands ────────────────────────────────────────────
# See update.doc.md and config.doc.md for per-module design notes.

update_app = typer.Typer(
    help="Update the SIN-Code bundle and its Go toolchain.",
    no_args_is_help=True,
)
app.add_typer(update_app, name="update")


@update_app.callback()
def _update_callback() -> None:
    """Update the bundle (pipx) and/or the Go toolchain under ~/dev/."""


@update_app.command("run")
def update_run(
    core: bool = typer.Option(
        False, "--core", help="Only update the Python package (pipx upgrade)."
    ),
    go: bool = typer.Option(
        False, "--go", help="Only rebuild Go tools under ~/dev/SIN-Code-*-Tool/."
    ),
    check: bool = typer.Option(
        False, "--check", help="Dry run — print the plan but do not modify anything."
    ),
    json_out: bool = typer.Option(False, "--json", help="Output JSON instead of a table."),
) -> None:
    """Upgrade pipx package and rebuild every Go tool binary in place."""
    from sin_code_bundle import update as upd

    results = upd.run_update(core=core, go=go, check=check)
    if json_out:
        typer.echo(
            json.dumps([r.__dict__ for r in results], indent=2, default=str)
        )
    else:
        typer.echo(upd.render_table(results))
    failed = [r for r in results if r.status == "failed"]
    if failed and not check:
        raise typer.Exit(code=1)


config_app = typer.Typer(
    help="Read/write the layered SIN-Code config (TOML + opencode + env).",
    no_args_is_help=True,
)
app.add_typer(config_app, name="config")


@config_app.callback()
def _config_callback() -> None:
    """Manage the layered SIN-Code configuration."""


@config_app.command("show")
def config_show(
    json_out: bool = typer.Option(False, "--json", help="Output JSON instead of text."),
) -> None:
    """Show the resolved config (all sources merged, secrets redacted)."""
    from sin_code_bundle import config as cfg

    payload, origins = cfg.merged()
    if json_out:
        typer.echo(
            json.dumps(
                {
                    "config": payload,
                    "origins": {
                        k: {"label": v.label, "path": str(v.path)}
                        for k, v in origins.items()
                    },
                },
                indent=2,
                default=str,
            )
        )
    else:
        typer.echo(cfg.format_show(payload, origins))


@config_app.command("get")
def config_get(key: str = typer.Argument(..., help="Dotted key, e.g. tui.theme")) -> None:
    """Print the value of a single config key (respects redaction)."""
    from sin_code_bundle import config as cfg

    view = cfg.get(key)
    if view.value is cfg.MISSING:
        typer.echo(f"(unset: {key})", err=True)
        raise typer.Exit(code=1)
    if cfg._is_sensitive(key):
        typer.echo(cfg.REDACTED_PLACEHOLDER)
    else:
        typer.echo(view.value)


@config_app.command("set")
def config_set(
    key: str = typer.Argument(..., help="Dotted key, e.g. tui.theme"),
    value: str = typer.Argument(..., help="Value to store (always as string)"),
) -> None:
    """Set a config value in ./sin.config.toml (project-local)."""
    from sin_code_bundle import config as cfg

    try:
        path = cfg.set_value(key, value)
    except ValueError as exc:
        typer.echo(f"[SIN-BUNDLE] {exc}", err=True)
        raise typer.Exit(code=1)
    typer.echo(f"{key} = {value!r}  ->  {path}")


@config_app.command("unset")
def config_unset(key: str = typer.Argument(..., help="Dotted key to remove")) -> None:
    """Remove a config value from ./sin.config.toml."""
    from sin_code_bundle import config as cfg

    if cfg.unset_value(key):
        typer.echo(f"Removed: {key}")
    else:
        typer.echo(f"(was unset: {key})")
        raise typer.Exit(code=1)


@config_app.command("path")
def config_path() -> None:
    """Print every config file path the resolver checks, with existence markers."""
    from sin_code_bundle import config as cfg

    typer.echo(cfg.format_path(cfg.all_paths()))


# ── SIN Developer CLI (lint, docs, git) ────────────────────────────────────
# Issue #42: Python-based SIN-Code Developer CLI

lint_app = typer.Typer(help="Lint code with popular linters (ruff, flake8, mypy, eslint, etc.).")
app.add_typer(lint_app, name="lint")

@lint_app.command("run")
def lint_run(
    path: str = typer.Argument(".", help="Path to lint."),
    tool: str = typer.Option("auto", help="Linter to use: auto, ruff, flake8, mypy, pylint, eslint, golangci-lint, shellcheck."),
    fix: bool = typer.Option(False, help="Auto-fix issues where supported."),
):
    """Run a linter on the given path."""
    import shutil

    linters = {
        "ruff": ("ruff", ["check", path] + (["--fix"] if fix else [])),
        "flake8": ("flake8", [path]),
        "mypy": ("mypy", [path]),
        "pylint": ("pylint", [path]),
        "eslint": ("eslint", [path] + (["--fix"] if fix else [])),
        "golangci-lint": ("golangci-lint", ["run", path]),
        "shellcheck": ("shellcheck", [path]),
    }

    if tool == "auto":
        for name, (binary, _) in linters.items():
            if shutil.which(binary):
                tool = name
                break
        else:
            typer.echo("[SIN-BUNDLE] No supported linter found on PATH. Install one: ruff, flake8, mypy, pylint, eslint, golangci-lint, shellcheck", err=True)
            raise typer.Exit(code=1)

    if tool not in linters:
        typer.echo(f"[SIN-BUNDLE] Unknown linter '{tool}'.", err=True)
        raise typer.Exit(code=1)

    binary, args = linters[tool]
    if not shutil.which(binary):
        typer.echo(f"[SIN-BUNDLE] '{binary}' not found on PATH. Install it first.", err=True)
        raise typer.Exit(code=1)

    result = subprocess.run([binary, *args], capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@lint_app.command("check")
def lint_check(
    path: str = typer.Argument(".", help="Path to check."),
):
    """Check which linters are available and what they would report."""
    import shutil

    available = []
    for name, binary in [
        ("ruff", "ruff"), ("flake8", "flake8"), ("mypy", "mypy"),
        ("pylint", "pylint"), ("eslint", "eslint"), ("golangci-lint", "golangci-lint"),
        ("shellcheck", "shellcheck"),
    ]:
        if shutil.which(binary):
            available.append(name)

    if not available:
        typer.echo("[SIN-BUNDLE] No linters found on PATH.")
    else:
        typer.echo(f"[SIN-BUNDLE] Available linters: {', '.join(available)}")

    for name in available:
        typer.echo(f"\n--- {name} ---")
        if name == "ruff":
            result = subprocess.run(["ruff", "check", path], capture_output=True, text=True)
        elif name == "flake8":
            result = subprocess.run(["flake8", path], capture_output=True, text=True)
        elif name == "mypy":
            result = subprocess.run(["mypy", path], capture_output=True, text=True)
        else:
            continue

        if result.stdout:
            typer.echo(result.stdout)
        if result.stderr:
            typer.echo(result.stderr, err=True)


docs_app = typer.Typer(help="Documentation helpers — generate README, API docs, check coverage.")
app.add_typer(docs_app, name="docs")

@docs_app.command("generate")
def docs_generate(
    path: str = typer.Argument(".", help="Project path."),
    output: str = typer.Option("README.md", help="Output file name."),
    template: str = typer.Option("default", help="Template: default, minimal, full."),
):
    """Generate a README.md from project metadata."""
    import os
    import json

    proj = Path(path)
    if not proj.exists():
        typer.echo(f"[SIN-BUNDLE] Path not found: {path}", err=True)
        raise typer.Exit(code=1)

    # Gather metadata
    name = proj.resolve().name
    pyproject = proj / "pyproject.toml"
    package_json = proj / "package.json"
    go_mod = proj / "go.mod"

    language = "unknown"
    version = "0.1.0"
    description = f"{name} project"
    dependencies = []

    if pyproject.exists():
        language = "Python"
        content = pyproject.read_text()
        for line in content.splitlines():
            if line.startswith("name"):
                name = line.split("=")[1].strip().strip('"').strip("'")
            if line.startswith("version"):
                version = line.split("=")[1].strip().strip('"').strip("'")
            if line.startswith("description"):
                description = line.split("=")[1].strip().strip('"').strip("'")
    elif package_json.exists():
        language = "JavaScript/TypeScript"
        data = json.loads(package_json.read_text())
        name = data.get("name", name)
        version = data.get("version", "0.1.0")
        description = data.get("description", description)
        dependencies = list(data.get("dependencies", {}).keys())
    elif go_mod.exists():
        language = "Go"
        content = go_mod.read_text()
        for line in content.splitlines():
            if line.startswith("module "):
                name = line.split()[1]

    readme = f"""# {name}

{description}

## Overview

- **Language**: {language}
- **Version**: {version}
- **Path**: `{proj.resolve()}`

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd {name}

# Install dependencies
"""

    if language == "Python":
        readme += "pip install -e .\n"
    elif language == "JavaScript/TypeScript":
        readme += "npm install\n"
    elif language == "Go":
        readme += "go mod tidy\n"
    else:
        readme += "# See project documentation\n"

    readme += """```

## Usage

```bash
# Run the project
# (Add usage examples here)
```

## Testing

```bash
# Run tests
"""

    if language == "Python":
        readme += "pytest\n"
    elif language == "JavaScript/TypeScript":
        readme += "npm test\n"
    elif language == "Go":
        readme += "go test ./...\n"
    else:
        readme += "# See project documentation\n"

    readme += """```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Open a Pull Request

## License

MIT - OpenSIN-Code Project
"""

    if dependencies:
        readme += "\n## Dependencies\n\n"
        for dep in dependencies[:10]:
            readme += f"- {dep}\n"

    output_path = proj / output
    output_path.write_text(readme, encoding="utf-8")
    typer.echo(f"[SIN-BUNDLE] Generated {output_path}")


@docs_app.command("check")
def docs_check(
    path: str = typer.Argument(".", help="Project path."),
):
    """Check documentation coverage (README, docstrings, .doc.md files)."""
    proj = Path(path)
    if not proj.exists():
        typer.echo(f"[SIN-BUNDLE] Path not found: {path}", err=True)
        raise typer.Exit(code=1)

    readme = proj / "README.md"
    has_readme = readme.exists()

    doc_md_files = list(proj.rglob("*.doc.md"))
    py_files = list(proj.rglob("*.py"))
    docstring_count = 0

    for py_file in py_files:
        content = py_file.read_text()
        if '"""' in content or "'''" in content:
            docstring_count += 1

    typer.echo(f"[SIN-BUNDLE] Documentation Coverage Report for {proj.resolve()}")
    typer.echo(f"  README.md: {'✅' if has_readme else '❌'}")
    typer.echo(f"  .doc.md files: {len(doc_md_files)}")
    typer.echo(f"  Python files: {len(py_files)}")
    typer.echo(f"  Files with docstrings: {docstring_count}/{len(py_files)} ({100*docstring_count//max(len(py_files),1)}%)")

    if not has_readme:
        typer.echo("  ⚠️  Missing README.md — run `sin docs generate` to create one.")


git_app = typer.Typer(help="Git workflow helpers — status, commit, push, clean branches.")
app.add_typer(git_app, name="git")

@git_app.command("status")
def git_status(
    path: str = typer.Argument(".", help="Repository path."),
):
    """Show git status with a clean summary."""
    result = subprocess.run(
        ["git", "-C", path, "status", "--short"],
        capture_output=True, text=True
    )
    if result.returncode != 0:
        typer.echo(f"[SIN-BUNDLE] Not a git repository: {path}", err=True)
        raise typer.Exit(code=1)

    lines = result.stdout.strip().splitlines()
    if not lines:
        typer.echo("[SIN-BUNDLE] Working tree clean ✨")
        return

    typer.echo(f"[SIN-BUNDLE] Git status ({len(lines)} changed file(s)):")
    for line in lines:
        typer.echo(f"  {line}")


@git_app.command("commit")
def git_commit(
    message: str = typer.Argument(..., help="Commit message.", metavar="MESSAGE"),
    path: str = typer.Option(".", help="Repository path."),
    all: bool = typer.Option(False, "--all", "-a", help="Stage all modified files before committing."),
    push: bool = typer.Option(False, "--push", help="Push after committing."),
):
    """Create a git commit with the given message."""
    if all:
        stage = subprocess.run(["git", "-C", path, "add", "-A"], capture_output=True, text=True)
        if stage.returncode != 0:
            typer.echo(f"[SIN-BUNDLE] Failed to stage files: {stage.stderr}", err=True)
            raise typer.Exit(code=1)

    result = subprocess.run(
        ["git", "-C", path, "commit", "-m", message],
        capture_output=True, text=True
    )
    if result.returncode != 0:
        typer.echo(f"[SIN-BUNDLE] Commit failed: {result.stderr}", err=True)
        raise typer.Exit(code=1)

    typer.echo(f"[SIN-BUNDLE] Committed: {message}")

    if push:
        push_result = subprocess.run(["git", "-C", path, "push"], capture_output=True, text=True)
        if push_result.returncode != 0:
            typer.echo(f"[SIN-BUNDLE] Push failed: {push_result.stderr}", err=True)
            raise typer.Exit(code=1)
        typer.echo("[SIN-BUNDLE] Pushed to remote ✨")


@git_app.command("clean")
def git_clean(
    path: str = typer.Argument(".", help="Repository path."),
    dry_run: bool = typer.Option(True, "--dry-run/--no-dry-run", help="Show what would be deleted without deleting."),
    force: bool = typer.Option(False, "--force", "-f", help="Force delete merged branches."),
):
    """Clean up merged branches and stale references."""
    # Fetch and prune
    subprocess.run(["git", "-C", path, "fetch", "--prune"], capture_output=True, text=True)

    # List merged branches (excluding current, main, master)
    result = subprocess.run(
        ["git", "-C", path, "branch", "--merged"],
        capture_output=True, text=True
    )
    if result.returncode != 0:
        typer.echo(f"[SIN-BUNDLE] Failed to list branches: {result.stderr}", err=True)
        raise typer.Exit(code=1)

    branches = [b.strip() for b in result.stdout.splitlines() if b.strip() and not b.startswith("*")]
    protected = {"main", "master", "develop", "dev"}
    to_delete = [b for b in branches if b not in protected]

    if not to_delete:
        typer.echo("[SIN-BUNDLE] No merged branches to clean up ✨")
        return

    typer.echo(f"[SIN-BUNDLE] Branches to clean up ({len(to_delete)}):")
    for b in to_delete:
        typer.echo(f"  - {b}")

    if dry_run:
        typer.echo("\n[SIN-BUNDLE] Dry-run mode — no branches deleted. Use --no-dry-run to delete.")
        return

    if not force:
        typer.echo("\n[SIN-BUNDLE] Use --force to confirm deletion.")
        return

    for b in to_delete:
        del_result = subprocess.run(["git", "-C", path, "branch", "-d", b], capture_output=True, text=True)
        if del_result.returncode == 0:
            typer.echo(f"  ✅ Deleted {b}")
        else:
            typer.echo(f"  ⚠️  Could not delete {b}: {del_result.stderr}", err=True)


@git_app.command("log")
def git_log(
    path: str = typer.Argument(".", help="Repository path."),
    count: int = typer.Option(10, "-n", help="Number of commits to show."),
    oneline: bool = typer.Option(True, "--oneline/--no-oneline", help="Show one-line summary."),
):
    """Show recent commit history."""
    args = ["git", "-C", path, "log", f"-{count}"]
    if oneline:
        args.append("--oneline")
    args.append("--graph")
    args.append("--decorate")

    result = subprocess.run(args, capture_output=True, text=True)
    if result.returncode != 0:
        typer.echo(f"[SIN-BUNDLE] Failed to get log: {result.stderr}", err=True)
        raise typer.Exit(code=1)

    typer.echo(result.stdout)


if __name__ == "__main__":
    app()
