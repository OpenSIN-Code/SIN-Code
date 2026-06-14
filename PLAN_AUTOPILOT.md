# ULTRA PLAN — SIN-Code Autopilot (Ultra-Autonomous Coding)

> Goal: turn SIN-Code from a *reactive* coding CLI (you prompt, it codes) into an
> *ultra-autonomous* coding system that, given a single high-level **objective**,
> proposes its own work, executes it through the verified agent loop, **measures**
> the result against a metric, **keeps or reverts** the change, learns, and repeats
> — until a budget is exhausted. No per-task prompting required.
>
> Inspired by [`karpathy/autoresearch`](https://github.com/karpathy/autoresearch)
> (metric-driven overnight optimization loops, `program.md` as the only human-edited
> file) and [`OpenSIN-Code/autodev-cli`](https://github.com/OpenSIN-Code/autodev-cli)
> (verification-first gates + bounded autonomy + closed learning loop).

---

## 1. What already exists (reused, not rebuilt)

| Capability | Package | Status |
|---|---|---|
| PLAN→ACT→VERIFY→DONE loop | `internal/agentloop` | ✅ mature |
| Verification gate (M3) | `internal/verify` | ✅ |
| Persistent goal queue (lease/retry/priority) | `internal/autonomy` (`queue.go`) | ✅ |
| Cron + file-watch triggers | `internal/autonomy` (`triggers.go`) | ✅ |
| Autonomous worker daemon | `daemon_cmd.go` | ✅ |
| Closed learning loop (SQLite lessons) | `internal/lessons` | ✅ |
| Multi-agent orchestration | `internal/orchestrator` | ✅ |
| Loop assembly | `internal/loopbuilder` | ✅ |

**The daemon today still needs goals added manually** (`sin-code goal add ...`).
That is the autonomy gap this plan closes.

## 2. The gap: objective-driven self-direction

`autoresearch`'s key insight: the human edits **only** `program.md` (objective +
metric + budget). The agent generates and runs its own experiments. SIN-Code has the
*execution* primitives but no *self-direction* layer that:

1. reads a high-level objective + success metric + budget (`program.md`);
2. **proposes** the next best concrete goal (the "researcher"/mutator);
3. runs it through the existing verified loop;
4. **extracts a numeric metric** from the verify command output;
5. **keeps** the change if the metric improved, **reverts** (git) otherwise;
6. records an **experiment journal** entry + a **lesson**;
7. enforces **bounded autonomy** (wall-clock + experiment caps, M4);
8. loops until budget is spent, then prints a session report.

## 3. New layer: `internal/autopilot`

```
OBSERVE ─► PROPOSE ─► ACT (agentloop) ─► VERIFY ─► MEASURE ─► KEEP / REVERT ─► LEARN ─┐
   ▲                                                                                  │
   └──────────────────────────── until budget exhausted ─────────────────────────────┘
```

### Files (each gets its own issue with full code)

| # | File | Responsibility |
|---|---|---|
| 1 | `internal/autopilot/program.go` | Parse `program.md` → Objective, Metric, Direction (min/max), BudgetMinutes, MaxExperiments, Invariants |
| 2 | `internal/autopilot/budget.go` | Bounded autonomy watchdog (wall-clock + experiment caps), M4 |
| 3 | `internal/autopilot/metric.go` | Extract numeric metric from verify output (regex), compare, decide improvement |
| 4 | `internal/autopilot/snapshot.go` | Git keep/revert: snapshot before, commit on keep, hard-reset on revert |
| 5 | `internal/autopilot/journal.go` | SQLite experiment journal (proposal, metric before/after, kept/reverted) |
| 6 | `internal/autopilot/proposer.go` | The "researcher": propose next goal from objective + journal + lessons (LLM + deterministic fallback) |
| 7 | `internal/autopilot/autopilot.go` | Orchestrator wiring all of the above onto the existing verified loop |
| 8 | `auto_cmd.go` (top-level) | `sin-code auto` command (self-registers via `init()`) |
| + | `program.md` template + `*_test.go` | Bootstrap + tests |

## 4. Bounded autonomy (safety, non-negotiable)

- **M3 verification-first**: every kept change must pass the verify gate. `auto`
  refuses to start without a verify command (same contract as `daemon`).
- **M4 bounded**: hard `--budget-minutes` and `--max-experiments`; the budget
  watchdog stops the loop deterministically.
- **AGENTS.md firewall**: invariants in `program.md` / `AGENTS.md` are read-only
  context; the proposer is instructed never to touch them.
- **Headless = ask→deny**: like the daemon, autopilot cannot self-escalate
  permissions.
- **Reversible**: every experiment is a git snapshot; a bad change is hard-reset,
  never left half-applied.

## 5. `program.md` format

```markdown
# Objective
Reduce p95 latency of the JSON parser without breaking any tests.

## Metric
name: bench_ns_per_op
direction: minimize
extract: /bench_ns_per_op=([0-9.]+)/

## Budget
minutes: 120
max_experiments: 24

## Invariants (DO NOT MODIFY)
- Public API of pkg/parser stays source-compatible
- All existing tests keep passing
```

## 6. CLI

```bash
# bootstrap
sin-code auto init                 # writes program.md template + .sin-code/

# run autonomously (overnight)
sin-code auto run \
  --verify-cmd "go test ./... && go test -bench=. -run=^$ ./pkg/parser" \
  --budget-minutes 120 --max-experiments 24

# inspect
sin-code auto status --json        # budget left, best metric, last experiments
sin-code auto journal              # full experiment history
```

## 7. Metric-driven keep/revert (the autoresearch core)

```
snapshot = git stash-create / commit baseline
run goal through verified loop
if !verified: revert; journal(reverted, reason=verify-fail); learn; continue
m = metric.Extract(verifyOutput)
if metric.Improved(best, m): git commit (keep); best = m; journal(kept)
else: git reset --hard snapshot (revert); journal(reverted, reason=regressed); learn
```

## 8. MCP / WebUI exposure (follow-up)

Expose `autopilot_status`, `autopilot_journal`, `autopilot_run` as MCP tools
(mirror autodev-cli's `autodev-mcp`) so the WebUI v2 can drive overnight runs.

## 9. Test plan

- `program_test.go` — parsing, defaults, invariant extraction
- `budget_test.go` — time + experiment caps, expiry
- `metric_test.go` — regex extraction, minimize/maximize comparison, no-metric case
- `snapshot_test.go` — keep commits, revert hard-resets (temp git repo)
- `journal_test.go` — record/query round-trip, best-so-far
- `proposer_test.go` — deterministic fallback proposal, lesson injection
- `autopilot_test.go` — full OBSERVE→…→LEARN cycle with fakes (no real LLM/git)

## 10. Rollout

1. PR 1: `autopilot` package + `auto` command + tests (this plan).
2. PR 2: MCP tools + WebUI v2 wiring.
3. PR 3: multi-agent autopilot (swarm of proposers, first-verified-improvement-wins).
