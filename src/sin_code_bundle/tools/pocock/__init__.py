# SPDX-License-Identifier: MIT
"""Pocock Workflow Tools for SIN Code Bundle.

This module provides tools for the Pocock development paradigm:
- Socratic alignment (grill-me)
- TDD enforcement (tdd-enforcer)
- DAG-based task orchestration (dag-kanban)
- Multi-agent coordination (teammate-adapter)
- Runtime stability utilities (zod-patch, safe-start, cleanup-hook)
"""

from .grill_me import GrillMe, run_grill_me
from .tdd_enforcer import TDDEnforcer, run_tdd_enforcer
from .dag_kanban import DAGKanban, run_dag_kanban

__all__ = [
    "GrillMe",
    "run_grill_me",
    "TDDEnforcer",
    "run_tdd_enforcer",
    "DAGKanban",
    "run_dag_kanban",
]
