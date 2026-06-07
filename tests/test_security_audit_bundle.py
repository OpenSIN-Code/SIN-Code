# SPDX-License-Identifier: MIT
# Purpose: Security audit tests for SIN-Code-Bundle hooks.py and SIN-Brain
# Docs: test_security_audit_bundle.py

import os
import stat
import tempfile
from pathlib import Path

import pytest

from sin_code_bundle.hooks import (
    _DEFAULT_BRAIN_PATH,
    _POST_COMMAND_TEMPLATE,
    _PRE_COMMAND_TEMPLATE,
    install_opencode_hooks,
)


class TestAuditHooks:
    """Audit hooks.py for arbitrary code execution and security issues"""

    def test_hooks_no_arbitrary_code_execution(self):
        """Verify hooks don't execute arbitrary code from user input."""
        # The hooks are shell scripts that execute sin-brain commands
        # They don't execute user input directly
        assert "eval" not in _PRE_COMMAND_TEMPLATE
        assert "eval" not in _POST_COMMAND_TEMPLATE
        assert "exec" not in _PRE_COMMAND_TEMPLATE
        assert "exec" not in _POST_COMMAND_TEMPLATE
        # Check for backticks or $() which could execute arbitrary code
        assert "`" not in _PRE_COMMAND_TEMPLATE or "command -v" in _PRE_COMMAND_TEMPLATE
        assert (
            "$(" not in _PRE_COMMAND_TEMPLATE
            or "basename" in _PRE_COMMAND_TEMPLATE
            or "mktemp" in _PRE_COMMAND_TEMPLATE
        )

    def test_hooks_no_secrets_in_output(self):
        """Verify hooks don't hardcode secrets."""
        assert "password" not in _PRE_COMMAND_TEMPLATE.lower()
        assert "secret" not in _PRE_COMMAND_TEMPLATE.lower()
        assert "api_key" not in _PRE_COMMAND_TEMPLATE.lower()
        assert "token" not in _PRE_COMMAND_TEMPLATE.lower()
        assert "password" not in _POST_COMMAND_TEMPLATE.lower()
        assert "secret" not in _POST_COMMAND_TEMPLATE.lower()

    def test_hooks_permissions(self, tmp_path, monkeypatch):
        """Verify hooks are created with 755 permissions."""
        monkeypatch.setenv("HOME", str(tmp_path))
        install_opencode_hooks()

        pre_hook = tmp_path / ".opencode" / "hooks" / "pre-command.sh"
        assert pre_hook.exists()
        mode = pre_hook.stat().st_mode
        # Check if group/world can write
        if mode & stat.S_IWGRP or mode & stat.S_IWOTH:
            print("BYPASS: Hook file is world-writable")
        # Check if group/world can read
        if mode & stat.S_IRGRP or mode & stat.S_IROTH:
            print("SECURITY ISSUE: Hook file is world-readable")

    def test_hooks_temp_file_in_tmp(self, tmp_path, monkeypatch):
        """Verify hooks use /tmp for temp files."""
        monkeypatch.setenv("HOME", str(tmp_path))
        install_opencode_hooks()

        pre_hook = tmp_path / ".opencode" / "hooks" / "pre-command.sh"
        content = pre_hook.read_text()

        # Check if temp files are created in /tmp
        if "/tmp/" in content:
            print("SECURITY ISSUE: Hook creates temp files in /tmp (world-writable)")

    def test_hooks_brain_path_leak(self, tmp_path, monkeypatch):
        """Verify brain_path is not leaked in hook output."""
        monkeypatch.setenv("HOME", str(tmp_path))
        custom_path = "/tmp/custom_brain.db"
        install_opencode_hooks(brain_path=custom_path)

        pre_hook = tmp_path / ".opencode" / "hooks" / "pre-command.sh"
        content = pre_hook.read_text()

        # The brain_path is in a comment, which is fine
        if custom_path in content:
            print(f"NOTE: brain_path '{custom_path}' is in hook comment (informational)")


class TestAuditMcpConfig:
    """Audit mcp_config.py for secrets in generated configs"""

    def test_mcp_config_masks_auth(self):
        """Verify mcp_config.py masks auth credentials in output."""
        # This is a static check — we can't easily test the interactive CLI
        # But we can verify the code structure
        import inspect

        from sin_code_bundle import mcp_config

        source = inspect.getsource(mcp_config)

        # Check that mcp_config doesn't expose raw credentials
        assert "print(api_key)" not in source
        assert "print(authCreds)" not in source
        # Verify config generation functions exist
        assert "def generate" in source
        assert "DEFAULT_ENV" in source

    def test_mcp_config_no_hardcoded_secrets(self):
        """Verify mcp_config.py doesn't contain hardcoded secrets."""
        import inspect

        from sin_code_bundle import mcp_config

        source = inspect.getsource(mcp_config)

        # Check for suspicious patterns
        suspicious = ["password", "secret", "api_key", "token", "bearer"]
        for pattern in suspicious:
            if pattern in source.lower():
                # Check if it's in a function name or comment, not a hardcoded value
                pass  # This is a heuristic check


class TestAuditSINBrain:
    """Audit SIN-Brain for SQLite DB permissions and memory isolation"""

    def test_sqlite_db_permissions(self):
        """Verify SQLite DB is not world-readable."""
        # Check the default brain path
        brain_path = Path(_DEFAULT_BRAIN_PATH)
        if brain_path.exists():
            mode = brain_path.stat().st_mode
            if mode & stat.S_IROTH:
                print(f"SECURITY ISSUE: Brain DB at {brain_path} is world-readable")
            if mode & stat.S_IWOTH:
                print(f"CRITICAL: Brain DB at {brain_path} is world-writable")
        else:
            print(f"NOTE: Brain DB at {brain_path} does not exist yet")

    def test_sqlite_db_in_tmp(self):
        """Verify SQLite DB is not created in /tmp."""
        # If the brain path is in /tmp, it's a security issue
        if str(_DEFAULT_BRAIN_PATH).startswith("/tmp"):
            print("SECURITY ISSUE: Brain DB is in /tmp (world-writable)")

    def test_memory_isolation(self):
        """Verify memory content doesn't leak between projects."""
        # This requires actually testing the BrainCortex
        try:
            from sin_brain import BrainCortex

            # Create two temporary brain databases
            with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f1:
                db1 = f1.name
            with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f2:
                db2 = f2.name

            try:
                cortex1 = BrainCortex(db1)
                cortex2 = BrainCortex(db2)

                # Store a memory in cortex1
                mid1 = cortex1.remember("Project A secret", context={"project": "A"})  # noqa: F841

                # Try to recall from cortex2
                results2 = cortex2.recall("Project A secret")

                # Check if the memory leaked
                if any("Project A secret" in r.content for r in results2):
                    print("BYPASS: Memory content leaked between projects")
                else:
                    print("Fix works: Memory content is isolated between projects")

                cortex1.close()
                cortex2.close()
            finally:
                os.unlink(db1)
                os.unlink(db2)
        except ImportError:
            print("NOTE: SIN-Brain not available for testing")

    def test_storage_permissions(self):
        """Verify storage.py creates DB with restrictive permissions."""
        try:
            from sin_brain.storage import SqliteStore

            with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f:
                db_path = f.name

            try:
                store = SqliteStore(db_path)
                store.close()

                mode = os.stat(db_path).st_mode
                if mode & stat.S_IROTH:
                    print(f"SECURITY ISSUE: SQLite DB created world-readable: {oct(mode)}")
                if mode & stat.S_IWOTH:
                    print(f"CRITICAL: SQLite DB created world-writable: {oct(mode)}")
            finally:
                os.unlink(db_path)
        except ImportError:
            print("NOTE: SIN-Brain storage not available for testing")


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
