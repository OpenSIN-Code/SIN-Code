# SPDX-License-Identifier: MIT
"""Purpose: Tests for the `sin-read` CLI shim.

Docs: test_sin_read.doc.md

Run as module:
    python -m sin_code_bundle.cli_shims.sin_read <path>

Tests cover:
- Existing file (success path)
- Summarize mode (returns structural overview)
- Non-existent file (graceful JSON error)
- Directory path (returns listing)
- URI scheme (graceful error when sckg not installed)
"""
from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


def _run_cli(*args: str) -> subprocess.CompletedProcess:
    """Run `python -m sin_code_bundle.cli_shims.sin_read` with the given args."""
    return subprocess.run(
        [sys.executable, "-m", "sin_code_bundle.cli_shims.sin_read", *args],
        capture_output=True,
        text=True,
        timeout=30,
    )


def test_sin_read_existing_file(tmp_path: Path):
    """An existing file returns JSON with `content`, `chars`, `truncated`."""
    f = tmp_path / "hello.txt"
    f.write_text("hello world")
    result = _run_cli(str(f))
    assert result.returncode == 0, f"stderr: {result.stderr}"
    data = json.loads(result.stdout)
    assert "content" in data
    assert "hello world" in data["content"]
    assert data["chars"] == 11
    assert data["truncated"] is False


def test_sin_read_summarize_mode(tmp_path: Path):
    """`--summarize` returns a structural overview, NOT the raw content."""
    f = tmp_path / "lines.txt"
    lines = [f"line {i}" for i in range(20)]
    f.write_text("\n".join(lines))
    result = _run_cli(str(f), "--summarize")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert "lines" in data
    assert data["lines"] == 20
    assert "first_5" in data and len(data["first_5"]) == 5
    assert "last_5" in data and len(data["last_5"]) == 5
    # In summarize mode, `content` must NOT be present
    assert "content" not in data


def test_sin_read_nonexistent_file():
    """A missing path returns JSON error (not a non-zero exit)."""
    result = _run_cli("/no/such/file/sin-read-test")
    # Implementation returns JSON error inside body, exit code stays 0.
    # The agent parses the JSON to detect errors — non-zero exit would
    # break that contract.
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert "error" in data
    assert "not found" in data["error"]


def test_sin_read_directory_returns_listing(tmp_path: Path):
    """A directory path returns a JSON listing of children (not content)."""
    (tmp_path / "a.txt").write_text("a")
    (tmp_path / "b.txt").write_text("b")
    result = _run_cli(str(tmp_path))
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["type"] == "directory"
    assert "a.txt" in data["items"]
    assert "b.txt" in data["items"]


def test_sin_read_max_chars_truncation(tmp_path: Path):
    """`--max-chars 10` truncates a longer file to head+tail halves."""
    f = tmp_path / "long.txt"
    f.write_text("A" * 100)
    result = _run_cli(str(f), "--max-chars", "10")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["truncated"] is True
    assert data["chars"] == 100  # original size preserved
    # head and tail are each max_chars//2 = 5 chars
    assert len(data["content"]) == 5
    assert len(data["tail"]) == 5


def test_sin_read_uri_scheme_sckg():
    """URI scheme falls through to VirtualFS (graceful error if sckg absent)."""
    result = _run_cli("sckg://module/foo")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    # SCKG is optional. Either it returns real data OR an error — both fine.
    assert "error" in data or "module" in data
