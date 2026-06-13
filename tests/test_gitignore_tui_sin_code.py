"""
test_gitignore_tui_sin_code.py — regression for issue #61.

Asserts that:
  1. .gitignore contains a rule for `cmd/sin-code/tui/.sin-code/`.
  2. No file under that directory is currently tracked by git
     (i.e. the untracked runtime DBs stay untracked, not committed).
  3. The pre-existing rule for `cmd/sin-code/internal/.sin-code/` is
     NOT removed by the fix.

Run with:  pytest tests/test_gitignore_tui_sin_code.py -q
Exit code: 0 on pass, 1 on any failure.
"""
from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path

import pytest

if shutil.which("git") is None:
    pytest.skip("git not on PATH; skipping issue-61 regression test", allow_module_level=True)

REPO_ROOT = Path(__file__).resolve().parent.parent
GITIGNORE = REPO_ROOT / ".gitignore"
TUI_SIN_CODE = "cmd/sin-code/tui/.sin-code/"
INTERNAL_SIN_CODE = "cmd/sin-code/internal/.sin-code/"


def _git(*args: str) -> str:
    """Run a git command in the repo root, return stdout (stripped)."""
    out = subprocess.run(
        ["git", "-C", str(REPO_ROOT), *args],
        capture_output=True, text=True, check=False, timeout=10,
    )
    return out.stdout.strip()


def test_tui_sin_code_rule_present() -> None:
    """AC-1: .gitignore must contain a rule that matches the TUI runtime dir."""
    text = GITIGNORE.read_text(encoding="utf-8")
    pattern = rf"^\s*{re.escape(TUI_SIN_CODE.rstrip('/'))}/?\s*(?:#.*)?$"
    matches = [ln for ln in text.splitlines() if re.match(pattern, ln)]
    assert matches, (
        f"Expected a .gitignore rule for `{TUI_SIN_CODE}`, "
        f"found none. See issue #61."
    )


def test_tui_sin_code_is_ignored() -> None:
    """AC-2: `git check-ignore` must return exit 0 for the runtime dir."""
    rc = subprocess.run(
        ["git", "-C", str(REPO_ROOT), "check-ignore",
         f"{TUI_SIN_CODE.rstrip('/')}/lessons.db"],
        capture_output=True, text=True, check=False, timeout=10,
    )
    assert rc.returncode == 0, (
        f"`git check-ignore` did not ignore the TUI runtime DB. "
        f"stdout={rc.stdout!r} stderr={rc.stderr!r}"
    )


def test_no_tracked_file_under_tui_sin_code() -> None:
    """AC-6: nothing inside cmd/sin-code/tui/.sin-code/ may be tracked."""
    out = _git("ls-files", TUI_SIN_CODE.rstrip("/"))
    assert out == "", (
        f"Found tracked files under {TUI_SIN_CODE}: {out!r}. "
        f"A `git rm --cached` migration would be needed."
    )


def test_internal_sin_code_rule_still_present() -> None:
    """AC-3: the pre-existing rule for internal/.sin-code/ must survive."""
    text = GITIGNORE.read_text(encoding="utf-8")
    pattern = rf"^\s*{re.escape(INTERNAL_SIN_CODE.rstrip('/'))}/?\s*(?:#.*)?$"
    matches = [ln for ln in text.splitlines() if re.match(pattern, ln)]
    assert matches, (
        f"Regression: the pre-existing rule for `{INTERNAL_SIN_CODE}` "
        f"was removed by issue #61's fix."
    )


def test_gitignore_companion_doc_exists() -> None:
    """AC-9: CoDocs companion must exist (and .gitignore must reference it)."""
    companion = REPO_ROOT / ".gitignore.doc.md"
    assert companion.exists(), (
        f"Missing CoDocs companion: {companion}. See AGENTS.md §3."
    )
    first_line = GITIGNORE.read_text(encoding="utf-8").splitlines()[0]
    assert "Docs:" in first_line and ".gitignore.doc.md" in first_line, (
        f"`.gitignore` first line must be `# Docs: .gitignore.doc.md`, "
        f"got: {first_line!r}"
    )
