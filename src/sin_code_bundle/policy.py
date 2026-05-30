"""Risk-gating, approval, and tamper-evident audit logging for SIN tools.

MCP has no native access control. This module wraps every tool execution with:
  - a per-tool risk classification (read | write | exec | network)
  - a configurable policy (allow | ask | deny) per risk class
  - an append-only, hash-chained audit log under .sin/audit/log.jsonl
  - path sandboxing helpers so tools cannot read/write outside the project root

Policy is loaded from .sin/policy.yaml (falls back to safe defaults).
"""

from __future__ import annotations

import hashlib
import json
import os
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable, Literal, Optional

try:
    import yaml
except ImportError:  # pragma: no cover
    yaml = None  # type: ignore

RiskClass = Literal["read", "write", "exec", "network"]
Decision = Literal["allow", "ask", "deny"]

TOOL_RISK: dict[str, RiskClass] = {
    "impact": "read",
    "semantic_diff": "read",
    "semantic_review": "read",
    "architectural_debt": "read",
    "prove": "read",
    "verify_tests": "exec",
    "mock_env": "network",
}

DEFAULT_POLICY: dict[RiskClass, Decision] = {
    "read": "allow",
    "write": "ask",
    "exec": "ask",
    "network": "ask",
}


class PolicyError(RuntimeError):
    """Raised when a tool call is denied by policy."""


@dataclass
class Policy:
    rules: dict[RiskClass, Decision] = field(default_factory=lambda: dict(DEFAULT_POLICY))
    auto_approve: bool = field(default_factory=lambda: os.environ.get("SIN_AUTO_APPROVE") == "1")

    @classmethod
    def load(cls, root: Path = Path(".")) -> "Policy":
        path = root / ".sin" / "policy.yaml"
        if path.exists() and yaml is not None:
            data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
            rules = {**DEFAULT_POLICY, **(data.get("rules") or {})}
            return cls(rules=rules, auto_approve=bool(data.get("auto_approve", False)))
        return cls()

    def decide(self, tool: str) -> Decision:
        risk = TOOL_RISK.get(tool, "exec")
        return self.rules.get(risk, "ask")


# --------------------------------------------------------------------------- #
# Tamper-evident audit log (hash chain)
# --------------------------------------------------------------------------- #
class AuditLog:
    def __init__(self, root: Path = Path(".")) -> None:
        self.path = root / ".sin" / "audit" / "log.jsonl"
        self.path.parent.mkdir(parents=True, exist_ok=True)

    def _last_hash(self) -> str:
        if not self.path.exists():
            return "0" * 64
        last = ""
        for line in self.path.read_text(encoding="utf-8").splitlines():
            if line.strip():
                last = line
        if not last:
            return "0" * 64
        return json.loads(last).get("hash", "0" * 64)

    def record(self, tool: str, args: dict, decision: Decision, outcome: str) -> str:
        prev = self._last_hash()
        entry = {
            "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "tool": tool,
            "risk": TOOL_RISK.get(tool, "exec"),
            "decision": decision,
            "outcome": outcome,
            "args_keys": sorted(args.keys()),
            "prev": prev,
        }
        digest = hashlib.sha256(
            (prev + json.dumps(entry, sort_keys=True)).encode("utf-8")
        ).hexdigest()
        entry["hash"] = digest
        with self.path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry) + "\n")
        return digest

    def verify_chain(self) -> bool:
        """Return True if the hash chain is intact (no tampering)."""
        if not self.path.exists():
            return True
        prev = "0" * 64
        for line in self.path.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            entry = json.loads(line)
            stored = entry.pop("hash", "")
            if entry.get("prev") != prev:
                return False
            recomputed = hashlib.sha256(
                (prev + json.dumps(entry, sort_keys=True)).encode("utf-8")
            ).hexdigest()
            if recomputed != stored:
                return False
            prev = stored
        return True


# --------------------------------------------------------------------------- #
# Path sandboxing
# --------------------------------------------------------------------------- #
def ensure_within_root(target: str | Path, root: Optional[str | Path] = None) -> Path:
    """Resolve `target` and guarantee it stays inside the project root."""
    root_path = Path(root or os.environ.get("SIN_PROJECT_ROOT", ".")).resolve()
    resolved = (
        (root_path / target).resolve()
        if not Path(target).is_absolute()  # type: ignore[arg-type]
        else Path(target).resolve()  # type: ignore[arg-type]
    )
    if root_path not in resolved.parents and resolved != root_path:
        raise PolicyError(f"path '{resolved}' is outside project root '{root_path}'")
    return resolved


# --------------------------------------------------------------------------- #
# Gate used by the MCP server to wrap a tool call
# --------------------------------------------------------------------------- #
def guarded(
    tool: str,
    args: dict,
    run: Callable[[], dict],
    root: Path = Path("."),
    approver: Optional[Callable[[str, dict], bool]] = None,
) -> dict:
    """Apply policy + audit around a tool execution.

    `approver` is called for 'ask' decisions; defaults to auto-deny unless
    SIN_AUTO_APPROVE=1 (so non-interactive runs are safe by default).
    """
    policy = Policy.load(root)
    audit = AuditLog(root)
    decision = policy.decide(tool)

    if decision == "deny":
        audit.record(tool, args, decision, "denied")
        raise PolicyError(f"tool '{tool}' denied by policy (risk={TOOL_RISK.get(tool)})")

    if decision == "ask":
        approved = policy.auto_approve or (approver(tool, args) if approver else False)
        if not approved:
            audit.record(tool, args, decision, "rejected")
            raise PolicyError(
                f"tool '{tool}' requires approval (risk={TOOL_RISK.get(tool)}). "
                "Set SIN_AUTO_APPROVE=1 or adjust .sin/policy.yaml."
            )

    try:
        result = run()
        audit.record(tool, args, decision, "ok")
        return result
    except Exception as exc:  # noqa: BLE001
        audit.record(tool, args, decision, f"error:{type(exc).__name__}")
        raise
