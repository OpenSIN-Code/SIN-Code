# orchestrator/verifier.go

Verified Execution — every agent action is gated by machine-checkable
postconditions instead of "exit 0". A `Verdict` is a structured,
replayable record: it is the contract between execution, speculative
selection, and repair.

## Public surface

- `Check{Kind, Name, Cmd, Timeout, AllowedPaths}` — single postcondition
- `CheckResult{Check, Passed, Output, Duration}` — outcome of one check
- `Verdict{TaskID, Candidate, Results, Score, Passed, CreatedAt}` — aggregate
  - `Score`: weighted pass ratio over all checks (`build=1.0, test=0.9, diff-scope=0.7, predicate=0.5, lint=0.3`)
  - `Passed`: every mandatory check passed (default mandatory: `build`+`test`)
  - `Diagnosis()`: structured repair context with failing-check output
- `Verifier{Workdir, MandatoryKinds}` — runs checks
  - `Verify(ctx, taskID, candidate, checks) *Verdict` — always returns a verdict
- `DefaultGoChecks()` — canonical Go suite (`go build`, `go test`, `go vet`)
- `BestVerdict(verdicts) *Verdict` — passed first, then highest score, then earliest

## Consumers

- `SpeculativeRunner.runCandidate` — selection
- `Critic.Drive` — repair context
- `TargetedVerifier.VerifyStaged` — staged fast+full gate
- `MutationProbe` — uses the verifier per fast-stage run

## Notes

- A check that cannot start counts as failed, never as an error —
  verification must always produce a verdict.
- `runCheck` enforces a 3-minute default timeout per check.
- `truncate` caps output at 2000 bytes to keep verdicts replayable.
