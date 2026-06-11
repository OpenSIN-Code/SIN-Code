# SPDX-License-Identifier: MIT
"""Multi-repo support: contracts, plan parsing, two-phase merger."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from pathlib import Path

from .ledger import Ledger
from .models import Plan, Task
from .worktree import GitError, Worktree, _git

_CONTRACT_RE = re.compile(
    r"<sin-contract>\s*(\{.*?\})\s*</sin-contract>", re.DOTALL)

CONTRACT_INSTRUCTIONS = """\
Wenn dieser Task Schnittstellen definiert, die andere Repos brauchen
(API-Routen, Typen, Event-Namen, Env-Vars), exportiere sie am ENDE deiner
Ausgabe exakt so:
<sin-contract>{"endpoints": [...], "types": [...], "notes": "..."}</sin-contract>
Nur Fakten, die du tatsächlich implementiert hast."""


def extract_contract(agent_output: str) -> dict | None:
    """Last <sin-contract> block wins (agent can self-correct)."""
    matches = _CONTRACT_RE.findall(agent_output)
    for raw in reversed(matches):
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            continue
    return None


class ContractStore:
    """Contracts live in the ledger (kind='contract') — resume-safe."""

    def __init__(self, ledger: Ledger, plan_id: str) -> None:
        self.ledger = ledger
        self.plan_id = plan_id

    def publish(self, task_id: str, contract: dict) -> None:
        self.ledger.emit(self.plan_id, task_id, "contract", contract)

    def collect(self, dep_ids) -> dict:
        out: dict = {}
        for ev in self.ledger.history(self.plan_id):
            if ev["kind"] == "contract" and ev["task_id"] in dep_ids:
                out[ev["task_id"]] = ev["payload"]  # last wins
        return out

    def render(self, dep_ids, titles: dict) -> str:
        contracts = self.collect(dep_ids)
        if not contracts:
            return ""
        parts = ["Verträge (Contracts) deiner Upstream-Tasks — das sind "
                 "implementierte FAKTEN, halte dich exakt daran:"]
        for tid, c in contracts.items():
            parts.append(f"## {titles.get(tid, tid)}\n"
                         + json.dumps(c, indent=2))
        return "\n\n".join(parts)


@dataclass(frozen=True)
class RepoRef:
    name: str
    path: str
    base_branch: str = "main"


@dataclass
class MultiRepoPlan:
    goal: str
    repos: dict  # name -> RepoRef
    plan: Plan
    task_repo: dict = field(default_factory=dict)  # task_id -> repo name

    @property
    def id(self) -> str:
        return self.plan.id

    def validate(self) -> None:
        self.plan.validate()
        for tid, rname in self.task_repo.items():
            if rname not in self.repos:
                raise ValueError(
                    f"task {tid} references unknown repo {rname!r}")
        for repo in self.repos.values():
            if not (Path(repo.path) / ".git").exists():
                raise GitError(
                    f"{repo.path} is not a git repository")


def multirepo_plan_from_dict(data: dict) -> MultiRepoPlan:
    """Plan format: {goal, repos: {name: {path, base_branch?}},
    tasks: [{key, repo, title, instructions, deps?, ...}]}"""
    repos = {
        name: RepoRef(name=name,
                      path=str(Path(cfg["path"]).resolve()),
                      base_branch=cfg.get("base_branch", "main"))
        for name, cfg in data["repos"].items()
    }
    if not repos:
        raise ValueError("multi-repo plan needs at least one repo")
    default_repo = next(iter(repos))

    # Build tasks directly here (do NOT delegate to plan_from_dict — its
    # two-pass finalize() includes deps in the hash, so pre-resolved ids
    # from a separate pass wouldn't match the final hash).
    from .planfile import _task_from as _build_task
    title_to_id: dict = {}
    tasks: list = []
    key_repo: dict = {}
    # First pass: build with deps=() to learn the final-id for each key
    for rt in data["tasks"]:
        key = rt.get("key") or rt["title"]
        title_to_id[key] = _build_task(rt, deps=()).finalize().id
    # Second pass: build with REAL resolved deps, then pin the id
    for rt in data["tasks"]:
        key = rt.get("key") or rt["title"]
        rname = rt.get("repo", default_repo)
        if rname not in repos:
            raise ValueError(
                f"task {key!r}: unknown repo {rname!r}")
        dep_ids = tuple(title_to_id.get(d, d)
                        for d in rt.get("deps", []))
        task = _build_task(rt, deps=dep_ids).finalize()
        # Pin the pre-resolved id (deps don't change the content-hash
        # for these tasks, since title+instructions+backend are the
        # primary input and deps are resolved after finalization).
        if task.id != title_to_id[key]:
            task = Task(**{**{f: getattr(task, f)
                                for f in task.__dataclass_fields__},
                            "id": title_to_id[key]})
        tasks.append(task)
        key_repo[task.id] = rname

    plan = Plan(
        goal=data["goal"],
        tasks=tuple(tasks),
        repo=str(repos[default_repo].path),
        base_branch="main",
    )
    plan.validate()
    mrp = MultiRepoPlan(goal=data["goal"], repos=repos, plan=plan,
                        task_repo=key_repo)
    mrp.validate()
    return mrp


@dataclass
class MergeUnit:
    task_id: str
    worktree: Worktree
    repo_name: str


class TwoPhaseMerger:
    """Coordinates the global commit point across all repos.

    commit(): merges ALL units in deterministic order (repos
    topologically by task deps, within a repo by DAG order). Before
    the first merge, a snapshot tag is set per repo. If ANY merge
    fails, ALL already-merged repos are reset to their snapshot ->
    all-or-nothing.
    """

    def __init__(self, repos: dict, ledger: Ledger, plan_id: str) -> None:
        self.repos = repos
        self.ledger = ledger
        self.plan_id = plan_id
        self.units: list = []

    def stage(self, unit: MergeUnit) -> None:
        self.units.append(unit)
        self.ledger.emit(self.plan_id, unit.task_id, "merge:staged",
                         {"repo": unit.repo_name,
                          "branch": unit.worktree.branch})

    def commit(self, order: list) -> None:
        rank = {tid: i for i, tid in enumerate(order)}
        units = sorted(self.units,
                       key=lambda u: rank.get(u.task_id, 1 << 30))

        snapshots: dict = {}
        for name, ref in self.repos.items():
            tag = f"sin-global-snap/{self.plan_id}"
            _git(ref.path, "tag", "-f", tag, ref.base_branch)
            snapshots[name] = tag
        self.ledger.emit(self.plan_id, "*", "merge:phase2_begin",
                         {"units": len(units),
                          "snapshots": list(snapshots)})

        merged: list = []
        try:
            for unit in units:
                unit.worktree.merge_back()
                merged.append(unit)
                self.ledger.emit(self.plan_id, unit.task_id, "merged",
                                 {"repo": unit.repo_name, "phase": 2})
        except GitError as e:
            rolled: set = set()
            for unit in merged:
                ref = self.repos[unit.repo_name]
                if unit.repo_name not in rolled:
                    _git(ref.path, "reset", "--hard",
                         snapshots[unit.repo_name], check=False)
                    rolled.add(unit.repo_name)
            self.ledger.emit(
                self.plan_id, "*", "merge:phase2_rollback",
                {"failed_unit": (units[len(merged)].task_id
                                 if len(merged) < len(units) else "?"),
                 "rolled_back_repos": sorted(rolled),
                 "error": str(e)})
            raise GitError(
                f"two-phase commit aborted, all repos restored: {e}"
            ) from e
        self.ledger.emit(self.plan_id, "*", "merge:phase2_done",
                         {"merged": len(merged)})
