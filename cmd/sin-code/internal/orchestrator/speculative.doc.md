# orchestrator/speculative.go

Speculative Best-of-N — N agents attack the same task in parallel inside
isolated git worktrees. The `BestVerdict` wins, the winner is merged,
losing worktrees are destroyed.

## Public surface

- `Candidate{ID, Agent, Worktree, Output, Verdict, Err}` — one attempt
- `SpeculativeRunner{RepoRoot, Checks, MaxParallel, KeepLosers, WorkdirBase}`
  - `Run(ctx, task, agents, scratch) *SpecResult`
  - `MergeWinner(ctx, winner) (diff, error)` — applies the winner's patch
- `SpecResult{Winner, Candidates}` — full trace

## Behavior

- Bounded fan-out: `MaxParallel` semaphore (default 3).
- When `RepoRoot` is empty the runner operates in a scratch directory
  (used for tests and for non-git workspaces) — `MergeWinner` returns
  an error in that mode since there is no base to diff against.
- Loser worktrees are removed via `git worktree remove --force` +
  `os.RemoveAll` (best-effort cleanup).
- `BestVerdict` is deterministic: passed first, then highest score,
  then earliest `CreatedAt`.

## Failure modes

- `git worktree add` failure → candidate has `Err`, `Verdict.Passed=false`.
- Agent-level error → captured in `Candidate.Err`; verification still
  runs and produces a verdict.
