"""Purpose: Integration tests for the SIN-Code MCP tools.

Docs: test_mcp_integration.doc.md
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from sin_code_bundle.dap_bridge import DAPSession, SINRuntimeTrace  # noqa: E402
from sin_code_bundle.interceptor import SINInterceptor  # noqa: E402
from sin_code_bundle.orchestration_worktrees import SINWorktreeOrchestrator  # noqa: E402


class TestInterceptor:
    """Architectural rule enforcement."""

    def test_preflight_blocks_hardcoded_secrets(self, tmp_path):
        """Hardcoded `API_KEY = '<literal>'` must trip the no_hardcoded_secrets rule and deny the write."""
        interceptor = SINInterceptor(repo_root=tmp_path)
        result = interceptor.preflight("sin_write", {"content": "API_KEY = 'sk-1234567890abcdef'"})
        assert result["allowed"] is False
        assert any(v["rule"] == "no_hardcoded_secrets" for v in result["violations"])

    def test_preflight_allows_env_var_secret(self, tmp_path):
        """Reading the key from os.environ is the safe pattern — must be allowed with zero violations."""
        interceptor = SINInterceptor(repo_root=tmp_path)
        result = interceptor.preflight(
            "sin_write", {"content": "API_KEY = os.environ.get('API_KEY')"}
        )
        assert result["allowed"] is True
        assert result["violations"] == []

    def test_preflight_warns_on_eval(self, tmp_path):
        """`eval $cmd` is a warning-level violation (severity != error) — must be reported, not silently passed."""
        interceptor = SINInterceptor(repo_root=tmp_path)
        result = interceptor.preflight("sin_bash", {"command": "eval $cmd"})
        assert any(v["rule"] == "no_eval_exec" for v in result["violations"])

    def test_preflight_extracts_sin_edit(self, tmp_path):
        """sin_edit must inspect new_content (not old_content) for secret leaks — old is going away, new is the risk."""
        interceptor = SINInterceptor(repo_root=tmp_path)
        result = interceptor.preflight(
            "sin_edit", {"old_content": "x", "new_content": "PASSWORD = 'foo'"}
        )
        assert result["allowed"] is False


class TestDAPBridge:
    """DAP runtime tracing (graceful degradation when debugpy not installed)."""

    def test_dap_session_unsupported_language(self, tmp_path):
        """An unknown language id must return a structured error, not crash the DAP bridge."""
        session = DAPSession("brainfuck", "x.bin", tmp_path)
        result = session.start()
        assert "error" in result
        assert "Unsupported" in result["error"]

    def test_runtime_trace_unknown_function_returns_error(self, tmp_path):
        """Tracing a nonexistent function must succeed-or-error gracefully — never raise (debugpy optional)."""
        tracer = SINRuntimeTrace(repo_root=tmp_path)
        # Should attempt start; debugpy may or may not be installed
        result = tracer.trace_function(
            "nonexistent.py", "nonexistent", "python", store_in_memory=False
        )
        # Either succeeds (debugpy installed) or returns error (missing) — both acceptable
        assert "success" in result or "error" in result

    def test_stop_trace_unknown_session(self, tmp_path):
        """Stopping a session id we never created must report an error, not silently no-op."""
        tracer = SINRuntimeTrace(repo_root=tmp_path)
        result = tracer.stop_trace("never_existed")
        assert "error" in result


class TestWorktreeOrchestrator:
    """Isolated git worktrees (only runs in a git repo)."""

    def test_non_git_repo_returns_error(self, tmp_path):
        """Without .git, create_worktree must refuse (not crash with a stack trace)."""
        orchestrator = SINWorktreeOrchestrator(repo_root=tmp_path)
        result = orchestrator.create_worktree()
        assert "error" in result
        assert "git" in result["error"].lower()

    def test_git_repo_creates_and_cleans_worktree(self, tmp_path):
        """End-to-end: initialise a real git repo, create a worktree on a branch, then clean it up."""
        # Set up a tiny git repo
        repo = tmp_path / "repo"
        repo.mkdir()
        subprocess.run(["git", "init", "-q"], cwd=repo, check=True)
        subprocess.run(["git", "config", "user.email", "t@t"], cwd=repo, check=True)
        subprocess.run(["git", "config", "user.name", "t"], cwd=repo, check=True)
        (repo / "README.md").write_text("init")
        subprocess.run(["git", "add", "."], cwd=repo, check=True)
        subprocess.run(["git", "commit", "-q", "-m", "init"], cwd=repo, check=True)

        orchestrator = SINWorktreeOrchestrator(repo_root=repo)
        create_result = orchestrator.create_worktree("test-branch")
        assert create_result.get("success") is True
        assert Path(create_result["worktree_path"]).exists()

        cleanup_result = orchestrator.cleanup_worktree(create_result["worktree_path"])
        assert cleanup_result.get("success") is True
        assert not Path(create_result["worktree_path"]).exists()
