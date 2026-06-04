# SPDX-License-Identifier: MIT
"""Risk-gating, approval, and tamper-evident audit logging for SIN tools.

MCP has no native access control. This module wraps every tool execution with:
  - a per-tool risk classification (read | write | exec | network)
  - a configurable policy (allow | ask | deny) per risk class
  - an append-only, hash-chained audit log under .sin/audit/log.jsonl
  - path sandboxing helpers so tools cannot read/write outside the project root

Policy is loaded from .sin/policy.yaml (falls back to safe defaults).

Docs: policy.doc.md
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

# ── Tool risk classification ─────────────────────────────────────────
# New MCP tools must be added here so the policy engine can rate them.
TOOL_RISK: dict[str, RiskClass] = {
    "impact": "read",
    "semantic_diff": "read",
    "semantic_review": "read",
    "architectural_debt": "read",
    "prove": "read",
    "verify_tests": "exec",
    "mock_env": "network",
}

# Safe defaults: reads are silent, everything else prompts.
# Never set "exec" or "network" to "allow" without explicit user opt-in.
DEFAULT_POLICY: dict[RiskClass, Decision] = {
    "read": "allow",
    "write": "ask",
    "exec": "ask",
    "network": "ask",
}


class PolicyError(RuntimeError):
    """Raised when a tool call is denied by policy."""


# ── Policy: Rule Container ────────────────────────────────────────────
@dataclass
class Policy:
    """Loaded policy rules + auto-approval flag.

    `auto_approve` is a kill switch for the approval prompt: if True
    (or `SIN_AUTO_APPROVE=1` in the env), all "ask" decisions pass
    without user interaction. Use only in trusted CI.
    """

    rules: dict[RiskClass, Decision] = field(default_factory=lambda: dict(DEFAULT_POLICY))
    auto_approve: bool = field(default_factory=lambda: os.environ.get("SIN_AUTO_APPROVE") == "1")

    @classmethod
    def load(cls, root: Path = Path(".")) -> "Policy":
        """Load policy from `<root>/.sin/policy.yaml`, falling back to defaults.

        Missing file or missing PyYAML → returns a `Policy` populated with
        `DEFAULT_POLICY` and `auto_approve` derived from `SIN_AUTO_APPROVE`.
        User-supplied `rules` are merged on top of defaults (per-key override).
        """
        path = root / ".sin" / "policy.yaml"
        if path.exists() and yaml is not None:
            data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
            rules = {**DEFAULT_POLICY, **(data.get("rules") or {})}
            return cls(rules=rules, auto_approve=bool(data.get("auto_approve", False)))
        return cls()

    def decide(self, tool: str) -> Decision:
        """Map a tool name to its policy decision.

        Unknown tools default to risk class `"exec"` (fail-closed — the
        default decision for `exec` is `"ask"`, so they prompt unless
        `auto_approve` is on).
        """
        risk = TOOL_RISK.get(tool, "exec")
        return self.rules.get(risk, "ask")


# ── Tamper-evident Audit Log (hash chain) ────────────────────────────────
class AuditLog:
    """Append-only JSONL log under `<root>/.sin/audit/log.jsonl`.

    Each entry's `hash` is `sha256(prev_hash || canonical_json(entry))`,
    forming a hash chain. `verify_chain()` re-walks the file to confirm
    no entry has been edited or removed. Argument *values* are never
    logged — only the *keys* (to avoid leaking secrets via the audit log).
    """

    def __init__(self, root: Path = Path(".")) -> None:
        self.path = root / ".sin" / "audit" / "log.jsonl"
        # parents=True — creates .sin/ and .sin/audit/ in one shot
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
        """Append one entry to the log and return its hash.

        `args` is inspected by *key* only (sorted) — values are not stored,
        so secrets in tool args never reach disk.
        """
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
        # sort_keys=True — canonical form so verify_chain() can reproduce
        # the exact same digest bit-for-bit regardless of dict insertion order
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
            # MUST use sort_keys=True here to match the digest computed in
            # record(); otherwise any field-order change in json.dumps would
            # fail verification even on a benign log.
            recomputed = hashlib.sha256(
                (prev + json.dumps(entry, sort_keys=True)).encode("utf-8")
            ).hexdigest()
            if recomputed != stored:
                return False
            prev = stored
        return True


# ── Path Sandboxing ───────────────────────────────────────────────────────
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


# ── Guarded Tool Wrapper (MCP gate) ────────────────────────────────────────
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
