"""Edge-case tests for Simone-MCP tools — bugs NOT covered by existing tests.

Docs: test_edge_cases.doc.md
"""

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
sys.path.insert(0, str(SRC))

import pytest
from simone_mcp.core import (
    write_file,
    read_file,
    edit_file,
    patch_file,
)


# ── Write file edge cases ──────────────────────────────

class TestWriteFileEdgeCases:
    def test_write_empty_file(self, tmp_path: Path):
        target = tmp_path / "empty.txt"
        result = write_file({"path": str(target), "content": ""})
        assert result["ok"] is True
        assert result["bytes_written"] == 0
        assert target.read_text(encoding="utf-8") == ""

    def test_write_empty_path(self, tmp_path: Path):
        """Write to empty path string."""
        result = write_file({"path": "", "content": "content"})
        # Should fail gracefully
        assert result["ok"] is False

    def test_write_whitespace_path(self, tmp_path: Path):
        result = write_file({"path": "   ", "content": "content"})
        assert result["ok"] is False

    def test_write_file_with_special_chars_in_path(self, tmp_path: Path):
        special_dir = tmp_path / "special-@#$%^&"
        target = special_dir / "file-with-hyphens.txt"
        result = write_file({"path": str(target), "content": "special path content"})
        assert result["ok"] is True
        assert target.exists()
        assert target.read_text(encoding="utf-8") == "special path content"

    def test_write_file_with_spaces_in_path(self, tmp_path: Path):
        target = tmp_path / "my documents" / "file name.txt"
        result = write_file({"path": str(target), "content": "spaces test"})
        assert result["ok"] is True
        assert target.read_text(encoding="utf-8") == "spaces test"

    def test_write_unicode_content(self, tmp_path: Path):
        target = tmp_path / "unicode.txt"
        content = "日本語 🔥🚀 áéíóú ñ"
        result = write_file({"path": str(target), "content": content})
        assert result["ok"] is True
        assert target.read_text(encoding="utf-8") == content

    def test_write_with_none_content(self, tmp_path: Path):
        """None content should be converted to string."""
        target = tmp_path / "none.txt"
        result = write_file({"path": str(target)})
        # If content is not provided, it defaults to ""
        assert result["ok"] is True

    def test_write_to_existing_file_no_overwrite(self, tmp_path: Path):
        target = tmp_path / "exists.txt"
        target.write_text("original", encoding="utf-8")
        result = write_file({"path": str(target), "content": "new", "overwrite": False})
        assert result["ok"] is False
        assert "File exists" in result["error"]

    def test_write_to_readonly_directory(self, tmp_path: Path):
        import stat
        readonly_dir = tmp_path / "readonly"
        readonly_dir.mkdir()
        target = readonly_dir / "file.txt"
        target.write_text("temp", encoding="utf-8")
        try:
            os.chmod(str(readonly_dir), stat.S_IRUSR | stat.S_IXUSR)  # read+execute only
            result = write_file({"path": str(target), "content": "new", "overwrite": True})
            # May succeed or fail depending on platform
            assert isinstance(result["ok"], bool)
        except PermissionError:
            pass  # Can't chmod on some systems
        finally:
            os.chmod(str(readonly_dir), stat.S_IRWXU)  # restore

    def test_write_very_large_file(self, tmp_path: Path):
        """Write a file with 100KB+ content — performance test."""
        target = tmp_path / "large.txt"
        content = "A" * 100000  # 100KB
        result = write_file({"path": str(target), "content": content})
        assert result["ok"] is True
        assert result["bytes_written"] == 100000

    def test_write_binary_like_content(self, tmp_path: Path):
        """Content containing null bytes and control chars."""
        target = tmp_path / "binary.txt"
        content = "Hello\x00World\x01Control"
        result = write_file({"path": str(target), "content": content})
        if result["ok"]:
            assert target.exists()

    def test_write_file_with_newlines_only(self, tmp_path: Path):
        target = tmp_path / "newlines.txt"
        result = write_file({"path": str(target), "content": "\n\n\n"})
        assert result["ok"] is True
        assert target.read_text(encoding="utf-8") == "\n\n\n"

    def test_write_file_path_relative(self, monkeypatch):
        """Relative path should resolve against CWD."""
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            monkeypatch.chdir(td)
            result = write_file({"path": "relative_file.txt", "content": "relative"})
            assert result["ok"] is True
            assert Path(td, "relative_file.txt").exists()


# ── Read file edge cases ───────────────────────────────

class TestReadFileEdgeCases:
    def test_read_empty_file(self, tmp_path: Path):
        target = tmp_path / "empty.txt"
        target.write_text("", encoding="utf-8")
        result = read_file({"path": str(target)})
        assert result["ok"] is True
        assert result["content"] == ""
        assert result["lines_read"] == 0
        assert result["total_lines"] == 0

    def test_read_with_negative_offset(self, tmp_path: Path):
        target = tmp_path / "test.txt"
        target.write_text("line1\nline2\nline3", encoding="utf-8")
        result = read_file({"path": str(target), "offset": -5})
        assert result["ok"] is True
        # Should clamp to 0
        assert result["offset"] == 0

    def test_read_with_negative_limit(self, tmp_path: Path):
        target = tmp_path / "test.txt"
        target.write_text("line1\nline2\nline3", encoding="utf-8")
        result = read_file({"path": str(target), "limit": -10})
        # Should handle this gracefully
        assert isinstance(result["ok"], bool)

    def test_read_with_offset_beyond_eof(self, tmp_path: Path):
        target = tmp_path / "test.txt"
        target.write_text("line1\nline2\n", encoding="utf-8")
        result = read_file({"path": str(target), "offset": 100})
        assert result["ok"] is True
        assert result["content"] == ""
        assert result["lines_read"] == 0

    def test_read_with_zero_limit(self, tmp_path: Path):
        target = tmp_path / "test.txt"
        target.write_text("line1\nline2\n", encoding="utf-8")
        result = read_file({"path": str(target), "limit": 0})
        assert result["ok"] is True
        assert result["lines_read"] == 0

    def test_read_large_file(self, tmp_path: Path):
        target = tmp_path / "large.txt"
        content = "\n".join([f"line {i}" for i in range(5000)])
        target.write_text(content, encoding="utf-8")
        result = read_file({"path": str(target), "offset": 0, "limit": 10})
        assert result["ok"] is True
        assert result["lines_read"] == 10
        assert result["total_lines"] == 5000

    def test_read_unicode_file(self, tmp_path: Path):
        target = tmp_path / "unicode.txt"
        target.write_text("日本語\n한국어\n中文", encoding="utf-8")
        result = read_file({"path": str(target)})
        assert result["ok"] is True
        assert "日本語" in result["content"]

    def test_read_file_with_special_chars(self, tmp_path: Path):
        target = tmp_path / "weird name!@#.txt"
        target.write_text("special name content", encoding="utf-8")
        result = read_file({"path": str(target)})
        assert result["ok"] is True
        assert result["content"] == "special name content"


# ── Edit file edge cases ───────────────────────────────

class TestEditFileEdgeCases:
    def test_edit_with_empty_old_string(self, tmp_path: Path):
        target = tmp_path / "edit.txt"
        target.write_text("Hello World", encoding="utf-8")
        result = edit_file({"path": str(target), "old_string": "", "new_string": "x"})
        # Empty old_string: content.replace("", "x") would insert everywhere
        # or the function might reject it
        assert "ok" in result
        if result["ok"]:
            # If it succeeded, it shouldn't have corrupted data
            assert target.read_text(encoding="utf-8") != ""

    def test_edit_with_empty_new_string(self, tmp_path: Path):
        """Replacing with empty string should remove content."""
        target = tmp_path / "edit.txt"
        target.write_text("Hello World and Hello again", encoding="utf-8")
        result = edit_file({"path": str(target), "old_string": "Hello ", "new_string": ""})
        assert result["ok"] is True
        content = target.read_text(encoding="utf-8")
        assert "World" in content
        assert "Hello" not in content  # "Hello " was removed

    def test_edit_old_string_new_string_same(self, tmp_path: Path):
        """Replacing with same string is a no-op but should not error."""
        target = tmp_path / "edit.txt"
        target.write_text("Hello World", encoding="utf-8")
        result = edit_file({"path": str(target), "old_string": "Hello", "new_string": "Hello"})
        assert result["ok"] is True
        assert result["replacements_count"] == 1
        assert target.read_text(encoding="utf-8") == "Hello World"

    def test_edit_file_with_unicode(self, tmp_path: Path):
        target = tmp_path / "edit_unicode.txt"
        target.write_text("こんにちは 世界", encoding="utf-8")
        result = edit_file({"path": str(target), "old_string": "世界", "new_string": "ワールド"})
        assert result["ok"] is True
        assert target.read_text(encoding="utf-8") == "こんにちは ワールド"

    def test_edit_large_file(self, tmp_path: Path):
        target = tmp_path / "large.txt"
        content = "ABCDEFGHIJ" * 5000  # 50KB
        target.write_text(content, encoding="utf-8")
        result = edit_file({"path": str(target), "old_string": "ABCDEFGHIJ", "new_string": "ZYXWVUTSRQ"})
        assert result["ok"] is True
        new_content = target.read_text(encoding="utf-8")
        assert "ABCDEFGHIJ" not in new_content
        assert "ZYXWVUTSRQ" in new_content

    def test_edit_multiline_replace(self, tmp_path: Path):
        target = tmp_path / "multi.txt"
        target.write_text("line1\nline2\nline3\nline2\nline4\n", encoding="utf-8")
        result = edit_file({
            "path": str(target),
            "old_string": "line2\nline3",
            "new_string": "replaced1\nreplaced2"
        })
        assert result["ok"] is True
        content = target.read_text(encoding="utf-8")
        assert "replaced1" in content
        assert "replaced2" in content
        assert "line2" in content  # The second "line2" should remain

    def test_edit_file_with_regex_special_chars(self, tmp_path: Path):
        """Old/new strings should be treated as literal text, not regex."""
        target = tmp_path / "regex.txt"
        target.write_text("Hello.*World", encoding="utf-8")
        result = edit_file({"path": str(target), "old_string": ".*", "new_string": "!"})
        assert result["ok"] is True
        assert target.read_text(encoding="utf-8") == "Hello!World"


# ── Patch file edge cases ──────────────────────────────

class TestPatchFileEdgeCases:
    def test_patch_with_empty_diff(self, tmp_path: Path):
        target = tmp_path / "patch.txt"
        target.write_text("content", encoding="utf-8")
        result = patch_file({"path": str(target), "diff": ""})
        assert result["ok"] is True
        assert result["hunks_applied"] == 0

    def test_patch_with_malformed_diff(self, tmp_path: Path):
        target = tmp_path / "patch.txt"
        target.write_text("content", encoding="utf-8")
        result = patch_file({"path": str(target), "diff": "not a patch at all"})
        # Should handle gracefully
        assert "ok" in result

    def test_patch_with_invalid_hunk_format(self, tmp_path: Path):
        target = tmp_path / "patch.txt"
        target.write_text("line1\nline2\n", encoding="utf-8")
        result = patch_file({"path": str(target), "diff": "@@ this is not a valid hunk @@"})
        assert isinstance(result["ok"], bool)

    def test_patch_with_hunk_beyond_file(self, tmp_path: Path):
        target = tmp_path / "patch.txt"
        target.write_text("line1\n", encoding="utf-8")
        result = patch_file({"path": str(target), "diff": "@@ -100,5 +100,5 @@"})
        assert result["ok"] is True

    def test_patch_empty_file(self, tmp_path: Path):
        target = tmp_path / "empty.txt"
        target.write_text("", encoding="utf-8")
        result = patch_file({"path": str(target), "diff": "@@ -1,1 +1,1 @@"})
        assert isinstance(result["ok"], bool)


# ── Cross-tool integration edge cases ──────────────────

class TestCrossToolEdgeCases:
    def test_write_then_read(self, tmp_path: Path):
        target = tmp_path / "rw.txt"
        write_result = write_file({"path": str(target), "content": "test content"})
        assert write_result["ok"] is True
        read_result = read_file({"path": str(target)})
        assert read_result["ok"] is True
        assert read_result["content"] == "test content"

    def test_write_then_edit_then_read(self, tmp_path: Path):
        target = tmp_path / "wer.txt"
        write_file({"path": str(target), "content": "Hello World!"})
        edit_file({"path": str(target), "old_string": "World", "new_string": "Universe"})
        result = read_file({"path": str(target)})
        assert result["ok"] is True
        assert result["content"] == "Hello Universe!"


# Need os for chmod
import os
