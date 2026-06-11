# orchestrator/critic.go

Bounded verify→diagnose→repair loop. When a candidate fails verification
the `Verdict.Diagnosis()` is appended to the task description and the
agent retries — with the exact failing check output as context, not a
vague "it failed".

## Public surface

- `RepairPolicy{MaxAttempts, MinImprovement}`
  - `DefaultRepairPolicy()` → 3 attempts, 0.05 minimum improvement
- `Attempt{Round, Output, Verdict, Diagnose}`
- `CriticResult{Attempts, Final, Passed}`
- `Critic{Verifier, Checks, Policy}`
  - `Drive(ctx, agent, task, scratch) *CriticResult`

## Loop semantics

1. First attempt uses the original task description.
2. If a previous attempt failed, the description is wrapped with the
   diagnosis and the previous attempt is **discarded** — repair is
   targeted, not additive.
3. Stall detection: if a new attempt scores lower than
   `bestScore + MinImprovement`, the loop breaks early.
4. Original task description is restored in a `defer` even on panic.

## Caller responsibilities

- `Drive` mutates `task.Description` in-place per round. Callers that
  hold references to the original description must re-fetch after
  `Drive` returns (the `defer` restores it).
- `Verifier` is shared between rounds — `Checks` should be re-runnable
  (idempotent) since they may share build cache.
