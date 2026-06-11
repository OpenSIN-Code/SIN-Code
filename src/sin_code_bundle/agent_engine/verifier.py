# SPDX-License-Identifier: MIT
"""Multi-stage verification gate.

Stages run cheapest-first so failures short-circuit early:
1. architecture (optional, ADW rules)
2. semantic diff sanity (deletion cap + secret scan)
3. lint (optional)
4. tests (optional)
"""

from __future__ import annotations

import asyncio
import re
import shlex

from .telemetry import Telemetry
from .types import Verdict, VerdictKind

_SECRET_PATTERNS = [
    re.compile(r"AKIA[0-9A-Z]{16}"),
    re.compile(r"-----BEGIN (?:RSA |EC )?PRIVATE KEY-----"),
    re.compile(r"(?i)(api[_-]?key|secret|token)\s*[:=]\s*['\"][A-Za-z0-9_\-]{20,}['\"]"),
]


class Verifier:
    def __init__(
        self,
        repo_root: str,
        telemetry: Telemetry,
        *,
        lint_cmd: str | None = "ruff check .",
        test_cmd: str | None = "pytest -x -q",
        arch_cmd: str | None = None,
        max_deleted_lines: int = 2000,
        stage_timeout_s: float = 600.0,
    ) -> None:
        self.repo_root = repo_root
        self.telemetry = telemetry
        self.lint_cmd = lint_cmd
        self.test_cmd = test_cmd
        self.arch_cmd = arch_cmd
        self.max_deleted_lines = max_deleted_lines
        self.stage_timeout_s = stage_timeout_s

    async def _run(self, cmd: str) -> tuple[int, str]:
        proc = await asyncio.create_subprocess_exec(
            *shlex.split(cmd),
            cwd=self.repo_root,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
        )
        try:
            out, _ = await asyncio.wait_for(
                proc.communicate(), timeout=self.stage_timeout_s
            )
        except asyncio.TimeoutError:
            proc.kill()
            return 124, f"timeout after {self.stage_timeout_s}s: {cmd}"
        return proc.returncode or 0, out.decode(errors="replace")[-8000:]

    async def verify(self) -> Verdict:
        if self.arch_cmd:
            code, out = await self._run(self.arch_cmd)
            self.telemetry.emit("verify_stage", stage="architecture", code=code)
            if code != 0:
                return Verdict(
                    kind=VerdictKind.FAIL_ARCHITECTURE, detail=out,
                    repair_hint=(
                        "Fix architecture-rule violations reported above "
                        "before re-running. Do not bypass ADW rules."
                    ),
                )

        code, diff = await self._run("git diff --unified=0 HEAD")
        if code == 0 and diff:
            deleted = sum(
                1 for line in diff.splitlines()
                if line.startswith("-") and not line.startswith("---")
            )
            if deleted > self.max_deleted_lines:
                return Verdict(
                    kind=VerdictKind.FAIL_SEMANTIC,
                    detail=(
                        f"{deleted} deleted lines exceeds safety cap "
                        f"({self.max_deleted_lines})."
                    ),
                    repair_hint=(
                        "The change deletes too much code. Split the work "
                        "or restore unintentionally removed code."
                    ),
                )
            for pat in _SECRET_PATTERNS:
                for line in diff.splitlines():
                    if line.startswith("+") and pat.search(line):
                        return Verdict(
                            kind=VerdictKind.FAIL_SEMANTIC,
                            detail="potential secret introduced in diff",
                            repair_hint=(
                                "Remove the hardcoded secret and load it "
                                "from the environment instead."
                            ),
                        )

        if self.lint_cmd:
            code, out = await self._run(self.lint_cmd)
            self.telemetry.emit("verify_stage", stage="lint", code=code)
            if code != 0:
                return Verdict(
                    kind=VerdictKind.FAIL_LINT, detail=out,
                    repair_hint=(
                        "Fix the lint errors above. Prefer minimal, "
                        "targeted fixes over disabling rules."
                    ),
                )

        if self.test_cmd:
            code, out = await self._run(self.test_cmd)
            self.telemetry.emit("verify_stage", stage="tests", code=code)
            if code != 0:
                return Verdict(
                    kind=VerdictKind.FAIL_TESTS, detail=out,
                    repair_hint=(
                        "Make the failing tests pass. Read the assertion "
                        "output above; fix the code under test, never "
                        "weaken the tests."
                    ),
                )

        return Verdict(kind=VerdictKind.PASS)
