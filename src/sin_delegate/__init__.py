# SPDX-License-Identifier: MIT
"""sin-code-delegate — verification-gated sub-agent delegation for the SIN-Code stack.

Public API:
    AgentSpec, Budget, Plan, Task, TaskState, Verdict, Risk, RunResult
    Delegator, delegate — programmatic engine
"""

from .models import (
    AgentSpec, Budget, Plan, RunResult, Risk, Task, TaskOutcome,
    TaskState, Verdict,
)
from .engine import Delegator, delegate

__version__ = "0.1.0"
__all__ = [
    "AgentSpec", "Budget", "Delegator", "Plan", "RunResult", "Risk",
    "Task", "TaskOutcome", "TaskState", "Verdict",
    "delegate", "__version__",
]
