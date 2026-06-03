# policy.py

Risk-gating, approval, and tamper-evident audit logging for SIN tools.
MCP has no native access control; this module wraps every tool execution
with:

1. a per-tool risk classification (`read | write | exec | network`)
2. a configurable policy (`allow | ask | deny`) per risk class
3. an append-only, hash-chained audit log under `.sin/audit/log.jsonl`
4. path sandboxing helpers so tools cannot read/write outside the
   project root

Policy is loaded from `.sin/policy.yaml`; safe defaults are used if the
file is missing or `pyyaml` is not installed.

## Dependencies

- stdlib: `hashlib`, `json`, `os`, `time`, `dataclasses`
- optional: `pyyaml` (only when loading a real policy.yaml)

## Touched by

- `cli.py` — `sin policy` exposes the policy view/edit commands
- The MCP server wraps every tool call with `guarded()`

## What it does

1. **`TOOL_RISK`** — canonical tool name → risk class map.
2. **`DEFAULT_POLICY`** — safe defaults: reads `allow`, everything else
   `ask`.
3. **`Policy.load(root)`** — loads `.sin/policy.yaml` and merges with
   defaults; honours `auto_approve: true` (and `SIN_AUTO_APPROVE=1`).
4. **`AuditLog.record(...)`** — appends a JSON line with
   `{ts, tool, risk, decision, outcome, args_keys, prev, hash}` where
   `hash = sha256(prev + canonical_json(entry))`.
5. **`AuditLog.verify_chain()`** — recomputes the chain and returns
   `True` iff no line was tampered with.
6. **`ensure_within_root(target, root)`** — resolves `target` relative
   to `root` (or `$SIN_PROJECT_ROOT`) and raises `PolicyError` if the
   result escapes the project.
7. **`guarded(tool, args, run, approver)`** — full gate. Denies outright
   for `decision="deny"`, prompts (or auto-deny) for `"ask"`, runs
   otherwise. Every outcome is audit-logged.

## Important config

- `TOOL_RISK` — add new tools here as the bundle grows
- `DEFAULT_POLICY` — the safe baseline; never `allow` for `exec` or
  `network`
- `SIN_AUTO_APPROVE=1` — opt-in to skip approval prompts (CI / trusted)
- Audit log: `<root>/.sin/audit/log.jsonl`

## Usage

```python
from pathlib import Path
from sin_code_bundle.policy import guarded

def my_tool():
    return {"ok": True}

result = guarded(
    "impact",  # risk=read → allowed by default
    args={"symbol": "PoolManager"},
    run=my_tool,
    root=Path("."),
)
```

## Known caveats

- The audit log's hash chain detects *edits* but not *truncation*; a
  malicious actor with write access can still drop the file.
- `ensure_within_root` uses `Path.resolve()` — symlinks pointing
  outside the project can confuse it. Consider realpath with `strict=True`
  for stricter checks.
- `auto_approve` defaults to `False` so non-interactive runs are
  safe-by-default. Set `SIN_AUTO_APPROVE=1` only in trusted CI.
