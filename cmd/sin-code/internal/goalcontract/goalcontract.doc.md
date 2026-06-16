# `goalcontract.doc.md` — Definition-of-Done Contracts

A `GoalContract` is a machine-checkable Definition-of-Done for an autonomous goal.
It is the data half of the anti-babysitting loop: completion authority is taken
away from the worker and handed to a contract that an independent stop-gate
(`internal/stopgate`) enforces. A goal is "done" only when its contract is
satisfied — not when the model stops emitting tool calls.

## What it does

- **Bundles two kinds of acceptance criteria:**
  - **Deterministic checks** (`[]orchestrator.Check`): build/test/lint/predicate/
    diff-scope — reproducible, free, fail-closed. Reused from `internal/orchestrator`.
  - **Semantic criteria** (`[]string`): non-mechanical statements ("docs updated",
    "goal fully addressed") evaluated by the LLM judge in the stop-gate.
- **Marshal/Unmarshal:** serializes to a compact JSON string persisted in the
  goal queue's `contract` column. An empty contract marshals to `""` and is
  treated as "verify-gate only" (backwards-compatible with the pre-contract loop).
- **Resolve:** builds a contract from layered sources, additively, in priority
  order so the strictest reasonable contract wins.

## Resolution priority (additive, not exclusive)

1. **Explicit contract file** (`--contract-file`, JSON) + inline `--criteria` +
   `--done-when` shell predicate.
2. **Auto-detected repo checks** — e.g. a `go.mod` adds `DefaultGoChecks()`
   (build/test/vet) plus a `no-new-todos` diff guard.
3. **Verify-cmd fallback** — only when nothing else produced a deterministic
   check, the `--verify-cmd` becomes the single predicate.

## Files that import / touch it

- `cmd/sin-code/internal/stopgate/stopgate.go` — consumes the contract to decide completion.
- `cmd/sin-code/internal/loopbuilder/builder.go` — attaches a resolved contract to the loop's stop-gate.
- `cmd/sin-code/daemon_cmd.go` — resolves per-goal contracts before running.
- `cmd/sin-code/goal_cmd.go` — `goal add --criteria/--contract-file` builds & persists contracts.

## Important values & limits

| Field | Meaning |
|---|---|
| `DeterministicChecks` | fail-closed mechanical gates |
| `SemanticCriteria` | LLM-judged, only reached after deterministic pass |
| `MaxFilesChanged` / `MaxLinesChanged` | diff-scope bounds (0 = unbounded) |

- `IsEmpty()` is true for nil/zero contracts; such contracts disable extra gating.
- `noNewTodosScript` exits 0 outside a git repo (cannot judge → never block).

## Known caveats / footguns

- **Auto-detect is Go-only today.** Other ecosystems fall through to the
  verify-cmd fallback. Extend `autoDetectChecks` per language.
- **Semantic criteria need a judge.** Without `SIN_EVALUATOR_MODEL`/a judge in
  the stop-gate, semantic criteria are skipped (deterministic checks still run).
- **`no-new-todos` compares against `HEAD`.** Uncommitted baseline TODOs already
  in the tree are not flagged — only diff-introduced ones.
