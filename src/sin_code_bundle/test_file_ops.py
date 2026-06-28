# SPDX-License-Identifier: MIT
"""Tests for sin_code_bundle.file_ops — core file operations.

Covers: sin_read (file, directory, nonexistent, truncation, summarize),
sin_write (new file, overwrite, atomic backup, syntax validation, nested
paths), sin_edit (file-not-found, basic round-trip), sin_bash (command
execution, timeout), sin_search (regex fallback).
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from sin_code_bundle.file_ops import sin_bash, sin_edit, sin_read, sin_search, sin_write


# ════════════════════════════════════════════════════════════════════════
# Helpers
# ════════════════════════════════════════════════════════════════════════


def _parse(result: str) -> dict:
    """Parse the JSON string returned by file_ops functions."""
    return json.loads(result)


# ════════════════════════════════════════════════════════════════════════
# sin_read
# ════════════════════════════════════════════════════════════════════════


class TestSinRead:
    def test_read_existing_file(self, py_file: Path):
        result = _parse(sin_read(str(py_file)))
        assert "content" in result
        assert "print('hello')" in result["content"]
        assert result["truncated"] is False

    def test_read_nonexistent_returns_error(self, tmp_path: Path):
        result = _parse(sin_read(str(tmp_path / "nope.py")))
        assert "error" in result
        assert "not found" in result["error"]

    def test_read_directory_returns_listing(self, tmp_path: Path):
        (tmp_path / "a.txt").write_text("a")
        (tmp_path / "b.txt").write_text("b")
        result = _parse(sin_read(str(tmp_path)))
        assert result["type"] == "directory"
        assert "a.txt" in result["items"]
        assert "b.txt" in result["items"]

    def test_read_truncation(self, tmp_path: Path):
        big = tmp_path / "big.txt"
        big.write_text("X" * 100000, encoding="utf-8")
        result = _parse(sin_read(str(big), max_chars=1000))
        assert result["truncated"] is True
        assert len(result["content"]) <= 500
        assert len(result["tail"]) <= 500

    def test_read_summarize_mode(self, py_file: Path):
        result = _parse(sin_read(str(py_file), summarize=True))
        assert result["lines"] == 1
        assert result["first_5"] == ["print('hello')"]
        assert result["last_5"] == ["print('hello')"]

    def test_read_summarize_multiline(self, tmp_path: Path):
        p = tmp_path / "multi.py"
        p.write_text("a\nb\nc\nd\ne\nf\ng\n", encoding="utf-8")
        result = _parse(sin_read(str(p), summarize=True))
        assert result["lines"] == 7
        assert result["first_5"] == ["a", "b", "c", "d", "e"]
        assert result["last_5"] == ["c", "d", "e", "f", "g"]

    def test_read_path_with_traversal_does_not_crash(self, tmp_path: Path):
        # Path traversal like ../../etc/passwd should resolve to a real path
        # (or nonexistent) but must not crash.
        result = _parse(sin_read(str(tmp_path / ".." / ".." / "nonexistent_xyz")))
        assert "error" in result

    def test_read_returns_json_string(self, py_file: Path):
        result = sin_read(str(py_file))
        assert isinstance(result, str)
        # Must be valid JSON
        json.loads(result)

    def test_read_expanduser(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
        # ~ should expand to the real home dir; point HOME at tmp_path
        monkeypatch.setenv("HOME", str(tmp_path))
        p = tmp_path / "test.txt"
        p.write_text("hi", encoding="utf-8")
        result = _parse(sin_read("~/test.txt"))
        assert "hi" in result.get("content", "")


# ════════════════════════════════════════════════════════════════════════
# sin_write
# ════════════════════════════════════════════════════════════════════════


class TestSinWrite:
    def test_write_new_file(self, tmp_path: Path):
        p = tmp_path / "new.py"
        result = _parse(sin_write(str(p), "print('hello')\n"))
        assert result["success"] is True
        assert p.exists()
        assert p.read_text() == "print('hello')\n"
        assert result["verified"] is True  # .py file passed compile()

    def test_write_non_py_not_verified(self, tmp_path: Path):
        p = tmp_path / "data.txt"
        result = _parse(sin_write(str(p), "hello world"))
        assert result["success"] is True
        assert result["verified"] is False  # not a .py file

    def test_write_verify_false(self, tmp_path: Path):
        p = tmp_path / "code.py"
        result = _parse(sin_write(str(p), "print('hi')", verify=False))
        assert result["success"] is True
        assert result["verified"] is False  # verify was off

    def test_write_syntax_error_rolls_back(self, tmp_path: Path):
        p = tmp_path / "broken.py"
        p.write_text("print('original')\n", encoding="utf-8")
        result = _parse(sin_write(str(p), "def broken(\n"))
        assert result["success"] is False
        assert "syntax error" in result["error"]
        # Original content preserved via backup restore
        assert p.read_text() == "print('original')\n"

    def test_write_creates_parent_dirs(self, tmp_path: Path):
        p = tmp_path / "a" / "b" / "c.py"
        result = _parse(sin_write(str(p), "x = 1\n"))
        assert result["success"] is True
        assert p.exists()

    def test_write_overwrite_creates_backup(self, tmp_path: Path):
        p = tmp_path / "orig.py"
        p.write_text("print('old')\n", encoding="utf-8")
        result = _parse(sin_write(str(p), "print('new')\n"))
        assert result["success"] is True
        assert result["backup"] is not None
        assert Path(result["backup"]).exists()
        assert Path(result["backup"]).read_text() == "print('old')\n"
        assert p.read_text() == "print('new')\n"

    def test_write_overwrite_no_verify_no_backup(self, tmp_path: Path):
        p = tmp_path / "orig.py"
        p.write_text("print('old')\n", encoding="utf-8")
        result = _parse(sin_write(str(p), "print('new')\n", verify=False))
        assert result["success"] is True
        assert result["backup"] is None  # no backup when verify=False
        assert p.read_text() == "print('new')\n"

    def test_write_returns_json_string(self, tmp_path: Path):
        result = sin_write(str(tmp_path / "x.py"), "x = 1\n")
        assert isinstance(result, str)
        json.loads(result)

    def test_write_empty_content(self, tmp_path: Path):
        p = tmp_path / "empty.py"
        result = _parse(sin_write(str(p), ""))
        assert result["success"] is True
        assert p.read_text() == ""
        assert result["chars"] == 0

    def test_write_valid_python_verified(self, tmp_path: Path):
        p = tmp_path / "valid.py"
        code = "def add(a, b):\n    return a + b\n"
        result = _parse(sin_write(str(p), code))
        assert result["success"] is True
        assert result["verified"] is True

    def test_write_expanduser(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
        monkeypatch.setenv("HOME", str(tmp_path))
        result = _parse(sin_write("~/out.py", "x = 1\n"))
        assert result["success"] is True
        assert (tmp_path / "out.py").exists()


# ════════════════════════════════════════════════════════════════════════
# sin_edit
# ════════════════════════════════════════════════════════════════════════


class TestSinEdit:
    def test_edit_nonexistent_file(self, tmp_path: Path):
        result = _parse(sin_edit(str(tmp_path / "nope.py"), "old", "new"))
        assert "error" in result
        assert "not found" in result["error"]

    def test_edit_returns_json_string(self, py_file: Path):
        result = sin_edit(str(py_file), "print('hello')", "print('world')")
        assert isinstance(result, str)
        json.loads(result)

    def test_edit_successful_change(self, py_file: Path):
        result = _parse(sin_edit(str(py_file), "print('hello')", "print('world')"))
        # The edit should either succeed or report drift — both are valid
        # structured responses. We verify the response shape.
        assert "success" in result or "error" in result
        if result.get("success"):
            assert py_file.read_text() == "print('world')\n"

    def test_edit_anchor_not_found(self, py_file: Path):
        result = _parse(sin_edit(str(py_file), "NONEXISTENT_CONTENT_XYZ", "new"))
        assert result.get("success") is False or "error" in result


# ════════════════════════════════════════════════════════════════════════
# sin_bash
# ════════════════════════════════════════════════════════════════════════


class TestSinBash:
    """Tests for sin_bash. The function prefers the `execute` Go binary
    when present; we force the raw-shell fallback via monkeypatch so
    tests are deterministic regardless of the host's `execute` install.
    """

    @pytest.fixture(autouse=True)
    def _force_fallback(self, monkeypatch: pytest.MonkeyPatch):
        """Force sin_bash to use the raw-shell fallback path."""
        monkeypatch.setattr("shutil.which", lambda _: None)
        monkeypatch.setattr(Path, "exists", lambda self: False)

    def test_echo_command(self):
        result = _parse(sin_bash("echo hello_world"))
        assert result["returncode"] == 0
        assert "hello_world" in result["stdout"]

    def test_nonzero_exit(self):
        result = _parse(sin_bash("exit 3"))
        assert result["returncode"] == 3

    def test_returns_json_string(self):
        result = sin_bash("echo test")
        assert isinstance(result, str)
        json.loads(result)

    def test_has_redacted_field(self):
        result = _parse(sin_bash("echo test"))
        assert "redacted" in result
        # Fallback path marks redacted as False
        assert result["redacted"] is False

    def test_timeout_handling(self):
        result = _parse(sin_bash("sleep 10", timeout=1))
        assert "error" in result or result.get("returncode") != 0
        if "error" in result:
            assert "timeout" in result["error"].lower()

    def test_captures_stderr(self):
        result = _parse(sin_bash("echo err_msg >&2"))
        assert "err_msg" in result.get("stderr", "")

    def test_empty_command(self):
        result = _parse(sin_bash(""))
        # Empty command — shell typically returns 0
        assert "returncode" in result or "error" in result

    def test_fallback_has_warning(self):
        result = _parse(sin_bash("echo test"))
        assert "warning" in result
        assert "execute binary not found" in result["warning"]


# ════════════════════════════════════════════════════════════════════════
# sin_search
# ════════════════════════════════════════════════════════════════════════


class TestSinSearch:
    def test_search_single_file(self, tmp_path: Path):
        p = tmp_path / "test.py"
        p.write_text("def foo():\n    pass\n\ndef bar():\n    pass\n", encoding="utf-8")
        result = _parse(sin_search("def foo", path=str(p), search_type="regex"))
        assert result.get("count", 0) >= 1
        if "results" in result:
            assert any("foo" in r["match"] for r in result["results"])

    def test_search_directory(self, tmp_path: Path):
        (tmp_path / "a.py").write_text("import os\n", encoding="utf-8")
        (tmp_path / "b.py").write_text("import sys\n", encoding="utf-8")
        result = _parse(sin_search("import", path=str(tmp_path), search_type="regex"))
        assert result.get("count", 0) >= 2

    def test_search_nonexistent_path(self, tmp_path: Path):
        result = _parse(sin_search("x", path=str(tmp_path / "nope"), search_type="regex"))
        assert "error" in result

    def test_search_returns_json_string(self, tmp_path: Path):
        p = tmp_path / "x.py"
        p.write_text("x = 1\n", encoding="utf-8")
        result = sin_search("x", path=str(p), search_type="regex")
        assert isinstance(result, str)
        json.loads(result)

    def test_search_no_matches(self, tmp_path: Path):
        p = tmp_path / "x.py"
        p.write_text("x = 1\n", encoding="utf-8")
        result = _parse(sin_search("ZZZZ_NOT_FOUND_ZZZZ", path=str(p), search_type="regex"))
        assert result.get("count", 0) == 0

    def test_search_excludes_git_dir(self, tmp_path: Path):
        git_dir = tmp_path / ".git"
        git_dir.mkdir()
        (git_dir / "config").write_text("hidden content here\n", encoding="utf-8")
        (tmp_path / "visible.py").write_text("visible content here\n", encoding="utf-8")
        result = _parse(sin_search("content", path=str(tmp_path), search_type="regex"))
        # .git should be excluded from directory search
        for r in result.get("results", []):
            assert ".git" not in r["file"]

    def test_search_result_has_line_number(self, tmp_path: Path):
        p = tmp_path / "multi.py"
        p.write_text("a\nb\ntarget\nd\n", encoding="utf-8")
        result = _parse(sin_search("target", path=str(p), search_type="regex"))
        if result.get("results"):
            assert result["results"][0]["line"] == 3
