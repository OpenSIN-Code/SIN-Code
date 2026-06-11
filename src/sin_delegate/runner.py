# SPDX-License-Identifier: MIT
"""Pluggable sub-agent runner backends with secret redaction."""

from __future__ import annotations

import asyncio
import json
import os
import re
from dataclasses import dataclass
from typing import Any, Awaitable, Callable, Protocol

from .models import Task

_SECRET_PATTERNS = [
    re.compile(r"(?i)(api[_-]?key|token|secret|password|authorization)"
               r"\s*[:=]\s*\S+"),
    re.compile(r"\b(sk|pk|ghp|gho|pypi|xox[bap])-[A-Za-z0-9_\-]{10,}\b"),
    re.compile(r"\beyJ[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]+\b"),
]


def redact(text: str) -> str:
    for pat in _SECRET_PATTERNS:
        text = pat.sub("[REDACTED]", text)
    return text


@dataclass
class RunnerResult:
    ok: bool
    output: str
    exit_code: int


class Runner(Protocol):
    async def run(self, task: Task, cwd: str,
                  timeout: float) -> RunnerResult: ...


def _prompt(task: Task) -> str:
    parts = [
        "You are a focused sub-agent. Complete EXACTLY this task, nothing more.",
        f"# Task: {task.title}",
        task.instructions,
    ]
    if task.files_hint:
        parts.append("Focus on these paths: " + ", ".join(task.files_hint))
    if task.agent.system_hint:
        parts.append(task.agent.system_hint)
    parts.append(
        "Rules: stay inside the working directory; do not push; do not "
        "switch branches; commit nothing (the orchestrator commits); keep "
        "changes minimal."
    )
    return "\n\n".join(parts)


class SubprocessRunner:
    def __init__(self, argv_factory: Callable[[Task], list[str]]) -> None:
        self._argv_factory = argv_factory

    async def run(self, task: Task, cwd: str,
                  timeout: float) -> RunnerResult:
        argv = self._argv_factory(task)
        env = {**os.environ, **task.agent.env,
               "SIN_DELEGATE_TASK": task.id}
        proc = await asyncio.create_subprocess_exec(
            *argv, cwd=cwd, env=env,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
            stdin=asyncio.subprocess.DEVNULL,
        )
        try:
            out, _ = await asyncio.wait_for(
                proc.communicate(), timeout=timeout)
        except asyncio.TimeoutError:
            proc.kill()
            await proc.wait()
            return RunnerResult(False,
                               f"[timeout after {timeout:.0f}s]", -1)
        text = redact(out.decode(errors="replace"))[-100_000:]
        return RunnerResult(proc.returncode == 0, text,
                           proc.returncode or 0)


def _opencode_argv(task: Task) -> list[str]:
    argv = ["opencode", "run", "--print-logs"]
    if task.agent.model:
        argv += ["--model", task.agent.model]
    return argv + [_prompt(task)]


def _claude_argv(task: Task) -> list[str]:
    argv = ["claude", "-p", _prompt(task), "--output-format", "text",
            "--permission-mode", "acceptEdits"]
    if task.agent.model:
        argv += ["--model", task.agent.model]
    return argv


def _codex_argv(task: Task) -> list[str]:
    argv = ["codex", "exec", "--full-auto"]
    if task.agent.model:
        argv += ["--model", task.agent.model]
    return argv + [_prompt(task)]


def _command_argv(task: Task) -> list[str]:
    if not task.agent.command:
        raise ValueError("backend 'command' requires AgentSpec.command")
    return [a.replace("{prompt}", _prompt(task))
            for a in task.agent.command]


_BACKENDS = {
    "opencode": _opencode_argv,
    "claude": _claude_argv,
    "codex": _codex_argv,
    "command": _command_argv,
}


def runner_for(spec) -> Runner:
    if spec.backend == "echo":
        return EchoRunner()
    try:
        return SubprocessRunner(_BACKENDS[spec.backend])
    except KeyError:
        raise ValueError(
            f"unknown backend {spec.backend!r}; "
            f"choose one of {sorted(_BACKENDS)}") from None


class EchoRunner:
    """Dry-run backend: prints what WOULD happen. Used by --dry-run and tests."""

    async def run(self, task: Task, cwd: str,
                  timeout: float) -> RunnerResult:
        return RunnerResult(
            True,
            json.dumps(
                {"dry_run": True, "task": task.title, "cwd": cwd},
                indent=2),
            0,
        )
