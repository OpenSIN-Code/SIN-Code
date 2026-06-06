"""
Purpose: Publish an MCP server to PyPI (Python) or npm (Node).
Docs: publisher.doc.md
"""

from __future__ import annotations

import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any

# ── Constants ──────────────────────────────────────
PYPI_DEFAULT_REPO = "https://upload.pypi.org/legacy/"
PYPI_TEST_REPO = "https://test.pypi.org/legacy/"
NPM_DEFAULT_REGISTRY = "https://registry.npmjs.org/"


@dataclass
class PublishResult:
    """Result of a publish attempt."""

    ok: bool
    package: str
    version: str
    target: str  # "pypi", "pypi-test", "npm"
    output: str = ""
    error: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {
            "ok": self.ok,
            "package": self.package,
            "version": self.version,
            "target": self.target,
            "output": self.output,
            "error": self.error,
        }


@dataclass
class Publisher:
    """
    Build + publish an MCP server to a registry.

    - Python (template=python-fastmcp): runs `python -m build` and `twine upload`.
    - Node (template=node-mcp): runs `npm publish`.
    - Go (template=go-mcp): no central registry — emits instructions only.

    The publisher never holds credentials; it expects `TWINE_USERNAME` /
    `TWINE_PASSWORD` (or a `.pypirc`) for PyPI and `NPM_TOKEN` for npm.
    """

    dry_run: bool = False
    timeout: int = 300  # seconds for subprocess calls

    # ── Python (PyPI) ──────────────────────────────────────

    def publish_pypi(
        self,
        project_dir: Path,
        test: bool = False,
        skip_build: bool = False,
    ) -> PublishResult:
        """Build + upload a Python project to PyPI (or TestPyPI)."""
        target = "pypi-test" if test else "pypi"
        repo_url = PYPI_TEST_REPO if test else PYPI_DEFAULT_REPO
        name, version = self._read_pyproject(project_dir)
        if not skip_build and not self.dry_run:
            build = self._run(project_dir, ["python", "-m", "build"])
            if not build["ok"]:
                return PublishResult(
                    ok=False,
                    package=name,
                    version=version,
                    target=target,
                    output=build["stdout"],
                    error=build["stderr"],
                )
        if self.dry_run:
            return PublishResult(
                ok=True,
                package=name,
                version=version,
                target=target,
                output="dry-run: would twine upload --repository " + repo_url,
            )
        cmd = ["twine", "upload", "--repository-url", repo_url, "dist/*"]
        result = self._run(project_dir, cmd)
        return PublishResult(
            ok=result["ok"],
            package=name,
            version=version,
            target=target,
            output=result["stdout"] + result["stderr"],
            error="" if result["ok"] else result["stderr"],
        )

    # ── Node (npm) ──────────────────────────────────────

    def publish_npm(
        self,
        project_dir: Path,
        registry: str = NPM_DEFAULT_REGISTRY,
        tag: str = "latest",
    ) -> PublishResult:
        """Run `npm publish` for a Node project."""
        name, version = self._read_package_json(project_dir)
        if self.dry_run:
            return PublishResult(
                ok=True,
                package=name,
                version=version,
                target="npm",
                output=f"dry-run: would npm publish --registry {registry} --tag {tag}",
            )
        cmd = ["npm", "publish", "--registry", registry, "--tag", tag]
        result = self._run(project_dir, cmd)
        return PublishResult(
            ok=result["ok"],
            package=name,
            version=version,
            target="npm",
            output=result["stdout"] + result["stderr"],
            error="" if result["ok"] else result["stderr"],
        )

    # ── Go (no registry) ──────────────────────────────────────

    def publish_go(self, project_dir: Path) -> PublishResult:
        """Go modules don't have a central MCP-server registry.

        We return an `ok=False` with guidance to push a git tag instead.
        """
        name, version = "go-module", "v0.0.0"
        go_mod = project_dir / "go.mod"
        if go_mod.is_file():
            first = go_mod.read_text(encoding="utf-8").splitlines()
            for line in first:
                if line.startswith("module "):
                    name = line.split()[1].strip()
                    break
        return PublishResult(
            ok=False,
            package=name,
            version=version,
            target="go",
            output=(
                "Go modules do not have a central MCP-server registry. "
                "Tag the repo (`git tag vX.Y.Z && git push --tags`) and "
                "run `go list -m github.com/owner/repo@vX.Y.Z` to consume it."
            ),
            error="no registry",
        )

    # ── Dispatcher ──────────────────────────────────────

    def publish(self, project_dir: Path, template: str, **kwargs: Any) -> PublishResult:
        """Dispatch to the right publisher based on the template key."""
        if template == "python-fastmcp":
            test = bool(kwargs.get("test", False))
            return self.publish_pypi(project_dir, test=test)
        if template == "node-mcp":
            return self.publish_npm(project_dir)
        if template == "go-mcp":
            return self.publish_go(project_dir)
        return PublishResult(
            ok=False,
            package="",
            version="",
            target=template,
            output="",
            error=f"Unknown template: {template!r}",
        )

    # ── Helpers ──────────────────────────────────────

    def _read_pyproject(self, project_dir: Path) -> tuple[str, str]:
        """Extract (name, version) from `pyproject.toml` (no tomllib dep)."""
        path = project_dir / "pyproject.toml"
        if not path.is_file():
            return ("unknown", "0.0.0")
        name, version = "unknown", "0.0.0"
        in_project = False
        for raw in path.read_text(encoding="utf-8").splitlines():
            stripped = raw.strip()
            if stripped == "[project]":
                in_project = True
                continue
            if stripped.startswith("[") and in_project:
                in_project = False
            if in_project and "=" in stripped:
                key, _, val = stripped.partition("=")
                key = key.strip()
                val = val.strip().strip('"').strip("'")
                if key == "name":
                    name = val
                elif key == "version":
                    version = val
        return name, version

    def _read_package_json(self, project_dir: Path) -> tuple[str, str]:
        """Extract (name, version) from `package.json`."""
        import json

        path = project_dir / "package.json"
        if not path.is_file():
            return ("unknown", "0.0.0")
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            return ("unknown", "0.0.0")
        return (data.get("name", "unknown"), data.get("version", "0.0.0"))

    def _run(self, cwd: Path, cmd: list[str]) -> dict[str, Any]:
        """Run a subprocess and capture stdout/stderr."""
        if shutil.which(cmd[0]) is None:
            return {"ok": False, "stdout": "", "stderr": f"command not found: {cmd[0]}"}
        try:
            proc = subprocess.run(
                cmd,
                cwd=str(cwd),
                capture_output=True,
                text=True,
                timeout=self.timeout,
                check=False,
            )
            return {
                "ok": proc.returncode == 0,
                "stdout": proc.stdout,
                "stderr": proc.stderr,
            }
        except subprocess.TimeoutExpired:
            return {
                "ok": False,
                "stdout": "",
                "stderr": f"timeout after {self.timeout}s",
            }
