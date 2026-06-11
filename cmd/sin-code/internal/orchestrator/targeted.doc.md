# orchestrator/targeted.go

Targeted Verification — run only the affected test packages.
Inner loop uses `FastChecks` (seconds); the final gate uses `FinalChecks`
(the full suite). Soundness is preserved: nothing merges without the
full run; the inner loop just iterates 10-50x faster.

## Public surface

- `TargetedVerifier{Inner, Graph}`
  - `FastChecks(changedFiles) []Check` — minimal suite for the inner loop
  - `FinalChecks() []Check` — authoritative full gate (= `DefaultGoChecks()`)
  - `VerifyStaged(ctx, taskID, candidate, changedFiles) *Verdict` — fast then full
  - `Speedup(changedFiles) string` — telemetry-friendly reduction ratio

## Behavior

- If the change set is unknown to the impact graph (e.g. config files
  with no `fileToPkg` mapping), the build step falls back to
  `go build ./...` and the test step is skipped (nothing to scope to).
- Fast-fail: if the fast suite is red, the full suite is NOT run.
  Diagnosis comes from the cheap run.
- The candidate ID in the verdict is suffixed `@fast` or `@full` so
  episodes and blame records can distinguish stages.

## Caller responsibilities

- Construct an `ImpactGraph` once (cache it per repo) and pass it to
  the `TargetedVerifier`.
- Call `FastChecks` from the critic's inner loop; call `FinalChecks`
  (or `VerifyStaged`) exactly once before any merge.
