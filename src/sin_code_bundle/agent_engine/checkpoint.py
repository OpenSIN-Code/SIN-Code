# SPDX-License-Identifier: MIT
"""Checkpoint/Resume — crash-safe journal + workspace fingerprint."""

from __future__ import annotations

import json
import os
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .types import Plan, StepState


def _tree_hash(repo_root: str) -> str:
    if not repo_root or not Path(repo_root).exists():
        return ""
    stash = subprocess.run(
        ["git", "-C", repo_root, "stash", "create"],
        capture_output=True, text=True, timeout=60,
    ).stdout.strip()
    if stash:
        return stash
    head = subprocess.run(
        ["git", "-C", repo_root, "rev-parse", "HEAD^{tree}"],
        capture_output=True, text=True, timeout=60,
    ).stdout.strip()
    return head


@dataclass(slots=True)
class ResumeState:
    resumable: bool
    completed_steps: set[str]
    reason: str = ""
    last_checkpoint_ts: float | None = None


class CheckpointStore:
    def __init__(self, task_id: str, repo_root: str,
                 *, base_dir: str | None = None) -> None:
        self.task_id = task_id
        self.repo_root = repo_root
        root = Path(base_dir or os.environ.get("SIN_CHECKPOINT_DIR", "")
                    or Path.home() / ".sin" / "checkpoints")
        root.mkdir(parents=True, exist_ok=True)
        self.path = root / f"{task_id}.jsonl"

    def record_step(self, step_id: str, state: StepState) -> None:
        if state not in (StepState.SUCCEEDED, StepState.FAILED,
                         StepState.SKIPPED):
            return
        record = {
            "ts": round(time.time(), 3),
            "step_id": step_id,
            "state": state.value,
            "tree": _tree_hash(self.repo_root),
        }
        with self.path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(record) + "\n")
            fh.flush()
            os.fsync(fh.fileno())

    def record_run_complete(self, outcome: str) -> None:
        with self.path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps({
                "ts": round(time.time(), 3),
                "run_complete": True,
                "outcome": outcome,
            }) + "\n")
            fh.flush()
            os.fsync(fh.fileno())

    def _read_journal(self) -> list[dict[str, Any]]:
        if not self.path.exists():
            return []
        records: list[dict[str, Any]] = []
        for line in self.path.read_text(encoding="utf-8").splitlines():
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError:
                continue
        return records

    def load_resume_state(self) -> ResumeState:
        records = self._read_journal()
        if not records:
            return ResumeState(resumable=False, completed_steps=set(),
                               reason="no checkpoint journal")
        if any(r.get("run_complete") for r in records):
            return ResumeState(resumable=False, completed_steps=set(),
                               reason="previous run already completed")

        completed = {r["step_id"] for r in records
                     if r.get("state") == StepState.SUCCEEDED.value}
        if not completed:
            return ResumeState(resumable=False, completed_steps=set(),
                               reason="no succeeded steps to resume from")

        last_tree = next(
            (r["tree"] for r in reversed(records) if "tree" in r), None
        )
        current_tree = _tree_hash(self.repo_root)
        if last_tree and last_tree != current_tree:
            return ResumeState(
                resumable=False, completed_steps=set(),
                reason=(
                    "workspace changed since last checkpoint "
                    f"(expected tree {last_tree[:12]}, got "
                    f"{current_tree[:12]}) — refusing blind resume; "
                    "run `git diff` to inspect, or start a fresh run"
                ),
            )
        return ResumeState(
            resumable=True,
            completed_steps=completed,
            last_checkpoint_ts=records[-1].get("ts"),
        )

    @staticmethod
    def apply_to_plan(plan: Plan, resume: ResumeState) -> list[str]:
        skipped: list[str] = []
        for sid in resume.completed_steps:
            step = plan.steps.get(sid)
            if step is not None and step.state is StepState.PENDING:
                step.state = StepState.SUCCEEDED
                skipped.append(sid)
        return skipped

    def discard(self) -> None:
        self.path.unlink(missing_ok=True)
