# Loop System Enhancements — Tier 1–3 Roadmap

Follow-up issues that build on the completed loop core (#150–#155). Each issue
file contains the full problem statement, root cause, proposed solution with
**code blocks for every affected file**, and acceptance criteria.

## Tier 1 — Highest leverage

| ID | Title | Depends on | File |
|----|-------|------------|------|
| loop-009 | Parallel sub-agents via errgroup | #153 | [issue-loop-009-parallel-subagents.md](issue-loop-009-parallel-subagents.md) |
| loop-010 | Adaptive re-planning on stall | #150 | [issue-loop-010-adaptive-replanning.md](issue-loop-010-adaptive-replanning.md) |
| loop-011 | Diff-based progress score | #150, #010 | [issue-loop-011-diff-progress-score.md](issue-loop-011-diff-progress-score.md) |

## Tier 2 — Quality & learning

| ID | Title | Depends on | File |
|----|-------|------------|------|
| loop-012 | Persistent lesson-learning across runs | — | [issue-loop-012-persistent-lessons.md](issue-loop-012-persistent-lessons.md) |
| loop-013 | Speculative verification mid-run | #154 | [issue-loop-013-speculative-verification.md](issue-loop-013-speculative-verification.md) |
| loop-014 | Confidence-weighted stop-gate | — | [issue-loop-014-confidence-stopgate.md](issue-loop-014-confidence-stopgate.md) |

## Tier 3 — Observability & control

| ID | Title | Depends on | File |
|----|-------|------------|------|
| loop-015 | Structured run-trace & replay | — | [issue-loop-015-run-trace.md](issue-loop-015-run-trace.md) |
| loop-016 | Cost-aware model routing | #151 | [issue-loop-016-cost-aware-routing.md](issue-loop-016-cost-aware-routing.md) |
| loop-017 | `.sin-code.yml` expansion | #155, #010, #013, #016 | [issue-loop-017-repo-config-expansion.md](issue-loop-017-repo-config-expansion.md) |

## Recommended implementation order

1. **loop-010** (re-planning) + **loop-011** (diff progress) — together they fix
   the core "burns budget without real progress" weakness.
2. **loop-013** (speculative verify) — reduces stop-gate rejects at the source.
3. **loop-009** (parallel sub-agents) — wall-clock speedup once delegation is proven.
4. **loop-012** (lessons) + **loop-014** (confidence) — compounding quality gains.
5. **loop-015** (trace) + **loop-016** (routing) + **loop-017** (config) —
   observability and declarative control to operate the system at scale.

## Design invariants (shared by all issues)

- Every new capability is **opt-in**: a nil/zero field preserves exact legacy behavior.
- Every escalation/decision emits a **hook** and a **ledger entry** for observability.
- Best-effort side paths (lessons, speculative checks) **never fail a run**.
- New deterministic state must be **concurrency-safe** once sub-agents run in parallel (#009).
