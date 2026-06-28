---
name: skill-debug-deep
description: Use when user says 'debug', 'trace bug', 'why does this fail', 'root cause', 'RCA', 'deep debug'. Ultimate enterprise debugging workflow — facts-first RCA, cross-tool intent discovery, parallel subagents, web validation, minimal safe fix, and persistent knowledge flush.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
required_tools:
  - sin_scout
  - sin_grasp
  - sin_poc
lifecycle: native
sources: 
---

# Enterprise Deep Debug

Use this skill when a bug is complex, cross-cutting, flaky, or enterprise-scale (distributed systems, async flows, microservices, cloud-native, event streams).

## Triggers

- "deep debug", "root cause analysis", "RCA", "system-wide debugging"
- "flaky", "regression", "prod incident", "postmortem"

## Hard Rules (Anti-Hallucination)

- Do not patch before you have a reproducible failing case (command, input, expected vs actual).
- Prefer evidence over intuition. Every claim links to: file path + line, command output, or a cited external URL.
- Parallelize investigation, not patching: subagents gather evidence only; edits happen after synthesis.
- One hypothesis, one discriminating experiment. Change one variable at a time.
- Stop conditions are mandatory: set budgets and terminate when exceeded (no infinite loops).
- Secrets: never print or persist tokens/keys. Redact any credential-like strings.
- Any read outside the repo must come from the Project SSOT Source Map or from a direct project link discovered in repo evidence.
- Never apply a patch that fails validation.

## Budgets + Termination

- Wall clock: 35 min default (user can raise/lower).
- Phase budgets:
  - Phase 0 + 0.5 (repro + triage): 8 min
  - Phase I (intent discovery): 6 min
  - Phase II (evidence gathering): 12 min
  - Phase III (synthesis): 5 min
  - Phase IV (fix + validate): 10 min
- Hard stop behavior:
  - If Phase 0 gate (repro) is not met within budget: stop and request exactly what is missing.
  - If the hypothesis cannot be discriminated within remaining experiment budget: stop with top 2 hypotheses + the single best next discriminating experiment.
- Maintain a budget ledger in chat: time_spent, remaining_budget, queries_used, experiments_used, patch_iterations_used.

## Workflow

```
REPRO → INTENT → EVIDENCE → SYNTHESIZE → FIX → VALIDATE → FLUSH
```

1. **Reproduce** the failure with a minimal, reproducible case.
2. **Intent discovery** — understand what the code is supposed to do.
3. **Evidence gathering** — collect file paths, line numbers, command outputs, URLs.
4. **Synthesis** — form and discriminate hypotheses.
5. **Fix** — apply the minimal safe patch.
6. **Validate** — run the repro and confirm the fix.
7. **Knowledge flush** — write the RCA and lessons learned.

## Verification

- [ ] Reproducible failing case exists.
- [ ] Every claim has evidence (file + line, output, or URL).
- [ ] Hypotheses were discriminated with experiments.
- [ ] Budget ledger was maintained.
- [ ] Patch passes validation.
- [ ] RCA is documented.
