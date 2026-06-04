"""Tests for the hooks module (automatic .opencode hook installation).

Purpose: Verify that hooks.py correctly installs, lists, and uninstalls
pre-command and post-command shell hooks into ~/.opencode/hooks/.

Docs: tests/test_hooks.doc.md
"""

from __future__ import annotations

import os
from pathlib import Path

from sin_code_bundle.hooks import (
    _DEFAULT_BRAIN_PATH,
    _POST_COMMAND_TEMPLATE,
    _PRE_COMMAND_TEMPLATE,
    install_opencode_hooks,
    list_opencode_hooks,
    uninstall_opencode_hooks,
)


class TestInstallOpencodeHooks:
    """Test the install_opencode_hooks function."""

    def test_installs_both_hooks_by_default(self, tmp_path, monkeypatch):
        """Both pre-command and post-command hooks should be created."""
        monkeypatch.setenv("HOME", str(tmp_path))

        installed = install_opencode_hooks()

        assert len(installed) == 2
        pre_hook = Path(installed[0])
        post_hook = Path(installed[1])
        assert pre_hook.name == "pre-command.sh"
        assert post_hook.name == "post-command.sh"
        assert pre_hook.exists()
        assert post_hook.exists()
        # Both should be executable
        assert os.access(str(pre_hook), os.X_OK)
        assert os.access(str(post_hook), os.X_OK)

    def test_pre_hook_content(self, tmp_path, monkeypatch):
        """Pre-command hook should contain recall logic and shebang."""
        monkeypatch.setenv("HOME", str(tmp_path))
        install_opencode_hooks(post_command=False)

        pre_hook = tmp_path / ".opencode" / "hooks" / "pre-command.sh"
        content = pre_hook.read_text()
        assert content.startswith("#!/bin/bash")
        assert "sin-brain recall" in content
        assert "SIN_BRAIN_CONTEXT" in content
        assert "command -v sin-brain" in content  # defensive check

    def test_post_hook_content(self, tmp_path, monkeypatch):
        """Post-command hook should contain remember logic and shebang."""
        monkeypatch.setenv("HOME", str(tmp_path))
        install_opencode_hooks(pre_command=False)

        post_hook = tmp_path / ".opencode" / "hooks" / "post-command.sh"
        content = post_hook.read_text()
        assert content.startswith("#!/bin/bash")
        assert "sin-brain remember" in content
        assert "last_task_result.txt" in content
        assert "command -v sin-brain" in content  # defensive check

    def test_brain_path_in_comment(self, tmp_path, monkeypatch):
        """The brain_path should be documented in the hook comment."""
        monkeypatch.setenv("HOME", str(tmp_path))
        custom_path = "/tmp/custom_brain.db"
        install_opencode_hooks(brain_path=custom_path)

        pre_hook = tmp_path / ".opencode" / "hooks" / "pre-command.sh"
        content = pre_hook.read_text()
        assert custom_path in content

    def test_only_pre_hook(self, tmp_path, monkeypatch):
        """When post_command=False, only pre-hook should be installed."""
        monkeypatch.setenv("HOME", str(tmp_path))
        installed = install_opencode_hooks(post_command=False)

        assert len(installed) == 1
        assert "pre-command.sh" in installed[0]
        post_hook = tmp_path / ".opencode" / "hooks" / "post-command.sh"
        assert not post_hook.exists()

    def test_only_post_hook(self, tmp_path, monkeypatch):
        """When pre_command=False, only post-hook should be installed."""
        monkeypatch.setenv("HOME", str(tmp_path))
        installed = install_opencode_hooks(pre_command=False)

        assert len(installed) == 1
        assert "post-command.sh" in installed[0]
        pre_hook = tmp_path / ".opencode" / "hooks" / "pre-command.sh"
        assert not pre_hook.exists()

    def test_no_hooks_when_both_disabled(self, tmp_path, monkeypatch):
        """When both are disabled, nothing should be installed."""
        monkeypatch.setenv("HOME", str(tmp_path))
        installed = install_opencode_hooks(pre_command=False, post_command=False)

        assert installed == []


class TestUninstallOpencodeHooks:
    """Test the uninstall_opencode_hooks function."""

    def test_removes_existing_hooks(self, tmp_path, monkeypatch):
        """Should remove both hooks if they exist."""
        monkeypatch.setenv("HOME", str(tmp_path))
        install_opencode_hooks()

        removed = uninstall_opencode_hooks()

        assert len(removed) == 2
        for path in removed:
            assert not Path(path).exists()

    def test_returns_empty_when_no_hooks(self, tmp_path, monkeypatch):
        """Should return empty list when no hooks exist."""
        monkeypatch.setenv("HOME", str(tmp_path))
        removed = uninstall_opencode_hooks()
        assert removed == []


class TestListOpencodeHooks:
    """Test the list_opencode_hooks function."""

    def test_lists_installed_hooks(self, tmp_path, monkeypatch):
        """Should return paths to existing hooks."""
        monkeypatch.setenv("HOME", str(tmp_path))
        install_opencode_hooks()

        found = list_opencode_hooks()

        assert len(found) == 2
        assert all("pre-command.sh" in p or "post-command.sh" in p for p in found)

    def test_returns_empty_when_none(self, tmp_path, monkeypatch):
        """Should return empty list when no hooks are installed."""
        monkeypatch.setenv("HOME", str(tmp_path))
        found = list_opencode_hooks()
        assert found == []


class TestTemplates:
    """Test the hook templates directly."""

    def test_pre_template_has_defensive_check(self):
        """Pre-hook should not fail if sin-brain is missing."""
        assert "command -v sin-brain &> /dev/null" in _PRE_COMMAND_TEMPLATE

    def test_post_template_has_defensive_check(self):
        """Post-hook should not fail if sin-brain is missing."""
        assert "command -v sin-brain &> /dev/null" in _POST_COMMAND_TEMPLATE

    def test_post_template_cleans_up_result_file(self):
        """Post-hook should remove the temp file after reading."""
        assert "rm -f" in _POST_COMMAND_TEMPLATE
        assert "last_task_result.txt" in _POST_COMMAND_TEMPLATE

    def test_default_brain_path_is_set(self):
        """The default brain path should be a sensible default."""
        assert _DEFAULT_BRAIN_PATH == ".sin/brain.db"
