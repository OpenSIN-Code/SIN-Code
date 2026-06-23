# `baseline.doc.md` — The SinCode Loop System (always-on Definition-of-Done)

The baseline is the answer to one requirement: **never tell an agent again to
write tests, debug, finish the job, or update the docs.** Every goal implicitly
carries that work, so the loop injects it automatically. The baseline encodes
those "self-evident" obligations as a `GoalContract` fragment that is merged
into every resolved contract and enforced by the independent stop-gate.

## What it provides

- **`BaselineSemanticCriteria()`** — the LLM-judged rubric every goal is held to:
  tests actually exercise new behavior, suite passes, no debug scaffolding left,
  goal fully addressed (no stubs/TODOs), docs updated (README, CHANGELOG,
  MASTER_TODO/backlog, AGENTS.md, per-package `.doc.md`), and implied related
  work done.
- **`BaselineChecks(workspace)`** — mechanical, fail-closed predicates for Go
  repos:
  | Check | Fails when… |
  |---|---|
  | `baseline-tests-changed-with-code` | production `.go` changed but no `_test.go` changed |
  | `baseline-changelog-updated` | production `.go` changed but `CHANGELOG.md` untouched |
  | `baseline-codoc-present` | a changed package directory has no `*.doc.md` |
- **`Baseline(workspace)`** — bundles the two above into a `*GoalContract`.
- **`Preamble(contract)`** — renders the semantic criteria into a Definition-of-
  Done briefing injected into the worker prompt (via `loopbuilder`), so the
  agent does the work proactively instead of being told to after a rejection.
- **`BaselineEnabled(disable)`** — single source of truth for the on/off
  decision: ON by default, OFF when `disable` (the `--no-baseline` flag) is true
  or `SIN_BASELINE` is a falsey value (`off/0/false/no/disable`).

## Design — HYBRID, and safe by construction

- **Two halves, matching the stop-gate.** Cheap mechanical omissions are caught
  by deterministic predicates (fail-closed); judgement-heavy expectations are
  semantic criteria for the LLM judge. A green judge can never override a red
  predicate.
- **Every predicate is FAIL-OPEN.** Outside a git repo, on an empty diff, or on
  a docs-only change, the predicates exit 0. They only fire when production
  `.go` files actually changed, so the baseline never blocks work it cannot
  fairly judge.
- **Additive & deduped.** `Resolve(IncludeBaseline:true)` appends baseline
  checks/criteria only when not already present (by check name / exact
  criterion), so explicit `--criteria`, `--contract-file`, and auto-detected Go
  checks all coexist with the baseline.

## Where it is wired (default-on)

- `goalcontract.Resolve` — merges the baseline when `IncludeBaseline` is set.
- `cmd/sin-code/daemon_cmd.go` — every leased goal: `IncludeBaseline:
  BaselineEnabled(--no-baseline)`.
- `cmd/sin-code/auto_cmd.go` (`auto run`) — one session-wide contract holds every
  autopilot experiment to the baseline.
- `cmd/sin-code/internal/loopbuilder/builder.go` — turns the contract's criteria
  into the worker `Preamble`.
- Sub-goals spawned via `spawn_subgoal` inherit the baseline automatically,
  because the daemon re-resolves it when each child is leased.

## Scope & caveats

- **Goal executors only.** The baseline is applied by the autonomous goal
  loops (`daemon`, `auto`). Interactive/multi-agent surfaces (`swarm`, the WebUI
  chat `serve`, the TUI) are intentionally left opt-in so a quick interactive
  turn is never forced to refuse completion until docs/tests exist.
- **Go-only mechanical checks today.** The predicates key off `go.mod`. Other
  ecosystems still get the full semantic rubric; extend `BaselineChecks` to add
  language-specific mechanical gates.
- **CoDoc check verifies presence, not freshness.** Whether a `.doc.md` was
  meaningfully updated is left to the semantic judge.
- **Disable globally** with `--no-baseline` or `SIN_BASELINE=off`.
