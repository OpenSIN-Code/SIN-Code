"""Tests for the CEO Audit's local read-only legacy protocol adapters."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

SKILL_ROOT = Path(__file__).resolve().parents[1]
COMPAT_BIN = SKILL_ROOT / "scripts" / "compat-bin"


def _call(tool: str, arguments: dict) -> dict:
    request = {
        "jsonrpc": "2.0",
        "method": "tools/call",
        "id": 7,
        "params": {"name": tool, "arguments": arguments},
    }
    result = subprocess.run(
        [str(COMPAT_BIN / tool), "--mcp"],
        input=json.dumps(request) + "\n",
        text=True,
        capture_output=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    return json.loads(result.stdout)


def _text(response: dict) -> str:
    return response["result"]["content"][0]["text"]


def test_scout_adapter_returns_match_lines(tmp_path: Path) -> None:
    (tmp_path / "app.py").write_text("safe = True\nAPI_TOKEN = 'synthetic-test-key'\n")
    response = _call(
        "scout",
        {
            "path": str(tmp_path),
            "query": "API_TOKEN",
            "search_type": "regex",
            "max_results": 10,
        },
    )

    assert response["id"] == 7
    assert "Match: app.py:2:" in _text(response)


def test_discover_adapter_supports_brace_globs_and_line_counts(tmp_path: Path) -> None:
    (tmp_path / "one.py").write_text("a\nb\n")
    (tmp_path / "two.go").write_text("package main\n")
    (tmp_path / "skip.txt").write_text("ignored\n")
    response = _call(
        "discover",
        {
            "path": str(tmp_path),
            "pattern": "**/*.{py,go}",
            "max_results": 10,
        },
    )
    text = _text(response)

    assert "one.py — 2 lines" in text
    assert "two.go — 1 lines" in text
    assert "skip.txt" not in text


def test_map_adapter_summarizes_repository_without_network(tmp_path: Path) -> None:
    (tmp_path / "src").mkdir()
    (tmp_path / "src" / "app.py").write_text("print('ok')\n")
    response = _call("map", {"path": str(tmp_path), "action": "map"})
    text = _text(response)

    assert "Files: 1" in text
    assert "- src: 1" in text
    assert "- .py: 1" in text


def test_scout_adapter_repairs_legacy_unescaped_regex_json(tmp_path: Path) -> None:
    (tmp_path / "app.py").write_text("api_key = 'abcdefghijklmnopqrstuvwxyz'\n")
    # Deliberately mirrors the invalid JSON emitted by old shell heredocs:
    # \s and \' are legal regex escapes/content but invalid JSON escapes.
    raw = (
        '{"jsonrpc":"2.0","method":"tools/call","id":11,'
        '"params":{"name":"scout","arguments":{'
        f'"path":"{tmp_path}",'
        '"query":"api_key\\s*=\\s*[\\\']+[a-z]{20,}",'
        '"search_type":"regex","max_results":10}}}\n'
    )
    result = subprocess.run(
        [str(COMPAT_BIN / "scout"), "--mcp"],
        input=raw,
        text=True,
        capture_output=True,
        check=False,
    )

    assert result.returncode == 0, result.stdout + result.stderr
    response = json.loads(result.stdout)
    assert response["id"] == 11
    assert "Match: app.py:1:" in _text(response)


def test_scout_adapter_repairs_exact_legacy_quote_class(tmp_path: Path) -> None:
    (tmp_path / "config.py").write_text('api_key = "abcdefghijklmnopqrstuvwxyz"\n')
    query = r"""(api[_-]?key|secret[_-]?key|password|token)\s*=\s*['\\"][A-Za-z0-9]{20,}"""
    raw = (
        '{"jsonrpc":"2.0","method":"tools/call","id":11,'
        '"params":{"name":"scout","arguments":{'
        f'"path":"{tmp_path}","query":"{query}",'
        '"search_type":"regex","max_results":100,"include_context":true}}}\n'
    )
    result = subprocess.run(
        [str(COMPAT_BIN / "scout"), "--mcp"],
        input=raw,
        text=True,
        capture_output=True,
        check=False,
    )

    assert result.returncode == 0, result.stdout + result.stderr
    response = json.loads(result.stdout)
    assert response["id"] == 11
    assert "Match: config.py:1:" in _text(response)
