"""SIN-Code Verification Oracle.

An *independent* verification layer for AI coding agents. It answers one
question that agents systematically get wrong: "Did it actually work?" —
without trusting the agent's own self-report.

Three ground-truth signal sources, combined into a single Verdict:
  1. Diagnostics  - compiler / type-checker / linter (the cheapest, strongest signal)
  2. Execution    - build, test, and HTTP probes run in reality
  3. Trace-diff   - behavioral diff of real output before vs. after a change

Plus an Eval-Harness to measure whether any of this actually improves results.
"""
from __future__ import annotations

__version__ = "0.1.0"

from .oracle import VerificationOracle, Verdict, SignalResult, Confidence
from .diagnostics import DiagnosticsOracle, Diagnostic
from .execution import ExecutionOracle, ExecutionResult
from .trace_diff import TraceDiffer, BehaviorTrace, TraceDelta
from .eval_harness import EvalHarness, EvalTask, EvalReport

__all__ = [
    "VerificationOracle",
    "Verdict",
    "SignalResult",
    "Confidence",
    "DiagnosticsOracle",
    "Diagnostic",
    "ExecutionOracle",
    "ExecutionResult",
    "TraceDiffer",
    "BehaviorTrace",
    "TraceDelta",
    "EvalHarness",
    "EvalTask",
    "EvalReport",
]
