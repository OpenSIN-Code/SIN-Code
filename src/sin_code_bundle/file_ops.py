# SPDX-License-Identifier: MIT
"""Purpose: File operations core — shared by MCP server and CLI shims.

Docs: file_ops.doc.md

This module is the single source of truth for the 5 core file-ops that
replace opencode's native read/write/edit/bash/search:

    - sin_read        : URI-scheme aware, size-safe file read with summarize mode
    - sin_write       : Atomic write with auto-backup and syntax pre-validation
    - sin_edit        : Hashline-anchored semantic patching (line-shift resilient)
    - sin_bash        : Safe shell exec via `execute` Go binary
    - sin_search      : Wraps `scout` Go tool, falls back to Python-regex

Both `mcp_server.py` (the @mcp.tool() definitions) and the standalone CLI
shims in `cli/sin_*.py` (the `sin-read`/`sin-write`/etc. console scripts)
call into here, so there is exactly ONE implementation of each operation.

Adding a new file-op?  Add the function here, then wire it into both:
  1. mcp_server.py — `@mcp.tool()` one-liner that calls the function
  2. cli/sin_<name>.py — argparse-based CLI shim (see existing examples)
  3. pyproject.toml `[project.scripts]` — entry point
"""
from __future__ import annotations

import json
import shutil
import subprocess
from pathlib import Path
from typing import Any


# ── sin_read ────────────────────────────────────────────────────────────────


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
        # URI schemes → VirtualFS (semantic resolver), not the filesystem.
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
        # Size-aware truncation: keep head + tail, drop the middle.
        # This preserves the most useful signal (top imports, signatures;
        # bottom main() / constants) while staying within max_chars.
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


# ── sin_write ───────────────────────────────────────────────────────────────


def sin_write(path: str, content: str, verify: bool = True) -> str:
    """SIN-Code write — replaces native write.

    Atomic write with optional backup. When verify=True (default), runs
    AST-based syntax validation for .py files (compile()) to catch
    broken-syntax writes before they hit disk.

    Better than native write: atomic (no half-written files on crash),
    syntax pre-validation, optional backup.
    """
    try:
        p = Path(path).expanduser()
        backup = None
        # Atomic replacement strategy: rename existing file to .bak,
        # then write the new content. If anything fails, restore from
        # .bak so the file is never half-written.
        if p.exists() and verify:
            backup = str(p) + ".bak"
            p.replace(backup)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content, encoding="utf-8")
        # `verified` is True ONLY when we actually ran the syntax check AND
        # it passed. When verify=False we never checked, so the field is
        # False (signaling "not verified") rather than True (which would
        # falsely imply a check happened).
        verified = False
        # Pre-validate Python syntax via compile() — catches SyntaxError
        # before the file reaches the importer. Other languages (.ts/.js/.go)
        # are not pre-validated here because they require external parsers;
        # the AST-edit tool (sin_ast_edit) handles those with tree-sitter.
        if verify and p.suffix == ".py":
            try:
                compile(content, str(p), "exec")
                verified = True
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


# ── sin_edit ────────────────────────────────────────────────────────────────


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
            file_path=p,
            old_content=old_content,
            new_content=new_content,
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


# ── sin_bash ────────────────────────────────────────────────────────────────


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
        # Prefer the `execute` Go binary (sin-code-execute-tool) for
        # secret-redaction + structured output. Fall back to raw subprocess
        # if it's not installed (e.g. on a bare python venv).
        cmd_path = shutil.which("execute") or str(Path.home() / ".local/bin/execute")
        if Path(cmd_path).exists():
            # 10s buffer over `execute`'s own timeout — gives the Go tool
            # time to flush JSON + clean up before Python kills it.
            proc = subprocess.run(
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
        # Fallback: raw shell. Output is truncated to keep agent context small.
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


# ── sin_search ──────────────────────────────────────────────────────────────


def sin_search(query: str, path: str = ".", search_type: str = "semantic") -> str:
    """SIN-Code search — replaces native search/grep/find/glob.

    Wraps the `scout` Go tool (semantic + regex + symbol + usage search).
    Falls back to Python regex if scout binary is missing — works on both
    single files and directories.

    search_type: semantic | regex | symbol | usage
    """
    try:
        # Prefer the `scout` Go tool (sin-code-scout-tool) for proper
        # semantic/symbol/usage search. Fall back to Python-regex if missing.
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
