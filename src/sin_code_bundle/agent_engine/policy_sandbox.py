# SPDX-License-Identifier: MIT
"""Policy Sandbox — declarative Allow/Deny guards per tool."""

from __future__ import annotations

import json
import re
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


class PolicyViolation(PermissionError):
    """Tool call blocked by policy."""


@dataclass(slots=True)
class PolicyRule:
    action: str
    tool: str
    pattern: str
    arg: str | None = None
    reason: str = ""
    _rx: re.Pattern[str] | None = field(default=None, repr=False)

    def __post_init__(self) -> None:
        self._rx = re.compile(self.pattern)

    def matches(self, tool: str, kwargs: dict[str, Any]) -> bool:
        if self.tool not in ("*", tool):
            return False
        if self.arg is not None:
            value = kwargs.get(self.arg)
            return value is not None and bool(self._rx.search(str(value)))
        haystack = json.dumps(kwargs, default=str, ensure_ascii=False)
        return bool(self._rx.search(haystack))


@dataclass
class PolicySandbox:
    rules: list[PolicyRule] = field(default_factory=list)
    default: str = "allow"
    dry_run: bool = False
    audit_path: Path = field(
        default_factory=lambda: Path.home() / ".sin" / "policy-audit.jsonl"
    )

    @classmethod
    def load(cls, repo_root: str | None = None, *,
             dry_run: bool = False) -> "PolicySandbox":
        candidates: list[Path] = []
        if repo_root:
            candidates.append(Path(repo_root) / ".sin" / "policy.json")
        candidates.append(Path.home() / ".sin" / "policy.json")
        for path in candidates:
            if path.exists():
                try:
                    raw = json.loads(path.read_text(encoding="utf-8"))
                except json.JSONDecodeError:
                    continue
                return cls(
                    rules=[PolicyRule(
                        action=r["action"], tool=r.get("tool", "*"),
                        pattern=r["pattern"], arg=r.get("arg"),
                        reason=r.get("reason", ""),
                    ) for r in raw.get("rules", [])],
                    default=raw.get("default", "allow"),
                    dry_run=dry_run,
                )
        return cls(dry_run=dry_run)

    def decide(self, tool: str, kwargs: dict[str, Any]) -> tuple[bool, str]:
        matched_allow: PolicyRule | None = None
        for rule in self.rules:
            if not rule.matches(tool, kwargs):
                continue
            if rule.action == "deny":
                return False, rule.reason or f"deny rule: {rule.pattern}"
            matched_allow = matched_allow or rule
        if matched_allow is not None:
            return True, "explicit allow"
        if self.default == "deny":
            return False, "default deny (no allow rule matched)"
        return True, "default allow"

    def _audit(self, tool: str, kwargs: dict[str, Any],
               allowed: bool, reason: str) -> None:
        self.audit_path.parent.mkdir(parents=True, exist_ok=True)
        with self.audit_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps({
                "ts": round(time.time(), 3),
                "tool": tool,
                "allowed": allowed,
                "dry_run": self.dry_run,
                "reason": reason,
                "args_preview": json.dumps(
                    kwargs, default=str, ensure_ascii=False
                )[:400],
            }, ensure_ascii=False) + "\n")

    def wrap(self, router) -> None:
        original_call = router.call

        async def guarded_call(name: str, **kwargs: Any) -> Any:
            allowed, reason = self.decide(name, kwargs)
            if not allowed:
                self._audit(name, kwargs, allowed=False, reason=reason)
                if not self.dry_run:
                    raise PolicyViolation(
                        f"policy blocked tool {name!r}: {reason}"
                    )
            elif reason != "default allow":
                self._audit(name, kwargs, allowed=True, reason=reason)
            return await original_call(name, **kwargs)

        router.call = guarded_call
