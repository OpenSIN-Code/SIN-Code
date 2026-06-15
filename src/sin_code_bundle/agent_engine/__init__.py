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

from .builtin_tools import register_builtin_tools
from .delegate import (
    AdaptiveBudgetAllocator,
    DelegationCache,
    DelegationContext,
    DelegationSupervisor,
    make_delegate_tool,
    validate_result,
)
from .distiller import (
    KnowledgeDistiller,
    StandingRule,
    _heuristic_rule,
)
from .distiller import (
    _signature as _rule_signature,
)
from .executor import Executor
from .loop import AgentLoop
from .memory_bridge import MemoryBridge
from .planner import Planner
from .router import CircuitOpenError, ToolRouter
from .telemetry import Telemetry
from .tracing import Span, SpanEmitter, TraceAssembler, TraceContext
from .types import (
    AgentTask,
    Plan,
    Step,
    StepResult,
    StepState,
    Verdict,
    VerdictKind,
)
from .verifier import Verifier

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
    "TraceContext",
    "SpanEmitter",
    "Span",
    "TraceAssembler",
    "KnowledgeDistiller",
    "StandingRule",
    "DelegationContext",
    "AdaptiveBudgetAllocator",
    "DelegationCache",
    "DelegationSupervisor",
    "make_delegate_tool",
    "validate_result",
    "_heuristic_rule",
    "_rule_signature",
]

__version__ = "1.0.0"
