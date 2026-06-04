"""Verification Oracle core.

Combines the independent signal sources into a single, trustworthy Verdict
about whether a change "actually worked". The key design principle: the
agent's self-report is NOT an input. Only ground-truth signals are.

Signal precedence (strongest first):
  1. Execution failures (build/test red)   -> hard FAIL
  2. Diagnostics errors (type/compile)      -> hard FAIL
  3. Unintended behavioral change           -> FAIL unless explicitly expected
  4. Diagnostics warnings                    -> lowers confidence, not a fail
  5. Everything green                        -> PASS

Confidence reflects how much ground truth we actually had. If no tools were
available and nothing ran, we return UNVERIFIED with low confidence rather
than a false PASS — refusing to rubber-stamp is itself a feature.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum

from .diagnostics import DiagnosticsOracle, DiagnosticsReport
from .execution import ExecutionOracle, ExecutionResult
from .trace_diff import TraceDelta, TraceDiffer


class Confidence(str, Enum):
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"


@dataclass
class SignalResult:
    name: str
    passed: bool | None  # None = signal unavailable / not run
    weight: float
    detail: dict = field(default_factory=dict)


@dataclass
class Verdict:
    passed: bool
    confidence: Confidence
    reasons: list[str]
    signals: list[SignalResult] = field(default_factory=list)
    verified: bool = True  # False when we had no ground truth to judge on

    def as_dict(self) -> dict:
        return {
            "passed": self.passed,
            "verified": self.verified,
            "confidence": self.confidence.value,
            "reasons": self.reasons,
            "signals": [
                {"name": s.name, "passed": s.passed, "weight": s.weight, "detail": s.detail}
                for s in self.signals
            ],
        }


class VerificationOracle:
    """Top-level facade orchestrating the three ground-truth oracles."""

    def __init__(self, root: str = ".", env: dict | None = None):
        self.root = root
        self.diagnostics = DiagnosticsOracle()
        self.execution = ExecutionOracle(root=root, env=env)
        self.tracer = TraceDiffer(root=root)

    def verify(
        self,
        changed_files: list[str] | None = None,
        test_command: str | None = None,
        build_command: str | None = None,
        run_diagnostics: bool = True,
        expected_behavior_change: bool = False,
        trace_command: str | None = None,
        trace_before_ref: dict | None = None,
    ) -> Verdict:
        signals: list[SignalResult] = []
        reasons: list[str] = []
        had_ground_truth = False

        # --- 1. Diagnostics (type/compile/lint) ---
        if run_diagnostics:
            report: DiagnosticsReport = self.diagnostics.check(self.root, changed_files)
            if report.available_tools:
                had_ground_truth = True
                passed = report.error_count == 0
                signals.append(
                    SignalResult(
                        name="diagnostics",
                        passed=passed,
                        weight=3.0,
                        detail={
                            "errors": report.error_count,
                            "warnings": report.warning_count,
                            "tools": report.available_tools,
                        },
                    )
                )
                if not passed:
                    reasons.append(
                        f"{report.error_count} diagnostic error(s) from {report.available_tools}"
                    )
                elif report.warning_count:
                    reasons.append(f"{report.warning_count} warning(s) (non-blocking)")
            else:
                signals.append(
                    SignalResult(
                        name="diagnostics",
                        passed=None,
                        weight=3.0,
                        detail={"missing_tools": report.missing_tools},
                    )
                )

        # --- 2. Build ---
        if build_command:
            res: ExecutionResult = self.execution.run_command(build_command)
            had_ground_truth = True
            signals.append(
                SignalResult(
                    name="build",
                    passed=res.success,
                    weight=4.0,
                    detail={"exit_code": res.exit_code, "duration_s": res.duration_s},
                )
            )
            if not res.success:
                reasons.append(f"build failed (exit {res.exit_code})")

        # --- 3. Tests ---
        if test_command:
            res = (
                self.execution.run_pytest(test_command)
                if test_command == "pytest"
                else self.execution.run_command(test_command)
            )
            had_ground_truth = True
            signals.append(
                SignalResult(
                    name="tests",
                    passed=res.success,
                    weight=5.0,
                    detail={"exit_code": res.exit_code, "metrics": res.metrics},
                )
            )
            if not res.success:
                reasons.append(f"tests failed: {res.metrics or res.exit_code}")

        # --- 4. Behavioral trace diff ---
        if trace_command and trace_before_ref is not None:
            after = self.tracer.capture(trace_command)
            delta: TraceDelta = TraceDelta(
                changed=trace_before_ref.get("fingerprint") != after.fingerprint,
                exit_code_changed=trace_before_ref.get("exit_code") != after.exit_code,
                stdout_changed=True,
                artifact_changes={},
                event_changes=[],
                summary="fingerprint mismatch"
                if trace_before_ref.get("fingerprint") != after.fingerprint
                else "stable",
            )
            had_ground_truth = True
            # An unexpected behavioral change is a failure; an expected one is fine.
            behavior_ok = (not delta.changed) or expected_behavior_change
            signals.append(
                SignalResult(
                    name="behavior",
                    passed=behavior_ok,
                    weight=2.0,
                    detail={
                        "changed": delta.changed,
                        "summary": delta.summary,
                        "expected": expected_behavior_change,
                    },
                )
            )
            if delta.changed and not expected_behavior_change:
                reasons.append(f"unexpected behavioral change: {delta.summary}")

        return self._aggregate(signals, reasons, had_ground_truth)

    @staticmethod
    def _aggregate(
        signals: list[SignalResult], reasons: list[str], had_ground_truth: bool
    ) -> Verdict:
        if not had_ground_truth:
            return Verdict(
                passed=False,
                verified=False,
                confidence=Confidence.LOW,
                reasons=["no ground-truth signal available — refusing to confirm success"],
                signals=signals,
            )

        ran = [s for s in signals if s.passed is not None]
        failed = [s for s in ran if s.passed is False]
        passed = len(failed) == 0

        # Confidence scales with how much weight of ground truth we gathered.
        total_weight = sum(s.weight for s in ran)
        if total_weight >= 8:
            conf = Confidence.HIGH
        elif total_weight >= 4:
            conf = Confidence.MEDIUM
        else:
            conf = Confidence.LOW

        if passed and not reasons:
            reasons = ["all available ground-truth signals passed"]

        return Verdict(
            passed=passed, verified=True, confidence=conf, reasons=reasons, signals=signals
        )
