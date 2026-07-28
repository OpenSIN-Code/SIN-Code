#!/usr/bin/env python3
"""Local compatibility adapters for the retired ``tool --mcp`` interface.

The CEO Audit predates the unified ``sin-code`` binary and historically called
``scout --mcp``, ``discover --mcp`` and ``map --mcp``.  These adapters keep the
audit self-contained without re-introducing that legacy protocol into the
product runtime.  They intentionally implement only the read-only subset used
by the audit.
"""

from __future__ import annotations

import fnmatch
import json
import os
import re
import shutil
import subprocess
import sys
from collections import Counter
from pathlib import Path
from typing import Any, Iterable

EXCLUDED_DIRS = {
    ".git",
    ".hg",
    ".svn",
    ".venv",
    "venv",
    "node_modules",
    "vendor",
    "dist",
    "build",
    "coverage",
    "__pycache__",
}
MAX_FILE_BYTES = 2 * 1024 * 1024


def _loads_legacy_request(raw: str) -> dict[str, Any]:
    """Parse JSON emitted by the historical shell heredocs.

    The legacy audit embedded regular expressions directly in JSON strings, so
    sequences such as ``\\s`` and ``\\(`` were not escaped for JSON. The
    ``query`` value may also contain a quote that was not escaped at the JSON
    layer. We isolate that fixed-position field, serialize its bytes correctly,
    then apply a conservative invalid-backslash repair as a final fallback.
    """
    candidate = raw
    query_marker = '"query":"'
    next_field_marker = '","search_type"'
    query_start = candidate.find(query_marker)
    if query_start >= 0:
        value_start = query_start + len(query_marker)
        value_end = candidate.find(next_field_marker, value_start)
        if value_end >= 0:
            raw_query = candidate[value_start:value_end]
            candidate = (
                candidate[:query_start]
                + '"query":'
                + json.dumps(raw_query)
                + candidate[value_end + 1 :]
            )
    try:
        value = json.loads(candidate)
    except json.JSONDecodeError:
        repaired = re.sub(r'\\(?!["\\/bfnrtu])', r"\\\\", candidate)
        value = json.loads(repaired)
    if not isinstance(value, dict):
        raise ValueError("JSON-RPC request must be an object")
    return value


def _response(request_id: Any, text: str) -> dict[str, Any]:
    return {
        "jsonrpc": "2.0",
        "id": request_id,
        "result": {"content": [{"type": "text", "text": text}]},
    }


def _error(request_id: Any, message: str) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": -32000, "message": message}}


def _iter_text_files(root: Path) -> Iterable[Path]:
    for current_root, dirnames, filenames in os.walk(root):
        dirnames[:] = [name for name in dirnames if name not in EXCLUDED_DIRS]
        base = Path(current_root)
        for name in filenames:
            path = base / name
            try:
                if path.stat().st_size > MAX_FILE_BYTES:
                    continue
                with path.open("rb") as handle:
                    head = handle.read(4096)
                if b"\x00" in head:
                    continue
            except OSError:
                continue
            yield path


def _rg_search(root: Path, query: str, max_results: int) -> list[str] | None:
    rg = shutil.which("rg")
    if not rg:
        return None
    command = [
        rg,
        "--pcre2",
        "--no-heading",
        "--line-number",
        "--color",
        "never",
        "--max-filesize",
        "2M",
        "--glob",
        "!.git/**",
        "--glob",
        "!.venv/**",
        "--glob",
        "!node_modules/**",
        "--glob",
        "!vendor/**",
        "--glob",
        "!dist/**",
        "--glob",
        "!build/**",
        "--max-count",
        str(max_results),
        query,
        ".",
    ]
    try:
        process = subprocess.run(
            command,
            cwd=root,
            text=True,
            capture_output=True,
            timeout=30,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    if process.returncode not in (0, 1):
        return None
    return [
        line[2:] if line.startswith("./") else line
        for line in process.stdout.splitlines()[:max_results]
    ]


def _python_search(root: Path, query: str, max_results: int) -> list[str]:
    try:
        pattern = re.compile(query)
    except re.error:
        pattern = re.compile(re.escape(query))
    results: list[str] = []
    for path in _iter_text_files(root):
        try:
            text = path.read_text(errors="ignore")
        except OSError:
            continue
        for line_number, line in enumerate(text.splitlines(), 1):
            if pattern.search(line):
                try:
                    relative = path.relative_to(root)
                except ValueError:
                    relative = path
                results.append(f"{relative}:{line_number}:{line[:500]}")
                if len(results) >= max_results:
                    return results
    return results


def _scout(arguments: dict[str, Any]) -> str:
    root = Path(arguments.get("path") or ".").expanduser().resolve()
    query = str(arguments.get("query") or "")
    max_results = max(1, min(int(arguments.get("max_results") or 100), 5000))
    if not root.exists() or not query:
        return ""
    results = _rg_search(root, query, max_results)
    if results is None:
        results = _python_search(root, query, max_results)
    return "\n".join(f"Match: {line}" for line in results)


def _expand_braces(pattern: str) -> list[str]:
    match = re.search(r"\{([^{}]+)\}", pattern)
    if not match:
        return [pattern]
    prefix, suffix = pattern[: match.start()], pattern[match.end() :]
    return [prefix + option + suffix for option in match.group(1).split(",")]


def _matches_any(relative: str, patterns: list[str]) -> bool:
    for pattern in patterns:
        candidates = [pattern]
        if pattern.startswith("**/"):
            candidates.append(pattern[3:])
        if any(fnmatch.fnmatch(relative, candidate) for candidate in candidates):
            return True
    return False


def _line_count(path: Path) -> int:
    try:
        data = path.read_bytes()
    except OSError:
        return 0
    if not data:
        return 0
    return data.count(b"\n") + (0 if data.endswith(b"\n") else 1)


def _discover(arguments: dict[str, Any]) -> str:
    root = Path(arguments.get("path") or ".").expanduser().resolve()
    pattern = str(arguments.get("pattern") or "**/*")
    max_results = max(1, min(int(arguments.get("max_results") or 500), 10000))
    patterns = _expand_braces(pattern)
    rows: list[tuple[str, int]] = []
    if not root.exists():
        return ""
    for path in _iter_text_files(root):
        try:
            relative = path.relative_to(root).as_posix()
        except ValueError:
            relative = path.as_posix()
        if not _matches_any(relative, patterns):
            continue
        rows.append((relative, _line_count(path)))
        if len(rows) >= max_results:
            break
    return "\n".join(f"{relative} — {lines} lines" for relative, lines in rows)


def _map(arguments: dict[str, Any]) -> str:
    root = Path(arguments.get("path") or ".").expanduser().resolve()
    extensions: Counter[str] = Counter()
    directories: Counter[str] = Counter()
    total = 0
    if not root.exists():
        return ""
    for path in _iter_text_files(root):
        total += 1
        extensions[path.suffix.lower() or "[no extension]"] += 1
        try:
            relative = path.relative_to(root)
            directories[relative.parts[0] if len(relative.parts) > 1 else "."] += 1
        except ValueError:
            directories["."] += 1
    lines = [f"Repository: {root}", f"Files: {total}", "Top directories:"]
    lines.extend(f"- {name}: {count}" for name, count in directories.most_common(20))
    lines.append("Top extensions:")
    lines.extend(f"- {name}: {count}" for name, count in extensions.most_common(20))
    return "\n".join(lines)


def _dispatch(tool: str, arguments: dict[str, Any]) -> str:
    if tool == "scout":
        return _scout(arguments)
    if tool == "discover":
        return _discover(arguments)
    if tool == "map":
        return _map(arguments)
    raise ValueError(f"unsupported compatibility tool: {tool}")


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: tool_mcp.py <scout|discover|map> --mcp", file=sys.stderr)
        return 2
    tool = sys.argv[1]
    if "--mcp" not in sys.argv[2:]:
        print(f"{tool}: this compatibility adapter only supports --mcp", file=sys.stderr)
        return 2
    exit_code = 0
    for raw in sys.stdin:
        raw = raw.strip()
        if not raw:
            continue
        try:
            request = _loads_legacy_request(raw)
            request_id = request.get("id")
            arguments = request.get("params", {}).get("arguments", {})
            text = _dispatch(tool, arguments)
            response = _response(request_id, text)
        except Exception as exc:  # noqa: BLE001 - protocol boundary must return JSON errors
            response = _error(locals().get("request_id"), str(exc))
            exit_code = 1
        print(json.dumps(response, ensure_ascii=False), flush=True)
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
