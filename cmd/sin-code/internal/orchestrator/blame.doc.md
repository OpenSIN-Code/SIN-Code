# orchestrator/blame.go

Causal Blame — bisect the edit log to find the first edit that flipped a
check green→red. O(log n) verification runs via binary search. The
precondition is that the full edit log fails the check; the postcondition
is `BlameResult.Culprit` pointing at the responsible edit (or `nil` if
the failure pre-existed).

## Public surface

- `EditRecord{Seq, SHA, Path, Summary}`
- `EditLog{TaskID, Workdir, Base, Edits}`
- `BlameResult{Culprit, Check, Bisections, PriorGreen}` — `PriorGreen` is
  the highest seq that still passes (repair can rewind to it).
- `Blamer{Verifier}`
  - `Blame(ctx, log, failing) *BlameResult`

## Behavior

- Step 1: check the base commit. If it already fails, return early —
  the failure is pre-existing, not caused by current edits.
- Step 2: binary search the edit log by SHA. `checkAt` checks out the
  SHA in detached mode, runs the single check, restores the tip in `defer`.
- The "no git workdir" mode (Workdir empty) treats every prefix as
  passing — useful for tests.

## Caller responsibilities

- The agent layer must commit each applied edit to a scratch branch
  (`sin/run-<taskID>`) and append to `EditLog.Edits` before calling `Blame`.
- `Verifier` should be configured to run **only the failing check** for
  bisect efficiency (`Verify(ctx, ..., []Check{c})`).
