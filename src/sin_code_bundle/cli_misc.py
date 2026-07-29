# SPDX-License-Identifier: MIT
"""Top-level @app.command() functions — extracted from cli.py.

These are the remaining top-level commands that are NOT part of any sub-app.
"""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

import typer

from sin_code_bundle.cli_app import app
from sin_code_bundle.cli_forward import (  # noqa: F401
    _EXCLUDE,
    _build_cli_runner,
    _normalize_root_flag,
    _require,
)


# ── Core Status / Bootstrap Commands ────────────────────────────────────────
@app.command()
def status():
    """Zeigt, welche Subsysteme installiert sind."""
    import importlib.util
    import shutil

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

    # SIN-Code-Forge-Tool (issue #37): Go binary + pip-installed MCP server.
    report["Forge (code generation, external)"] = shutil.which("forge") is not None
    report["sin-forge (MCP server, external)"] = shutil.which("sin-forge") is not None

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
    """Review code semantically and plain text with a deterministic fallback."""
    from sin_code_bundle.review import review_files

    try:
        result = review_files(file_a, file_b)
    except ImportError:
        _require("sin_code_ibd", "pip install -e ../SIN-Code-Intent-Based-Diffing")
        raise
    typer.echo(json.dumps(result, indent=2))


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
def code(
    action: str = typer.Argument(
        ...,
        help="Action: review, debt, verify, preflight, codocs, sckg, audit, oracle, adw, ibd, discover, scout, or full",
    ),
    args: list[str] = typer.Argument(
        default_factory=list, help="Arguments to pass to the underlying command"
    ),
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


@app.command(name="mcp-config")
def mcp_config(
    client: str = typer.Argument(..., help="Target CLI: opencode | codex | hermes"),
    full: bool = typer.Option(False, "--full", help="Generate config for all 16 individual tools"),
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
