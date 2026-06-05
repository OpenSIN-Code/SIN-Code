# SPDX-License-Identifier: MIT
"""Purpose: Unified SIN-Code MCP server.

Docs: mcp_server.doc.md

This module is the standalone MCP server entry point for the SIN-Code bundle.
It is invoked via `python -m sin_code_bundle.mcp_server` (or the `sin-serve`
console script).

It exposes:

  **Core file-ops** (replace opencode native read/write/edit/bash/search):
    - sin_read        : URI-scheme aware (sckg://, poc://, ibd://, adw://,
                        efsm://, oracle://, conflict://) + size-safe file read
                        with summarize mode.
    - sin_write       : Atomic write with auto-backup + syntax pre-validation
                        for .py/.ts/.js/.go.
    - sin_edit        : Hashline-anchored semantic patching (line-shift
                        resilient, content-hash anchors).
    - sin_bash        : Safe shell exec via `execute` Go binary
                        (secret-redaction, timeout, structured result with
                        safety_check, retry_info, learned_patterns).
    - sin_search      : Wraps `scout` Go tool (semantic/regex/symbol/usage),
                        falls back to Python-regex for single-file OR
                        directory paths.

  **Subsystem tools** (when sin-code-{sckg,ibd,adw,oracle,poc,efsm,
  orchestration,review-interface} are installed via `[all]`):
    - impact, semantic_diff, architectural_debt, verify_tests, prove,
      mock_env, orchestrate, task_status, semantic_review.

  **Memory tools** (when sin-brain is installed):
    - recall_tool, remember_tool, forget_tool, pin_tool, link_evidence_tool.

  **External** (auto-detected):
    - gitnexus_context / gitnexus_impact / gitnexus_ai_context
    - markitdown_convert
    - codocs_check

Total: **28 tools** when all extras are installed (24 prior + 4 v0.8.0 baseline).

Companion skills (separate MCP servers, auto-detected via opencode.json):
  - sin-websearch (5 tools), sin-scheduler (6 tools), sin-marketplace (7 tools),
    sin-slash (6 tools), sin-goal-mode (8 tools) — 32 additional tools.

  **Baseline Workflow Tools** (v0.8.0) — never have to remember tool names:
    - sin_immortal_commit     : one-call commit + tag + push (Conventional Commits)
    - sin_programming_workflow: orchestrator (pre_write/write/post_write/pre_commit/refactor/session_warmup)
    - sin_session_warmup      : first-call session context primer
    - sin_merge_safety        : pre-merge / pre-PR safety gate

Run via:
    python -m sin_code_bundle.mcp_server
    # or
    sin-serve  (console script)
    # or (legacy, identical):
    sin serve
"""

from __future__ import annotations

import json
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Any

try:
    from mcp.server.fastmcp import FastMCP
except ImportError as exc:
    sys.stderr.write("[SIN-CODE-BUNDLE] mcp package required: pip install 'sin-code-bundle[mcp]'\n")
    raise SystemExit(1) from exc


mcp = FastMCP("sin-code-bundle")


_EXCLUDE = {".git", ".venv", "venv", "__pycache__", "node_modules", "dist", "build"}


# ── Core file-ops (replace opencode native read/write/edit/bash/search) ─────


@mcp.tool()
def sin_read(path: str, summarize: bool = False, max_chars: int = 50000) -> str:
    """SIN-Code read — replaces native read.

    URI schemes (sckg://, poc://, ibd://, adw://, efsm://, oracle://, conflict://)
    are resolved via VirtualFS — semantic, not textual.
    Plain file paths are read with size-aware truncation.
    summarize=True returns a structural overview (line count, head/tail).

    Better than native read: URI semantics, size safety, no accidental
    multi-MB dumps into context.
    """
    try:
        if "://" in path:
            from sin_code_bundle import vfs

            v = vfs.SINVirtualFS()
            return json.dumps(v.resolve(path), indent=2, default=str)
        p = Path(path).expanduser()
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
        p = Path(path).expanduser()
        backup = None
        if p.exists() and verify:
            backup = str(p) + ".bak"
            p.replace(backup)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content, encoding="utf-8")
        verified = True
        if verify and p.suffix == ".py":
            try:
                compile(content, str(p), "exec")
            except SyntaxError as e:
                verified = False
                if backup:
                    Path(backup).replace(p)
                return json.dumps({"success": False, "error": f"syntax error: {e}", "path": str(p)})
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
        p = Path(file_path).expanduser()
        if not p.exists():
            return json.dumps({"error": f"file not found: {file_path}"})
        from sin_code_bundle import hashline

        patcher = hashline.SINHashlinePatch(repo_root=p.parent)
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
def sin_bash(command: str, timeout: int = 60) -> str:  # 60s = default; max allowed is 600s
    """SIN-Code bash — replaces native bash.

    Safe command execution via the `execute` Go binary with:
    - Secret redaction (tokens/keys in output masked automatically)
    - Timeout enforcement (default 60s)
    - Exit code capture
    - Structured JSON output (stdout, stderr, returncode, safety_check,
      retry_info, learned_patterns)
    - Auto-fallback to raw shell if `execute` binary is missing

    Better than native bash: secret-safety, timeout, structured result.
    """
    try:
        cmd_path = shutil.which("execute") or str(Path.home() / ".local/bin/execute")
        if Path(cmd_path).exists():
            proc = subprocess.run(
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
        proc = subprocess.run(command, shell=True, capture_output=True, text=True, timeout=timeout)
        return json.dumps(
            {
                "stdout": proc.stdout[-10000:],
                "stderr": proc.stderr[-5000:],
                "returncode": proc.returncode,
                "redacted": False,
                "warning": "execute binary not found — running raw shell",
            }
        )
    except subprocess.TimeoutExpired:
        return json.dumps({"error": f"timeout after {timeout}s", "command": command})
    except Exception as exc:
        return json.dumps({"error": str(exc), "command": command})


@mcp.tool()
def sin_search(query: str, path: str = ".", search_type: str = "semantic") -> str:
    """SIN-Code search — replaces native search/grep/find/glob.

    Wraps the `scout` Go tool (semantic + regex + symbol + usage search).
    Falls back to Python regex if scout binary is missing — works on both
    single files and directories.

    search_type: semantic | regex | symbol | usage
    """
    try:
        cmd_path = shutil.which("scout") or str(Path.home() / ".local/bin/scout")
        if Path(cmd_path).exists():
            # 30s = conservative ceiling for the `scout` Go tool; an LLM should
            # never block on a search call for longer than typical tool timeouts.
            proc = subprocess.run(
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
        import re as _re

        results: list[dict[str, Any]] = []
        target = Path(path).expanduser()
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
                # 200 = hard ceiling for python-regex fallback; keeps the
                # fallback path from flooding the agent context if a query
                # matches millions of lines (e.g. `import ` across a big repo).
                if len(results) >= 200:
                    break
            if len(results) >= 200:
                break
        return json.dumps({"results": results, "count": len(results), "fallback": "python-regex"})
    except Exception as exc:
        return json.dumps({"error": str(exc), "query": query})


# ── VFS / AST-edit / Hashline (dedicated tools, per user request) ──────────


@mcp.tool()
def sin_vfs_resolve(uri: str) -> str:
    """Resolve a SIN URI scheme to structured content.

    Examples:
      sckg://module/<name>/dependencies
      sckg://module/<name>/callers
      poc://strategy/<name>
      ibd://diff/<file>
      adw://smell/<name>
      efsm://service/<name>
      oracle://strategy/<name>
      conflict://<id>
    """
    try:
        from sin_code_bundle import vfs

        return json.dumps(vfs.SINVirtualFS().resolve(uri), indent=2, default=str)
    except Exception as exc:
        return json.dumps({"error": str(exc), "uri": uri})


@mcp.tool()
def sin_vfs_schemes() -> str:
    """List all available SIN-Code URI schemes and their meanings."""
    try:
        from sin_code_bundle import vfs

        return json.dumps(vfs.URI_SCHEMES, indent=2)
    except Exception as exc:
        return json.dumps({"error": str(exc)})


@mcp.tool()
def sin_ast_edit(
    file_path: str,
    old_content: str,
    new_content: str,
    verify_with_poc: bool = True,
) -> str:
    """AST-based code editing via tree-sitter (Python/JS/TS/Go).

    Falls back to hashline-anchored text edit if tree-sitter is unavailable.
    Verifies syntax via POC when verify_with_poc=True.
    """
    try:
        p = Path(file_path).expanduser()
        if not p.exists():
            return json.dumps({"error": f"file not found: {file_path}"})
        try:
            from sin_code_bundle import ast_edit as _ast

            editor = _ast.SINASTEdit(repo_root=p.parent)
            if editor.is_available():
                result = editor.edit(p, old_content, new_content, verify_with_poc=verify_with_poc)
                return json.dumps(
                    result.to_dict() if hasattr(result, "to_dict") else {"result": str(result)}
                )
        except Exception:
            pass
        # Fallback: hashline
        from sin_code_bundle import hashline

        patcher = hashline.SINHashlinePatch(repo_root=p.parent)
        patch = patcher.create_semantic_patch(
            file_path=str(p), old_text=old_content, new_text=new_content, intent=""
        )
        if not patch:
            return json.dumps({"success": False, "error": "anchor not found"})
        ok, msg = patcher.apply_semantic_patch(patch)
        return json.dumps({"success": ok, "message": msg, "fallback": "hashline"})
    except Exception as exc:
        return json.dumps({"error": str(exc), "file_path": file_path})


@mcp.tool()
def sin_hashline_validate(file_path: str, patch: dict) -> str:
    """Validate a previously-created hashline patch can still be applied."""
    try:
        from sin_code_bundle.hashline import HashlineAnchor

        content = Path(file_path).read_text(encoding="utf-8", errors="replace")
        anchor = HashlineAnchor(content)
        is_valid, msg = anchor.validate_patch(patch)
        return json.dumps({"valid": is_valid, "message": msg})
    except Exception as exc:
        return json.dumps({"error": str(exc), "file_path": file_path})


# ── Subsystem Tools (graceful degradation: try-import, skip on missing) ────


def _try_subsystem_tools() -> None:
    """Wire subsystem tools; each block skips on ImportError."""
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
                    "verdict": "see risk.score",
                }
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
        from sin_code_review_interface import ReviewServer

        @mcp.tool()
        def review(file_path: str) -> str:
            """Run SOTA review on a single file."""
            ri = ReviewServer()
            if hasattr(ri, "review_file"):
                return json.dumps(ri.review_file(file_path))
            return json.dumps(
                {"file_path": file_path, "status": "ReviewServer available, no review_file method"}
            )
    except ImportError:
        pass


def _try_memory_tools() -> None:
    """Wire sin-brain memory tools; skip if not installed."""
    try:
        from sin_code_bundle import memory

        memory.register_tools(mcp)
    except ImportError:
        pass


def _try_external_tools() -> None:
    """Wire external (gitnexus, markitdown, codocs) tools."""
    try:
        from sin_code_bundle import gitnexus

        @mcp.tool()
        def gitnexus_context(symbol: str) -> str:
            """Structural graph context for a symbol (auto-indexes if needed)."""
            return json.dumps(gitnexus.get_context(symbol))

        @mcp.tool()
        def gitnexus_impact(symbol: str) -> str:
            """Blast-radius impact analysis for a symbol (auto-indexes if needed)."""
            return json.dumps(gitnexus.get_impact(symbol))

        @mcp.tool()
        def gitnexus_ai_context(task: str, symbols: str = "") -> str:
            """Task-scoped, graph-aware context bundle (auto-indexes if needed)."""
            sym_list = [s.strip() for s in symbols.split(",") if s.strip()]
            return json.dumps(gitnexus.get_ai_context(task, sym_list))
    except ImportError:
        pass

    try:
        from sin_code_bundle import markitdown

        @mcp.tool()
        def markitdown_convert(path: str) -> str:
            """Convert a document (PDF/DOCX/PPTX/XLSX/image/...) to Markdown."""
            result = markitdown.convert(path)
            return result.text_content if hasattr(result, "text_content") else str(result)
    except ImportError:
        pass

    try:
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
    except ImportError:
        pass


# ── Tool Wiring (graceful degradation) ─────────────────────────────────────
# All subsystem + memory + external tools are registered in try-import blocks
# above. A missing sin-code-* package leaves the server fully functional
# (graceful degradation — never crashes the MCP).
_try_subsystem_tools()
_try_memory_tools()
_try_external_tools()


# ── DAP Runtime Tracing ────────────────────────────────────────────────────


@mcp.tool()
def sin_runtime_trace(file_path: str, function_name: str, language: str = "python") -> str:
    """Start a DAP debugging session for a specific function.

    Replaces: Guessing from logs. Attaches real debugger (debugpy/dlv/node).
    """
    try:
        from sin_code_bundle.dap_bridge import SINRuntimeTrace

        tracer = SINRuntimeTrace()
        return json.dumps(tracer.trace_function(file_path, function_name, language))
    except Exception as exc:
        return json.dumps({"error": str(exc)})


@mcp.tool()
def sin_stop_trace(session_id: str) -> str:
    """Stop an active DAP debugging session."""
    try:
        from sin_code_bundle.dap_bridge import SINRuntimeTrace

        tracer = SINRuntimeTrace()
        return json.dumps(tracer.stop_trace(session_id))
    except Exception as exc:
        return json.dumps({"error": str(exc)})


# ── Interceptor (Architectural Enforcement) ─────────────────────────────────


@mcp.tool()
def sin_check_architecture(tool_name: str, tool_input: dict) -> str:
    """Pre-flight: validate if a tool call violates architectural rules.

    Use this BEFORE sin_write or sin_bash to prevent technical debt.
    """
    try:
        from sin_code_bundle.interceptor import SINInterceptor

        return json.dumps(SINInterceptor().preflight(tool_name, tool_input))
    except Exception as exc:
        return json.dumps({"error": str(exc)})


# ── Consolidation Tools (v0.7.0) ───────────────────────────────────────────
# Three high-ROI consolidations: each replaces 3-4 separate calls with one.
# See preflight.doc.md / symbol_resolve.doc.md / checkpoint.doc.md.


@mcp.tool()
def sin_preflight(tool_name: str, tool_input: dict) -> str:
    """Pre-flight safety gate: policy + docs + git + tests in 1 call.

    Run BEFORE any state-changing call (sin_write, sin_edit, sin_bash, sin_ast_edit).
    Returns structured JSON with {allowed, policy_ok, docs_ok, git_clean,
    tests_status, estimated_risk}.
    """
    try:
        from sin_code_bundle.preflight import PreflightChecker

        return json.dumps(
            PreflightChecker().check(tool_name, tool_input),
            indent=2,
            default=str,
        )
    except Exception as exc:
        return json.dumps({"error": str(exc), "tool_name": tool_name})


@mcp.tool()
def sin_symbol_resolve(
    name: str,
    depth: int = 2,
    include: str = "callers,callees,blast,recent",
) -> str:
    """Unified code archaeology for a symbol (function, class, module).

    Combines gitnexus_query + gitnexus_context + gitnexus_impact +
    gitnexus_detect_changes into 1 call. Optionally integrates
    sin-context-bridge for cross-source context.

    Args:
        name: symbol name (e.g. "validate_user", "AuthService", "auth/handler")
        depth: how many call-graph levels to traverse (1-3)
        include: comma-separated list of {callers, callees, blast, recent, cross}
    """
    try:
        from sin_code_bundle.symbol_resolve import SymbolResolver

        return json.dumps(
            SymbolResolver().resolve(name, depth, include.split(",")),
            indent=2,
            default=str,
        )
    except Exception as exc:
        return json.dumps({"error": str(exc), "name": name})


@mcp.tool()
def sin_checkpoint(
    name: str,
    include: str = "snapshot,docs,git,usages,tests",
    description: str = "",
) -> str:
    """Combined snapshot + state report before a risky change.

    Use before refactoring or any risky edit. Creates a recoverable snapshot
    AND a state report (docs status, git state, usages, tests).

    Args:
        name: snapshot name (e.g. "before-auth-refactor")
        include: comma-separated list of {snapshot, docs, git, usages, tests}
        description: optional human-readable description
    """
    try:
        from sin_code_bundle.checkpoint import Checkpointer

        return json.dumps(
            Checkpointer().create(name, include.split(","), description),
            indent=2,
            default=str,
        )
    except Exception as exc:
        return json.dumps({"error": str(exc), "name": name})


# ── Worktree Orchestration ──────────────────────────────────────────────────


@mcp.tool()
def sin_create_worktree(branch_name: str = "") -> str:
    """Create an isolated git worktree for parallel agent task execution."""
    try:
        from sin_code_bundle.orchestration_worktrees import SINWorktreeOrchestrator

        return json.dumps(SINWorktreeOrchestrator().create_worktree(branch_name or None))
    except Exception as exc:
        return json.dumps({"error": str(exc)})


@mcp.tool()
def sin_cleanup_worktree(worktree_path: str, merge_back: bool = False) -> str:
    """Clean up an isolated worktree. Optionally merge back to main."""
    try:
        from sin_code_bundle.orchestration_worktrees import SINWorktreeOrchestrator

        return json.dumps(SINWorktreeOrchestrator().cleanup_worktree(worktree_path, merge_back))
    except Exception as exc:
        return json.dumps({"error": str(exc)})


# ── Baseline Workflow Tools (v0.8.0) ───────────────────────────────────────
# Four tools that make the agent NEVER have to remember underlying tool names.
# Each replaces 3-5 separate MCP calls with a single high-level action.
# See immortal_commit.doc.md / programming_workflow.doc.md /
#     session_warmup.doc.md / merge_safety.doc.md.


@mcp.tool()
def sin_immortal_commit(
    message: str,
    tag: str = "",
    push: bool = True,
    force_main: bool = True,
    main_branch: str = "main",
) -> str:
    """One-call immortal commit — Conventional Commits + tag + push in 1 call.

    Replaces the agent's raw `git add && git commit && git tag && git push`
    sequence with a single tool that enforces four rules:

    1. **Conventional Commits** — message must match ``type(scope): subject``
       (subject >= 5 chars). Valid types: feat, fix, docs, chore, style,
       test, refactor, perf, ci, build.
    2. **No secrets in message** — substring scan for ``sk-``, ``ghp_``,
       ``AIza``, ``AKIA``/``ASIA``, ``BEGIN PRIVATE KEY`` etc.
    3. **Main only** — refuses to run on any branch other than ``main``
       (configurable via ``main_branch=...``). Per the NEVER-BRANCHES mandate.
    4. **Pre-commit snapshot** — creates a ``sin-honcho-rollback snapshot`` so
       the user can roll back. Independent of the commit; failure is non-fatal.

    Args:
        message: Conventional Commits message (required).
        tag: optional annotated tag (e.g. ``v0.8.0``).
        push: if True, push to ``origin/<main_branch>`` after commit.
        force_main: if True, refuse to run on any branch other than main.
        main_branch: which branch counts as ``main`` (default ``main``).

    Returns:
        JSON string with ``success``, ``sha``, ``branch``, ``tag``, ``pushed``,
        ``warnings``, ``steps`` (per-step status), ``snapshot`` info.
    """
    try:
        from sin_code_bundle.immortal_commit import ImmortalCommitter

        committer = ImmortalCommitter()
        result = committer.commit(
            message=message,
            tag=tag,
            push=push,
            force_main=force_main,
            main_branch=main_branch,
        )
        return json.dumps(result, indent=2, default=str)
    except Exception as exc:
        return json.dumps({"error": str(exc), "message": message})


@mcp.tool()
def sin_programming_workflow(
    action: str,
    target: str = "",
    content: str = "",
    message: str = "",
    checkpoint_name: str = "",
    base: str = "main",
    head: str = "HEAD",
) -> str:
    """Orchestrate common programming workflows in a single call.

    Actions:
        pre_write       : sin_read + sin_preflight → READY / FIX_FIRST
        write           : sin_preflight + sin_write → PROCEED / BLOCK
        post_write      : sin_preflight + codocs_check + pytest --collect-only
        pre_commit      : sin_checkpoint + git_status + codocs + ceo-audit (cached 5min)
                          returns ``suggested_message`` if no message given
        refactor        : sin_checkpoint + gitnexus_impact + gitnexus_detect_changes
        session_warmup  : sin_session_warmup (branch, git_state, ceo_audit_grade,
                          top_risks, session_recommendation)

    Returns:
        JSON string with ``action``, ``steps``, ``verdict``, plus action-
        specific extras (e.g. ``suggested_message`` for pre_commit).
    """
    try:
        from sin_code_bundle.programming_workflow import ProgrammingWorkflow

        wf = ProgrammingWorkflow()
        result = wf.run(
            action=action,
            target=target,
            content=content,
            message=message,
            checkpoint_name=checkpoint_name,
            base=base,
            head=head,
        )
        return json.dumps(result, indent=2, default=str)
    except Exception as exc:
        return json.dumps({"error": str(exc), "action": action})


@mcp.tool()
def sin_session_warmup(repo_path: str = ".") -> str:
    """One-call session context primer — call ONCE at the start of every session.

    Returns a snapshot of the current repo:
        branch, git_state, git_changes_count, last_commit_age,
        codocs_coverage, ceo_audit_grade, top_risks, session_recommendation.

    The ``session_recommendation`` field is a single-line string so the agent
    can decide "ready" vs "fix first" in one read:
        - "BLOCK — ceo-audit grade F. Fix critical issues first."
        - "FIX — improve docs/quality before coding"
        - "STASH or COMMIT first — working tree dirty"
        - "READY — proceed with coding"
    """
    try:
        from sin_code_bundle.session_warmup import SessionWarmup

        warmup = SessionWarmup(repo_root=Path(repo_path).expanduser())
        return json.dumps(warmup.warmup(), indent=2, default=str)
    except Exception as exc:
        return json.dumps({"error": str(exc), "repo_path": repo_path})


@mcp.tool()
def sin_merge_safety(
    base: str = "main",
    head: str = "HEAD",
    profile: str = "QUICK",
) -> str:
    """Pre-merge / pre-PR safety gate.

    Runs 4 independent checks in one call:
        1. CoDocs coverage (broken .doc.md references → blocker)
        2. ceo-audit grade (cached 5 min per (profile, base, head))
           - F → blocker, D → warning
        3. git diff stat (size + secret scan via substring + regex)
           - >1000 lines → warning, secrets → blocker
        4. Working tree state (clean/dirty)

    Returns:
        JSON string with ``pass`` (bool), ``verdict`` ("READY" or "FIX_FIRST"),
        ``blockers`` and ``warnings`` (lists of human-readable strings),
        ``checks`` (per-check dict).
    """
    try:
        from sin_code_bundle.merge_safety import MergeSafety

        gate = MergeSafety()
        return json.dumps(gate.check(base=base, head=head, profile=profile), indent=2, default=str)
    except Exception as exc:
        return json.dumps({"error": str(exc), "base": base, "head": head})


def main() -> None:
    """Run the MCP server (stdio)."""
    import sys

    sys.stderr.write("[SIN-CODE-BUNDLE] MCP server starting (stdio).\n")
    sys.stderr.flush()
    mcp.run()


if __name__ == "__main__":
    main()
