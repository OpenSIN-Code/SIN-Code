# SPDX-License-Identifier: MIT
"""Built-in tools wired into the ToolRouter — thin async wrappers."""

from __future__ import annotations

import asyncio
import os
import re
from pathlib import Path
from typing import Any

from .router import ToolRouter

_REDACT = [
    re.compile(r"(?i)(api[_-]?key|secret|token|password)=\S+"),
    re.compile(r"AKIA[0-9A-Z]{16}"),
    re.compile(r"ghp_[A-Za-z0-9]{36}"),
]


def _redact(text: str) -> str:
    for pat in _REDACT:
        text = pat.sub("[REDACTED]", text)
    return text


async def tool_bash(*, cmd: str, cwd: str, timeout_s: float = 300.0) -> dict[str, Any]:
    proc = await asyncio.create_subprocess_shell(
        cmd, cwd=cwd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        env={**os.environ, "GIT_TERMINAL_PROMPT": "0"},
    )
    try:
        out, err = await asyncio.wait_for(proc.communicate(), timeout=timeout_s)
    except asyncio.TimeoutError:
        proc.kill()
        raise RuntimeError(f"command timed out after {timeout_s}s: {cmd}")
    result = {
        "exit_code": proc.returncode,
        "stdout": _redact(out.decode(errors="replace")[-16000:]),
        "stderr": _redact(err.decode(errors="replace")[-8000:]),
    }
    if proc.returncode != 0:
        raise RuntimeError(
            f"exit {proc.returncode}: {result['stderr'][:1000] or result['stdout'][:1000]}"
        )
    return result


async def tool_read(*, path: str, cwd: str,
                    start: int = 1, limit: int = 400) -> dict[str, Any]:
    p = (Path(cwd) / path).resolve()
    if not str(p).startswith(str(Path(cwd).resolve())):
        raise PermissionError(f"path escapes workspace: {path}")
    lines = p.read_text(encoding="utf-8", errors="replace").splitlines()
    window = lines[start - 1: start - 1 + limit]
    return {
        "path": str(p),
        "total_lines": len(lines),
        "start": start,
        "content": "\n".join(
            f"{i + start}\t{line}" for i, line in enumerate(window)
        ),
    }


async def tool_write(*, path: str, content: str, cwd: str) -> dict[str, Any]:
    p = (Path(cwd) / path).resolve()
    if not str(p).startswith(str(Path(cwd).resolve())):
        raise PermissionError(f"path escapes workspace: {path}")
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8")
    return {"path": str(p), "bytes": len(content.encode())}


async def tool_edit(*, path: str, old: str, new: str, cwd: str) -> dict[str, Any]:
    p = (Path(cwd) / path).resolve()
    text = p.read_text(encoding="utf-8")
    count = text.count(old)
    if count == 0:
        raise ValueError(f"anchor not found in {path}")
    if count > 1:
        raise ValueError(
            f"anchor ambiguous in {path} ({count} matches) — "
            "provide more surrounding context"
        )
    p.write_text(text.replace(old, new, 1), encoding="utf-8")
    return {"path": str(p), "replaced": 1}


async def tool_search(*, pattern: str, cwd: str,
                      glob: str = "**/*.py", limit: int = 50) -> dict[str, Any]:
    rx = re.compile(pattern)
    hits: list[dict[str, Any]] = []
    root = Path(cwd)
    for f in root.glob(glob):
        if not f.is_file() or ".git" in f.parts or "node_modules" in f.parts:
            continue
        try:
            for i, line in enumerate(
                f.read_text(encoding="utf-8", errors="replace").splitlines(), 1
            ):
                if rx.search(line):
                    hits.append({"file": str(f.relative_to(root)),
                                 "line": i, "text": line.strip()[:200]})
                    if len(hits) >= limit:
                        return {"hits": hits, "truncated": True}
        except OSError:
            continue
    return {"hits": hits, "truncated": False}


def register_builtin_tools(router: ToolRouter) -> ToolRouter:
    router.register("sin_bash", tool_bash, failure_threshold=8)
    router.register("sin_read", tool_read)
    router.register("sin_write", tool_write)
    router.register("sin_edit", tool_edit)
    router.register("sin_search", tool_search)
    return router
