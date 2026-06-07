# SPDX-License-Identifier: MIT
"""Tests for SIN Developer CLI commands (lint, docs, git).

Docs: test_developer_cli.doc.md
"""

import os
import subprocess
import tempfile
from pathlib import Path

import pytest


def _run(args, timeout=30):
    """Run a sin command and return the result."""
    # Use `sin` entry point instead of `python -m` because `cli.py` has an
    # early `if __name__ == "__main__"` block that prevents commands defined
    # after line 1751 from being registered when running via `python -m`.
    if args[0] == "python":
        # Strip leading ["python", "-m", "sin_code_bundle.cli"]
        args = ["sin"] + args[3:]
    return subprocess.run(args, capture_output=True, text=True, timeout=timeout)


# ── lint tests ──────────────────────────────────────

class TestLintCommands:

    def test_lint_help(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "lint", "--help"])
        assert r.returncode == 0
        assert "run" in r.stdout
        assert "check" in r.stdout

    def test_lint_check_no_linters(self):
        """lint check should report no linters if none are installed."""
        # This is a smoke test — it will pass regardless of whether linters are installed
        r = _run(["python", "-m", "sin_code_bundle.cli", "lint", "check", "."])
        # Return code may be 0 or non-zero depending on linter availability
        assert "SIN-BUNDLE" in r.stdout or "SIN-BUNDLE" in r.stderr

    def test_lint_run_auto_no_linters(self):
        """lint run with auto should fail gracefully if no linters are found."""
        with tempfile.TemporaryDirectory() as tmpdir:
            r = _run(["python", "-m", "sin_code_bundle.cli", "lint", "run", tmpdir])
            # Should either run a linter or fail gracefully
            assert r.returncode in (0, 1)

    def test_lint_run_unknown_tool(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "lint", "run", ".", "--tool", "nonexistent"])
        assert r.returncode == 1
        assert "Unknown linter" in r.stderr or "Unknown linter" in r.stdout


# ── docs tests ──────────────────────────────────────

class TestDocsCommands:

    def test_docs_help(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "--help"])
        assert r.returncode == 0
        assert "generate" in r.stdout
        assert "check" in r.stdout

    def test_docs_generate_python_project(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create a fake Python project
            pyproject = Path(tmpdir) / "pyproject.toml"
            pyproject.write_text('[project]\nname = "test-proj"\nversion = "1.0.0"\ndescription = "A test project"\n')

            r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "generate", tmpdir, "--output", "README.md"])
            assert r.returncode == 0

            readme = Path(tmpdir) / "README.md"
            assert readme.exists()
            content = readme.read_text()
            assert "test-proj" in content
            assert "A test project" in content
            assert "Python" in content

    def test_docs_generate_js_project(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            package_json = Path(tmpdir) / "package.json"
            package_json.write_text('{"name": "test-js", "version": "2.0.0", "description": "JS test", "dependencies": {"lodash": "^4.17.0"}}')

            r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "generate", tmpdir, "--output", "README.md"])
            assert r.returncode == 0

            readme = Path(tmpdir) / "README.md"
            assert readme.exists()
            content = readme.read_text()
            assert "test-js" in content
            assert "JS test" in content
            assert "JavaScript/TypeScript" in content

    def test_docs_generate_go_project(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            go_mod = Path(tmpdir) / "go.mod"
            go_mod.write_text("module github.com/example/test-go\n\ngo 1.21\n")

            r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "generate", tmpdir, "--output", "README.md"])
            assert r.returncode == 0

            readme = Path(tmpdir) / "README.md"
            assert readme.exists()
            content = readme.read_text()
            assert "test-go" in content or "github.com/example/test-go" in content
            assert "Go" in content

    def test_docs_generate_nonexistent_path(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "generate", "/nonexistent/path"])
        assert r.returncode == 1
        assert "not found" in r.stderr.lower() or "not found" in r.stdout.lower()

    def test_docs_check(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create a Python file with docstring
            py_file = Path(tmpdir) / "test.py"
            py_file.write_text('"""Module docstring."""\ndef foo():\n    """Function docstring."""\n    pass\n')

            r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "check", tmpdir])
            assert r.returncode == 0
            assert "Documentation Coverage Report" in r.stdout
            assert "Python files: 1" in r.stdout
            assert "Files with docstrings: 1/1 (100%)" in r.stdout

    def test_docs_check_no_readme(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            py_file = Path(tmpdir) / "test.py"
            py_file.write_text('"""Module docstring."""\n')

            r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "check", tmpdir])
            assert r.returncode == 0
            assert "Missing README.md" in r.stdout

    def test_docs_check_nonexistent_path(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "check", "/nonexistent/path"])
        assert r.returncode == 1


# ── git tests ──────────────────────────────────────

class TestGitCommands:

    def test_git_help(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "git", "--help"])
        assert r.returncode == 0
        assert "status" in r.stdout
        assert "commit" in r.stdout
        assert "clean" in r.stdout
        assert "log" in r.stdout

    def test_git_status_non_git(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            r = _run(["python", "-m", "sin_code_bundle.cli", "git", "status", tmpdir])
            assert r.returncode == 1
            assert "Not a git repository" in r.stderr or "Not a git repository" in r.stdout

    def test_git_status_clean(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            # Initialize git repo
            subprocess.run(["git", "init"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.name", "Test"], cwd=tmpdir, capture_output=True)
            # Create and commit a file
            test_file = Path(tmpdir) / "test.txt"
            test_file.write_text("hello")
            subprocess.run(["git", "add", "."], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "commit", "-m", "initial"], cwd=tmpdir, capture_output=True)

            r = _run(["python", "-m", "sin_code_bundle.cli", "git", "status", tmpdir])
            assert r.returncode == 0
            assert "Working tree clean" in r.stdout or "clean" in r.stdout.lower()

    def test_git_status_with_changes(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            subprocess.run(["git", "init"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.name", "Test"], cwd=tmpdir, capture_output=True)
            test_file = Path(tmpdir) / "test.txt"
            test_file.write_text("hello")
            subprocess.run(["git", "add", "."], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "commit", "-m", "initial"], cwd=tmpdir, capture_output=True)
            # Modify file
            test_file.write_text("hello world")

            r = _run(["python", "-m", "sin_code_bundle.cli", "git", "status", tmpdir])
            assert r.returncode == 0
            assert "changed file" in r.stdout or "1" in r.stdout

    def test_git_commit(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            subprocess.run(["git", "init"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.name", "Test"], cwd=tmpdir, capture_output=True)
            test_file = Path(tmpdir) / "test.txt"
            test_file.write_text("hello")

            r = _run(["python", "-m", "sin_code_bundle.cli", "git", "commit", "test commit", "--path", tmpdir, "--all"])
            assert r.returncode == 0
            assert "Committed" in r.stdout

    def test_git_log(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            subprocess.run(["git", "init"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.name", "Test"], cwd=tmpdir, capture_output=True)
            test_file = Path(tmpdir) / "test.txt"
            test_file.write_text("hello")
            subprocess.run(["git", "add", "."], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "commit", "-m", "initial"], cwd=tmpdir, capture_output=True)

            r = _run(["python", "-m", "sin_code_bundle.cli", "git", "log", tmpdir, "-n", "5"])
            assert r.returncode == 0
            assert "initial" in r.stdout

    def test_git_clean_dry_run(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            subprocess.run(["git", "init"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "config", "user.name", "Test"], cwd=tmpdir, capture_output=True)
            test_file = Path(tmpdir) / "test.txt"
            test_file.write_text("hello")
            subprocess.run(["git", "add", "."], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "commit", "-m", "initial"], cwd=tmpdir, capture_output=True)
            # Create a branch and merge it
            subprocess.run(["git", "checkout", "-b", "feature"], cwd=tmpdir, capture_output=True)
            test_file.write_text("hello feature")
            subprocess.run(["git", "add", "."], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "commit", "-m", "feature"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "checkout", "main"], cwd=tmpdir, capture_output=True)
            subprocess.run(["git", "merge", "feature"], cwd=tmpdir, capture_output=True)

            r = _run(["python", "-m", "sin_code_bundle.cli", "git", "clean", tmpdir])
            assert r.returncode == 0
            # Dry run should show branch but not delete it
            assert "feature" in r.stdout
            assert "Dry-run" in r.stdout

            # Verify branch still exists
            branches = subprocess.run(["git", "branch"], cwd=tmpdir, capture_output=True, text=True)
            assert "feature" in branches.stdout


# ── Integration smoke tests for all developer CLI commands ──────────────────────────────────────

class TestDeveloperCLIIntegration:

    def test_all_developer_cli_commands_exist(self):
        """Verify all developer CLI subcommands are registered."""
        r = _run(["python", "-m", "sin_code_bundle.cli", "--help"])
        assert r.returncode == 0
        assert "lint" in r.stdout
        assert "docs" in r.stdout
        assert "git" in r.stdout
        assert "bench" in r.stdout
        assert "mcp-server" in r.stdout

    def test_lint_subcommands_exist(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "lint", "--help"])
        assert r.returncode == 0
        assert "run" in r.stdout
        assert "check" in r.stdout

    def test_docs_subcommands_exist(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "docs", "--help"])
        assert r.returncode == 0
        assert "generate" in r.stdout
        assert "check" in r.stdout

    def test_git_subcommands_exist(self):
        r = _run(["python", "-m", "sin_code_bundle.cli", "git", "--help"])
        assert r.returncode == 0
        assert "status" in r.stdout
        assert "commit" in r.stdout
        assert "clean" in r.stdout
        assert "log" in r.stdout
