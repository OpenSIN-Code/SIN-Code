# orchestrator/governor.go

Budget Governor — escalating ladder of compute strategies. Rung 1
(single-shot) → Rung 2 (single + repair) → Rung 3 (best-of-N + repair).
Each climb is logged with its justification (the failing verdict).

## Public surface

- `Rung{Name, Agents, RepairRounds, Timeout}`
- `DefaultLadder() []Rung` — three-rung default ladder
- `Escalation{FromRung, ToRung, Reason, Verdict, At}`
- `GovernorResult{Passed, FinalRung, Verdict, Escalations, TotalRounds}`
- `AgentFactory func(rung) []Agent` — per-rung agent construction
- `Governor{Ladder, Verifier, Checks, RepoRoot, Factory, Router}`
  - `Execute(ctx, task, scratch) *GovernorResult`

## Behavior

- For `Agents <= 1`: uses the `Critic` with `MaxAttempts = RepairRounds + 1`.
- For `Agents > 1`: uses `SpeculativeRunner` (best-of-N). If the winner
  fails, the rung's `RepairRounds` budget is spent on the winner inside
  its worktree via the `Critic`, then merged.
- Every escalation is recorded with the failing verdict and a reason
  string. Audit-friendly.
- All rungs respect the rung's per-rung `Timeout` (via
  `context.WithTimeout`).

## Caller responsibilities

- The factory must return `[]Agent` for each rung; rung 3 may use a
  stronger model (factory decides).
- `RepoRoot` enables the speculative fan-out; with empty `RepoRoot` the
  governor still runs but skips `git worktree add` (each candidate
  gets an empty scratch dir) and the `MergeWinner` step is a no-op.
