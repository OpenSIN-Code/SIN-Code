"""Diagnostics Oracle.

Instead of re-implementing a weaker AST analyzer, we treat the *existing*
language servers / compilers / linters as ground-truth oracles. This is the
cheapest and strongest correctness signal available, and it is exactly what
is missing from a tree-sitter-only stack.

Each provider:
  - declares which file extensions it covers,
  - detects whether its underlying tool is installed (graceful degradation),
  - shells out and parses the tool's machine-readable output into Diagnostic.

Adding a language = adding a Provider. No core changes required.
"""

from __future__ import annotations

import json
import shutil
import subprocess
from dataclasses import dataclass, field
from pathlib import Path


@dataclass
class Diagnostic:
    file: str
    line: int
    column: int
    severity: str  # "error" | "warning" | "info"
    message: str
    code: str | None = None
    source: str | None = None  # which tool produced it

    def as_dict(self) -> dict:
        return self.__dict__


@dataclass
class DiagnosticsReport:
    available_tools: list[str] = field(default_factory=list)
    missing_tools: list[str] = field(default_factory=list)
    diagnostics: list[Diagnostic] = field(default_factory=list)

    @property
    def error_count(self) -> int:
        return sum(1 for d in self.diagnostics if d.severity == "error")

    @property
    def warning_count(self) -> int:
        return sum(1 for d in self.diagnostics if d.severity == "warning")

    def as_dict(self) -> dict:
        return {
            "available_tools": self.available_tools,
            "missing_tools": self.missing_tools,
            "error_count": self.error_count,
            "warning_count": self.warning_count,
            "diagnostics": [d.as_dict() for d in self.diagnostics],
        }


def _run(cmd: list[str], cwd: str, timeout: int = 120) -> subprocess.CompletedProcess:
    return subprocess.run(
        cmd,
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
    )


class DiagnosticProvider:
    """Base class. Override `tool`, `extensions`, `run`."""

    name: str = "base"
    tool: str = ""  # executable name probed with shutil.which
    extensions: tuple[str, ...] = ()

    def available(self) -> bool:
        return bool(self.tool) and shutil.which(self.tool) is not None

    def covers(self, files: list[str]) -> bool:
        return any(f.endswith(self.extensions) for f in files)

    def run(self, root: str, files: list[str]) -> list[Diagnostic]:  # pragma: no cover
        raise NotImplementedError


class PyrightProvider(DiagnosticProvider):
    """Python type checking via pyright --outputjson (the strongest Python signal)."""

    name = "pyright"
    tool = "pyright"
    extensions = (".py",)

    def run(self, root: str, files: list[str]) -> list[Diagnostic]:
        proc = _run([self.tool, "--outputjson", root], cwd=root)
        out: list[Diagnostic] = []
        try:
            data = json.loads(proc.stdout or "{}")
        except json.JSONDecodeError:
            return out
        sev_map = {"error": "error", "warning": "warning", "information": "info"}
        for d in data.get("generalDiagnostics", []):
            rng = d.get("range", {}).get("start", {})
            out.append(
                Diagnostic(
                    file=d.get("file", ""),
                    line=rng.get("line", 0) + 1,
                    column=rng.get("character", 0) + 1,
                    severity=sev_map.get(d.get("severity", "error"), "error"),
                    message=d.get("message", ""),
                    code=str(d.get("rule")) if d.get("rule") else None,
                    source="pyright",
                )
            )
        return out


class RuffProvider(DiagnosticProvider):
    """Fast Python linting via ruff check --output-format=json."""

    name = "ruff"
    tool = "ruff"
    extensions = (".py",)

    def run(self, root: str, files: list[str]) -> list[Diagnostic]:
        proc = _run([self.tool, "check", "--output-format=json", root], cwd=root)
        out: list[Diagnostic] = []
        try:
            data = json.loads(proc.stdout or "[]")
        except json.JSONDecodeError:
            return out
        for d in data:
            loc = d.get("location", {})
            out.append(
                Diagnostic(
                    file=d.get("filename", ""),
                    line=loc.get("row", 0),
                    column=loc.get("column", 0),
                    severity="warning",
                    message=d.get("message", ""),
                    code=d.get("code"),
                    source="ruff",
                )
            )
        return out


class TscProvider(DiagnosticProvider):
    """TypeScript type checking via tsc --noEmit --pretty false."""

    name = "tsc"
    tool = "tsc"
    extensions = (".ts", ".tsx")

    def run(self, root: str, files: list[str]) -> list[Diagnostic]:
        proc = _run([self.tool, "--noEmit", "--pretty", "false"], cwd=root)
        out: list[Diagnostic] = []
        # Format: path(line,col): error TSxxxx: message
        for line in (proc.stdout + proc.stderr).splitlines():
            if "): error TS" not in line and "): warning TS" not in line:
                continue
            try:
                loc_part, rest = line.split("): ", 1)
                path, pos = loc_part.rsplit("(", 1)
                ln, col = pos.split(",")
                sev, rest2 = rest.split(" ", 1)
                code, message = rest2.split(":", 1)
                out.append(
                    Diagnostic(
                        file=path.strip(),
                        line=int(ln),
                        column=int(col),
                        severity="error" if sev == "error" else "warning",
                        message=message.strip(),
                        code=code.strip(),
                        source="tsc",
                    )
                )
            except (ValueError, IndexError):
                continue
        return out


class DiagnosticsOracle:
    """Runs every applicable, installed provider and aggregates results."""

    def __init__(self, providers: list[DiagnosticProvider] | None = None):
        self.providers = providers or [
            PyrightProvider(),
            RuffProvider(),
            TscProvider(),
        ]

    def check(self, root: str, changed_files: list[str] | None = None) -> DiagnosticsReport:
        root = str(Path(root).resolve())
        files = changed_files or [str(p) for p in Path(root).rglob("*") if p.is_file()]
        report = DiagnosticsReport()
        for provider in self.providers:
            if not provider.covers(files):
                continue
            if not provider.available():
                report.missing_tools.append(provider.tool)
                continue
            report.available_tools.append(provider.name)
            try:
                report.diagnostics.extend(provider.run(root, files))
            except (subprocess.TimeoutExpired, OSError):
                # A provider crashing must never crash the oracle.
                report.missing_tools.append(f"{provider.tool} (errored)")
        return report
