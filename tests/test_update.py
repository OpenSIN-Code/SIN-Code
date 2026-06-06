"""Tests for `sin update` — pure unit tests so no real pipx/git/go needed."""

from __future__ import annotations

from pathlib import Path

import pytest

from sin_code_bundle import update as upd


# ── discover_go_tools ────────────────────────────────────────────────────
def test_discover_go_tools_finds_repos(tmp_path: Path) -> None:
    """Two SIN-Code-*-Tool repos with cmd/<name>/main.go are picked up."""
    dev = tmp_path / "dev"
    bin_dir = tmp_path / "bin"
    # Repo A: name matches dir name
    a = dev / "SIN-Code-Foo-Tool"
    (a / "cmd" / "foo").mkdir(parents=True)
    (a / "cmd" / "foo" / "main.go").write_text("package main")
    # Repo B: same pattern
    b = dev / "SIN-Code-Bar-Tool"
    (b / "cmd" / "bar").mkdir(parents=True)
    (b / "cmd" / "bar" / "main.go").write_text("package main")
    # Repo C: missing main.go — must be skipped
    c = dev / "SIN-Code-Broken-Tool"
    (c / "cmd" / "broken").mkdir(parents=True)
    (c / "cmd" / "broken" / "no_go.txt").write_text("nope")

    tools = upd.discover_go_tools(dev_dir=dev, bin_dir=bin_dir)
    names = sorted(t.name for t in tools)
    assert names == ["bar", "foo"]


def test_discover_go_tools_empty_dir(tmp_path: Path) -> None:
    """Returns an empty list when no repos exist."""
    tools = upd.discover_go_tools(dev_dir=tmp_path, bin_dir=tmp_path / "bin")
    assert tools == []


def test_discover_go_tools_missing_dev_dir(tmp_path: Path) -> None:
    """Returns an empty list when ~/dev itself does not exist."""
    tools = upd.discover_go_tools(dev_dir=tmp_path / "nope", bin_dir=tmp_path / "bin")
    assert tools == []


# ── update_go_tool (check mode — the key invariant) ──────────────────────
def test_update_check_does_not_modify(tmp_path: Path) -> None:
    """`--check` mode must NEVER touch the binary or the repo."""
    repo = tmp_path / "SIN-Code-X-Tool"
    cmd = repo / "cmd" / "x"
    cmd.mkdir(parents=True)
    (cmd / "main.go").write_text("package main\nfunc main() {}\n")
    # initialize a git repo on a branch so _git_branch() returns a name
    import subprocess
    subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
    subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
    subprocess.run(
        ["git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init"],
        cwd=repo,
        check=True,
    )
    binary = tmp_path / "bin" / "x"
    binary.parent.mkdir(parents=True)
    binary.write_text("#!/bin/sh\necho 0.0.0\n")  # fake binary with version output
    import os
    os.chmod(binary, 0o755)
    pre_binary_bytes = binary.read_bytes()
    pre_main_bytes = (cmd / "main.go").read_bytes()

    tool = upd.GoTool(name="x", repo=repo, binary=binary)
    result = upd.update_go_tool(tool, check=True)

    # The file is unchanged after a --check run
    assert binary.read_bytes() == pre_binary_bytes
    assert (cmd / "main.go").read_bytes() == pre_main_bytes
    # Result is a planning record, not a real update
    assert result.status in {"would-update", "would-skip"}
    assert result.target == "x"


def test_update_check_skips_detached_head(tmp_path: Path) -> None:
    """A repo with no current branch (detached HEAD) is reported, not pulled."""
    repo = tmp_path / "SIN-Code-Y-Tool"
    (repo / "cmd" / "y").mkdir(parents=True)
    (repo / "cmd" / "y" / "main.go").write_text("package main")
    import subprocess
    subprocess.run(["git", "init", "-q"], cwd=repo, check=True)
    subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
    subprocess.run(
        ["git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init"],
        cwd=repo,
        check=True,
    )
    # explicit checkout into detached HEAD (commit hash, not branch)
    rev = subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=repo, capture_output=True, text=True, check=True
    ).stdout.strip()
    subprocess.run(["git", "checkout", "-q", rev], cwd=repo, check=True)
    assert upd._git_branch(repo) == ""  # detached

    binary = tmp_path / "bin" / "y"
    binary.parent.mkdir(parents=True)
    tool = upd.GoTool(name="y", repo=repo, binary=binary)
    result = upd.update_go_tool(tool, check=True)
    assert result.status == "would-skip"


# ── render_table ─────────────────────────────────────────────────────────
def test_render_table_empty() -> None:
    """Empty results render to the documented sentinel string."""
    assert upd.render_table([]) == "No update targets found."


def test_render_table_populated() -> None:
    """A populated result set renders a table with the expected columns."""
    results = [
        upd.UpdateResult(
            target="sin-code-bundle",
            old_version="1.3.0",
            new_version="1.4.0",
            status="updated",
        ),
        upd.UpdateResult(
            target="discover",
            old_version="v0.2.5",
            new_version="v0.2.6",
            status="updated",
        ),
    ]
    text = upd.render_table(results)
    # Header row
    assert "Target" in text and "Old" in text and "New" in text and "Status" in text
    # Both rows present
    assert "sin-code-bundle" in text
    assert "discover" in text
    # Separator line
    assert "----" in text


# ── update_python (graceful when pipx is missing) ────────────────────────
def test_update_python_handles_missing_pipx(monkeypatch: pytest.MonkeyPatch) -> None:
    """If pipx is not on PATH, the step reports a clean failure (not crash)."""
    monkeypatch.setattr(upd.shutil, "which", lambda _: None)
    result = upd.update_python(check=False)
    assert result.status == "failed"
    assert "pipx" in result.detail


def test_update_python_check_mode_is_pure_plan() -> None:
    """`--check` does not call pipx; result is a plan record."""
    result = upd.update_python(check=True)
    assert result.status == "would-update"
    assert "pipx upgrade" in result.detail
