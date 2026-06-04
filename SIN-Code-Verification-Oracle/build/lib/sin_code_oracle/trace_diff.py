"""Behavioral Trace-Diff.

AST diffs tell you *what changed in the source*. They cannot tell you *what
changed in behavior*. This module captures the real observable output of a
command (stdout, exit code, emitted artifacts, and optional structured events)
as a "behavior trace", then diffs two traces.

Typical use: capture a trace on the base revision, let the agent edit, capture
again, and diff. A green test suite that silently changed an API response is
caught here even when line-diffs and type-checks are clean.

Determinism helpers: the trace normalizes common sources of noise (timestamps,
temp paths, memory addresses, uuids) so diffs reflect semantics, not entropy.
"""
from __future__ import annotations

import hashlib
import json
import re
import subprocess
import time
from dataclasses import dataclass, field
from pathlib import Path


# Patterns that introduce non-determinism and would create false-positive diffs.
_NOISE_PATTERNS = [
    (re.compile(r"\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?\b"), "<TIMESTAMP>"),
    (re.compile(r"0x[0-9a-fA-F]+"), "<ADDR>"),
    (re.compile(r"\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b"), "<UUID>"),
    (re.compile(r"/tmp/[^\s\"']+"), "<TMP>"),
    (re.compile(r"\b\d+\.\d+s\b"), "<DURATION>"),
]


def _normalize(text: str) -> str:
    for pattern, repl in _NOISE_PATTERNS:
        text = pattern.sub(repl, text)
    return text.strip()


@dataclass
class BehaviorTrace:
    command: str
    exit_code: int
    stdout_normalized: str
    events: list[dict] = field(default_factory=list)  # optional structured events
    artifacts: dict[str, str] = field(default_factory=dict)  # path -> content hash
    raw_stdout: str = ""

    @property
    def fingerprint(self) -> str:
        payload = json.dumps(
            {
                "exit": self.exit_code,
                "out": self.stdout_normalized,
                "events": self.events,
                "artifacts": self.artifacts,
            },
            sort_keys=True,
        )
        return hashlib.sha256(payload.encode()).hexdigest()[:16]

    def as_dict(self) -> dict:
        return {
            "command": self.command,
            "exit_code": self.exit_code,
            "fingerprint": self.fingerprint,
            "events": self.events,
            "artifacts": self.artifacts,
        }


@dataclass
class TraceDelta:
    changed: bool
    exit_code_changed: bool
    stdout_changed: bool
    artifact_changes: dict[str, str]  # path -> added|removed|modified
    event_changes: list[dict]
    summary: str

    def as_dict(self) -> dict:
        return self.__dict__


class TraceDiffer:
    def __init__(self, root: str = ".", artifact_globs: list[str] | None = None):
        self.root = str(Path(root).resolve())
        # Files whose content we hash to detect emitted-output changes.
        self.artifact_globs = artifact_globs or []

    def capture(
        self,
        command: str,
        events_file: str | None = None,
        timeout: int = 120,
    ) -> BehaviorTrace:
        """Run `command` and snapshot its observable behavior.

        events_file: optional path the program writes JSON lines to; each line
        is parsed as a structured event and included in the trace (lets you
        capture domain-level behavior, not just stdout).
        """
        try:
            proc = subprocess.run(
                command, shell=True, cwd=self.root,
                capture_output=True, text=True, timeout=timeout, check=False,
            )
            exit_code, raw = proc.returncode, proc.stdout
        except subprocess.TimeoutExpired:
            exit_code, raw = 124, ""

        events: list[dict] = []
        if events_file:
            ep = Path(self.root) / events_file
            if ep.exists():
                for line in ep.read_text().splitlines():
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        events.append(json.loads(line))
                    except json.JSONDecodeError:
                        events.append({"raw": _normalize(line)})

        artifacts: dict[str, str] = {}
        for glob in self.artifact_globs:
            for p in Path(self.root).glob(glob):
                if p.is_file():
                    rel = str(p.relative_to(self.root))
                    artifacts[rel] = hashlib.sha256(p.read_bytes()).hexdigest()[:16]

        return BehaviorTrace(
            command=command,
            exit_code=exit_code,
            stdout_normalized=_normalize(raw),
            events=events,
            artifacts=artifacts,
            raw_stdout=raw,
        )

    def diff(self, before: BehaviorTrace, after: BehaviorTrace) -> TraceDelta:
        exit_changed = before.exit_code != after.exit_code
        stdout_changed = before.stdout_normalized != after.stdout_normalized

        artifact_changes: dict[str, str] = {}
        before_keys, after_keys = set(before.artifacts), set(after.artifacts)
        for k in after_keys - before_keys:
            artifact_changes[k] = "added"
        for k in before_keys - after_keys:
            artifact_changes[k] = "removed"
        for k in before_keys & after_keys:
            if before.artifacts[k] != after.artifacts[k]:
                artifact_changes[k] = "modified"

        event_changes = self._diff_events(before.events, after.events)

        changed = bool(exit_changed or stdout_changed or artifact_changes or event_changes)
        parts = []
        if exit_changed:
            parts.append(f"exit {before.exit_code}->{after.exit_code}")
        if stdout_changed:
            parts.append("stdout differs")
        if artifact_changes:
            parts.append(f"{len(artifact_changes)} artifact(s) changed")
        if event_changes:
            parts.append(f"{len(event_changes)} event delta(s)")
        summary = "no behavioral change" if not changed else "; ".join(parts)

        return TraceDelta(
            changed=changed,
            exit_code_changed=exit_changed,
            stdout_changed=stdout_changed,
            artifact_changes=artifact_changes,
            event_changes=event_changes,
            summary=summary,
        )

    @staticmethod
    def _diff_events(before: list[dict], after: list[dict]) -> list[dict]:
        """Multiset diff of structured events keyed by their JSON content."""
        def key(e: dict) -> str:
            return json.dumps(e, sort_keys=True)

        from collections import Counter

        cb, ca = Counter(map(key, before)), Counter(map(key, after))
        changes = []
        for k in ca - cb:
            changes.append({"type": "emitted", "count": (ca - cb)[k], "event": json.loads(k)})
        for k in cb - ca:
            changes.append({"type": "missing", "count": (cb - ca)[k], "event": json.loads(k)})
        return changes
