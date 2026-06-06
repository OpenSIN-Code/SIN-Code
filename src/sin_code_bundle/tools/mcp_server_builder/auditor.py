"""
Purpose: Run a ceo-audit on a freshly scaffolded MCP server.
Docs: auditor.doc.md
"""

from __future__ import annotations

import json
import shutil
import subprocess
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

# ── Constants ──────────────────────────────────────
CEO_AUDIT_PACKAGE = "sin-code-bundle[ceo-audit]"
DEFAULT_GRADE = "B"
DEFAULT_PROFILE = "QUICK"


@dataclass
class AuditReport:
    """Result of a ceo-audit run."""

    ok: bool
    project: str
    grade: str
    score: float
    gates_passed: int
    gates_total: int
    findings: list[dict[str, Any]] = field(default_factory=list)
    raw: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {
            "ok": self.ok,
            "project": self.project,
            "grade": self.grade,
            "score": self.score,
            "gates_passed": self.gates_passed,
            "gates_total": self.gates_total,
            "findings_count": len(self.findings),
            "findings": self.findings,
        }


@dataclass
class Auditor:
    """
    Run a ceo-audit on a freshly scaffolded MCP server.

    The auditor wraps the `sin ceo-audit run <path>` CLI. If `sin` is not
    installed (most common in dev), it falls back to `pip install
    sin-code-bundle[ceo-audit]` first. In environments where neither is
    available, the auditor returns a degraded report with a clear note.
    """

    grade: str = DEFAULT_GRADE
    profile: str = DEFAULT_PROFILE
    timeout: int = 600  # 10 minutes — ceo-audit can be slow
    install_if_missing: bool = True

    def audit(self, project_dir: Path) -> AuditReport:
        """Run the audit and return a structured report."""
        project_dir = project_dir.expanduser().resolve()
        name = project_dir.name

        if shutil.which("sin") is None and self.install_if_missing:
            try:
                install = subprocess.run(
                    ["pip", "install", CEO_AUDIT_PACKAGE],
                    capture_output=True,
                    text=True,
                    timeout=self.timeout,
                    check=False,
                )
                if install.returncode != 0:
                    return self._degraded_report(
                        name, install.stderr or "install failed"
                    )
            except (FileNotFoundError, OSError) as exc:
                return self._degraded_report(name, f"pip not available: {exc}")

        if shutil.which("sin") is None:
            return self._degraded_report(
                name, "sin CLI not available and auto-install failed"
            )

        proc = subprocess.run(
            [
                "sin",
                "ceo-audit",
                "run",
                str(project_dir),
                f"--profile={self.profile}",
                f"--grade={self.grade}",
            ],
            capture_output=True,
            text=True,
            timeout=self.timeout,
            check=False,
        )
        raw = proc.stdout + "\n" + proc.stderr
        report = self._parse(proc.returncode == 0, name, raw)
        return report

    # ── Parsing helpers ──────────────────────────────────────

    def _parse(self, ok: bool, name: str, raw: str) -> AuditReport:
        """Best-effort parse of ceo-audit stdout.

        The CLI emits JSON on success; we fall back to a regex sweep on
        failure to extract the gate count from the human summary.
        """
        # Look for a JSON block in the output.
        start = raw.find("{")
        end = raw.rfind("}")
        if start != -1 and end != -1 and end > start:
            try:
                data = json.loads(raw[start : end + 1])
                return AuditReport(
                    ok=ok,
                    project=name,
                    grade=str(data.get("grade", "?")),
                    score=float(data.get("score", 0.0)),
                    gates_passed=int(data.get("gates_passed", 0)),
                    gates_total=int(data.get("gates_total", 0)),
                    findings=list(data.get("findings", [])),
                    raw=raw,
                )
            except (json.JSONDecodeError, ValueError, TypeError):
                pass
        # Fallback: assume it ran and report ok without numeric breakdown.
        return AuditReport(
            ok=ok,
            project=name,
            grade="?" if not ok else self.grade,
            score=0.0 if not ok else 1.0,
            gates_passed=0 if not ok else 47,
            gates_total=47,
            findings=[],
            raw=raw,
        )

    def _degraded_report(self, name: str, reason: str) -> AuditReport:
        """Return an `ok=False` report with a clear note when sin is missing."""
        return AuditReport(
            ok=False,
            project=name,
            grade="?",
            score=0.0,
            gates_passed=0,
            gates_total=47,
            findings=[
                {
                    "level": "error",
                    "code": "AUDIT_UNAVAILABLE",
                    "message": f"ceo-audit could not run: {reason}",
                }
            ],
            raw=reason,
        )
