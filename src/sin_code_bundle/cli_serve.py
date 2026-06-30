# SPDX-License-Identifier: MIT
"""Serve subcommand — extracted from cli.py for maintainability.

Registers the `serve` command on the shared Typer app.
"""
from __future__ import annotations

import json

import typer

from sin_code_bundle.cli_app import app

_EXCLUDE = {"venv", ".venv", "node_modules", ".git", "__pycache__"}


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
                    [cmd_path, "--timeout", str(timeout), "--format", "json", "--command", command],
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
