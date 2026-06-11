# SPDX-License-Identifier: MIT
"""SIN Agent Engine — autonomous plan/execute/verify/repair loop.

Public API:
    AgentLoop      — top-level orchestrator (plan -> execute -> verify -> repair)
    Planner        — dependency-aware DAG planner with critical-path scheduling
    ToolRouter     — circuit-breaker tool routing with adaptive retry
    Executor       — parallel async executor over isolated git worktrees
    Verifier       — multi-stage verification gate
    Telemetry      — structured JSONL event log
    AgentTask, Plan, Step, StepResult, StepState, Verdict, VerdictKind
"""

from .types import (
    AgentTask,
    Plan,
    Step,
    StepResult,
    StepState,
    Verdict,
    VerdictKind,
)
from .planner import Planner
from .router import ToolRouter, CircuitOpenError
from .executor import Executor
from .verifier import Verifier
from .telemetry import Telemetry
from .memory_bridge import MemoryBridge
from .builtin_tools import register_builtin_tools
from .loop import AgentLoop

__all__ = [
    "AgentTask",
    "Plan",
    "Step",
    "StepResult",
    "StepState",
    "Verdict",
    "VerdictKind",
    "Planner",
    "ToolRouter",
    "CircuitOpenError",
    "Executor",
    "Verifier",
    "Telemetry",
    "MemoryBridge",
    "AgentLoop",
    "register_builtin_tools",
]

__version__ = "1.0.0"
