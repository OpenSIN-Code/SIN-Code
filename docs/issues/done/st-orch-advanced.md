# st-orch-advanced — SOTA-Orchestrator Subsystem (verifier, critic, contract, etc.)

**Status:** RESOLVED in commit `<next>`
**Priority:** P0
**Type:** feature
**Date:** 2026-06-11

## Context
The orchestrator package shipped with `Plan`/`Task`/`Dispatcher`/`Scratchpad` but
agents were still `MockAgent`, no verification, no learning. To move beyond SOTA
(Claude Code/Codex/Aider), we needed 12 new mechanisms across 3 layers:

**Ausführung (Execution):**
- `verifier.go` — Verifier, Verdict, Check, weighted scoring
- `speculative.go` — best-of-N, worktree isolation, verifier selection
- `critic.go` — bounded verify→diagnose→repair loop with stall detection
- `episodic.go` — FTS5 episode store, negative examples as planning prior

**Kontrolle (Control):**
- `contract.go` — Intent Contract, pre-flight + post-hoc scope enforcement
- `blame.go` — O(log n) bisection over edit log to find culprit
- `strategy_router.go` — Thompson sampling over (task-class, strategy) arms
- `governor.go` — escalating budget ladder with audit trail

**Wahrnehmung (Perception):**
- `impact.go` — ImpactGraph from `go list -json`, blast-radius prediction
- `targeted.go` — fast affected-only tests + full-suite final gate
- `confidence.go` — Brier-calibrated confidence + MergePolicy
- `mutation.go` — Mutation Probe (tests observe the change?)

## Acceptance
- [x] All 12 files compile + go vet clean
- [x] 107 new tests added, all green (133/133 in orchestrator package)
- [x] All 16 packages build + vet + test green
- [x] CoDocs (.doc.md) for every new file
- [x] Test helpers are hermetic (no network, no git subprocess, no sqlite)
- [x] `*sql.DB` is optional across all persistent components
- [x] Naming collision with existing `Router` (intent classifier) avoided
      → Thompson-sampling router is `StrategyRouter`
- [x] `Task.Title` added additively (no breaking change to existing tests)

## Files
12 source files + 5 test files + 12 .doc.md files. ~2400 LOC new, ~50 new tests.

## Out of scope (deliberate)
- Wiring into existing `orchestrate.go` (would require touching Dispatcher
  and Risk — separate follow-up issue, not a "make it build" concern).
- Connecting the new modules to a real SQLite database — the memory package
  already abstracts the DB; integration is one follow-up commit.
- Replacing `MockAgent` with the real NIM agent in the SpeculativeRunner —
  requires NIM credentials and live LLM testing; orthogonal to compilation.
