---
name: skill-process-pipeline
description: >
  Full SIN-Code pipeline in one shot — 10 stages, ~40% ecosystem coverage.
  Pre-flight (doctor+lessons+memory+goal+contract+checkpoint) → Grill (design)
  → Plan v2 (+cartographer+episodic+plan-merge) → GSD (+goal subtasks)
  → Execute (+worktree+conflict prediction+fusion+critic+governor+compaction+ledger)
  → Review (+adversary+complexity+ceo-audit+IBD+security) → Done Gate (stop-gate+GoalContract)
  → Commit (auto-commit+auto-PR) → Record (ledger+summary+lessons+memory+compress)
  → CI/CD (GitHub Actions). Triggers on "full pipeline", "/pipeline", "end to end".
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.29.1
required_tools:
  - sin_bash
  - sin_run_loop
  - sin_goal_add
  - sin_sckg
  - sin_orchestrate
lifecycle: native
---

# skill-process-pipeline — Full SIN-Code Stack (10 Stages)

## Overview

Chains 10 SIN-Code systems into a single deterministic pipeline.
Each stage feeds the next. No stage self-declares success without the
downstream gate passing. ~40% ecosystem coverage.

## When to Use

- User says "full pipeline", "/pipeline", "do everything", "end to end"
- High-stakes task needing the complete quality stack
- Task that should be tracked, verified, committed, and CI-deployed

## When NOT to Use

- Quick one-liner (just do it)
- Only need one stage (use the individual skill)
- Purely informational (no code changes)

## The Pipeline (10 Stages)

```
STAGE 0:  PRE-FLIGHT    — doctor, lessons, memory, goal+contract, checkpoint
STAGE 1:  GRILL         — adversarial design interview
STAGE 2:  PLAN          — plan v2 + cartographer + episodic + plan-merge
STAGE 3:  GSD           — phases + goal subtasks
STAGE 4:  EXECUTE       — delegate + worktree + fusion + critic + governor + ledger
STAGE 5:  REVIEW        — self-review + adversary + complexity + ceo-audit + IBD + security
STAGE 6:  DONE GATE     — stop-gate + GoalContract + PoC + Oracle + ADW + SCKG
STAGE 7:  COMMIT        — auto-commit + auto-PR
STAGE 8:  RECORD        — ledger + summary + lessons + memory + compress
STAGE 9:  CI/CD         — GitHub Actions deployment
```

---

### Stage 0: PRE-FLIGHT

**Systems:** doctor, lessons, memory, goal queue, GoalContract, checkpoint
**Input:** User task description
**Output:** Goal ID with contract, primed context, workspace checkpoint

1. **Health check:** `sin-code doctor` — verify Go, config, DBs, MCP, tools
   - If any check fails: report, ask user to fix before proceeding
2. **Lessons query:** Query lessons DB for similar past tasks
   - `sin-code memory search "<task>"` — find relevant memories
   - Look for `TypeFailedVerification` lessons matching the task domain
   - Brief agent: "Past failures to avoid: ..."
3. **Memory prime:** `sin-code memory prime --query "<task>"` 
   - Inject top-K relevant memories as context preamble
4. **Decision memory:** `sin-code decision list --query "<task>"` 
   - Check for prior architectural decisions relevant to this task
5. **Goal enqueue:** `sin-code goal add --prompt "<task>" --priority P1`
   - Record goal ID for tracking throughout pipeline
   - Add DoD criteria as goal contract: `--criteria "tests pass" --criteria "build clean" --criteria "no TODO/FIXME"`
   - This activates the stop-gate (Stage 6)
6. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-0"`
   - Workspace snapshot before any code changes
   - Can rewind here if any later stage goes wrong
7. **Ledger start:** First tool call auto-records to ledger
8. **Hooks:** Fire `session.start` and `goal.enqueued`

**Gate:** Doctor passes, goal ID recorded, checkpoint created.

---

### Stage 1: GRILL (Design Review)

**Skill:** `grill-me` / `skill-process-grill`
**Input:** Task description + primed context from Stage 0
**Output:** Decision tree with resolved and open points

1. `grill_start(topic="$TASK", context="$PRIMED_CONTEXT")`
2. Ask 5+ adversarial questions, one at a time, with recommended answers
3. `grill_record_answer(session_id, question, answer)` for each
4. `grill_synthesize(session_id)` → decisions + assumptions
5. Record decisions: `sin-code decision add --title "<decision>" --context "<grill output>"`

**Gate:** 5+ questions asked, all branches resolved or explicitly open.

If user has clear design, note it and skip to Stage 2.

---

### Stage 2: PLAN (Strategy + Execution Plan)

**Skill:** `plan v2` (`--lite` for simple, full for complex, `--from-spec` if spec exists)
**Input:** Grill decisions + task description
**Output:** Execution-ready plan with phases, tasks, risks, done criteria

1. **Cartographer index:** `sin-code sckg build` — build semantic code graph
   - Query: `sin-code sckg query "<task domain>"` — find hot paths, entry points
   - Feed into plan for dependency-aware task ordering
2. **Episodic memory:** Query orchestrator episodic memory for similar past plans
   - "Last time we did X, phase 2 failed because Y" → adjust plan
3. **Plan v2:**
   - Simple: `plan --lite` (6 stages, S/M/L estimates)
   - Complex: full plan v2 (13 stages, PERT, Monte Carlo, OKRs, risk scoring)
   - From spec: `plan --from-spec` (load `.sin/specs/*.md`, decompose to tasks)
4. **Plan-Merge (complex tasks only):** If `fusion.enabled = true` and task is complex:
   - N planners generate plans in parallel
   - Judge merges best insights into one
   - 1 coder executes the merged plan
5. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-2"`

**Gate:** Plan has phases, tasks with validation, done criteria, risks.

---

### Stage 3: GSD (Project Lifecycle)

**Skill:** `skill-process-gsd`
**CLI:** `sin-code gsd`
**Input:** Approved plan
**Output:** `.gsd/` state with phases, plans per phase, goal subtasks

1. `sin-code gsd init --name "$PROJECT" --description "$TASK"`
2. For each plan phase: `sin-code gsd phase add "$TITLE" --priority $PRIORITY`
3. For each phase: create task plan (save to `.gsd/plans/`)
4. **Goal subtasks:** `sin-code goal subtask <goal-id> --prompt "Execute phase N"`
   - Each phase becomes a goal subtask with its own contract
5. `sin-code gsd status` — verify project structure
6. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-3"`

**Gate:** All phases created, each has a plan, goal subtasks enqueued.

---

### Stage 4: EXECUTE (Parallel Wave Execution)

**Skill:** `delegate-subagents`
**Systems:** worktree, conflict prediction, fusion, critic, governor, compaction, ledger
**Input:** Phase plan with waves
**Output:** Code changes, test results

1. `sin-code gsd execute $PHASE_ID` — analyze waves (topological sort)
2. **Conflict prediction:** `sin-code goal predict-conflicts` before wave execution
   - If conflicts predicted: reorder tasks or assign non-overlapping files
3. **For each wave (dependency-ordered):**
   a. **Worktree isolation (optional):** `sin-code goal worktree create` for each parallel task
      - Each subagent works in isolated git worktree — true parallelism, no file conflicts
   b. Launch all tasks in wave as parallel subagents (`delegate-subagents`)
      - Each subagent gets: full context, file assignment, validation criteria
      - Set `required_tools` per subagent (tool coverage enforcement)
      - Session context injection: lessons + memory primed for each subagent
   c. Wait for all subagents in wave to complete
   d. **Verify each result:** build + test + lint
   e. **On verify.fail:**
      - **Fusion tournament:** `sin-code fusion status` — if enabled, fan task to N providers
        - First to pass verify-gate wins (PoC mode)
        - Or: all run, judge evaluates (Oracle mode)
      - **Critic repair:** Bounded verify-diagnose-retry loop
        - Critic analyzes failure, suggests fix, worker retries
        - Max 3 repair iterations per task
      - **Governor escalation:** single-shot → repair → best-of-N
        - Governor decides when to escalate compute
   f. **Loop detection:** Monitor for stuck subagents (repeated identical tool calls)
      - If detected: abort subagent, report, relaunch with tighter prompt
   g. `sin-code gsd execute $PHASE_ID --task $TASK_ID --status done`
4. **Context compaction:** During long execution, monitor context window
   - `sin-code context` — check usage
   - If >80%: deterministic compaction (preserve verification evidence)
   - If >90%: LLM compaction (summarize older turns)
5. After all waves: full build + test + vet
6. **Ledger:** Every tool call, verify result, fusion dispatch auto-records
7. **Hooks:** Fire `task.complete` per wave, `fusion.dispatch` if fusion used
8. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-4"`

**Gate:** All tasks done, build passes, tests pass, vet clean.

---

### Stage 5: REVIEW (Multi-Layer Quality Review)

**Skills:** `self-review`, `skill-process-grill` (adversary mode)
**Systems:** adversary, complexity, ceo-audit, IBD, security scan
**Input:** All code changes (git diff)
**Output:** Review report with findings (BLOCKER/MAJOR/MINOR/NIT)

1. **Self-review (CEO):** Load `self-review` skill
   - Reconstruct scope from original task + grill decisions + plan
   - Requirement check: every requirement covered?
   - File-by-file check: read every changed file
   - Run `scripts/verify.sh` or `go build && go test -race && go vet`
   - Severity: BLOCKER / MAJOR / MINOR / NIT
2. **Adversary probes:** `sin-code orchestrate --adversary`
   - Proposes executable attacks on the change
   - Probes that run red PROVE the attack landed
   - Survivors become regression tests
3. **Complexity review:** `sin-code review --complexity`
   - Ponytail 5-tag findings: delete / simplify / rebuild / risk / verify
   - Check for sin-debt markers in changed code: `sin-code debt stats`
4. **CEO-Audit (optional, for large changes):** `sin-code ceo-audit --profile QUICK`
   - 48-gate quality audit (security, performance, code quality, deps, tests, docs, compliance)
   - Board-ready Markdown + SARIF report
5. **Intent-Based Diffing:** `sin-code ibd --before <checkpoint> --after HEAD`
   - Compare actual changes against stated intent (from plan)
   - Flag any changes that weren't in the plan (scope creep)
   - Flag any planned changes that are missing (incomplete)
6. **Security scan:** `sin-code security scan --path .`
   - Secrets scanner, SAST, SCA, SBOM, container scan
   - SARIF output for CI integration
7. **Fix all BLOCKER + MAJOR immediately**, re-verify after each fix
8. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-5"`

**Gate:** 0 open BLOCKER, 0 open MAJOR, security scan clean.

---

### Stage 6: DONE GATE (Machine Verification)

**Systems:** stop-gate, GoalContract, PoC, Oracle, ADW, SCKG, dodone
**Input:** Final codebase state + GoalContract from Stage 0
**Output:** Exit 0 (WIRKLICH FERTIG) or Exit 2/3 (back to Stage 4)

1. **Stop-Gate with GoalContract:**
   - Deterministic checks first (fail-closed: ANY failed check blocks):
     - Build: `go build ./...` → exit 0
     - Test: `go test ./... -race -count=1` → exit 0
     - Lint: `go vet ./...` → exit 0
     - Predicates: every `--criteria` from Stage 0 evaluated
     - Diff-scope: changes stay within allowed globs
   - Then LLM judge (`SIN_EVALUATOR_MODEL`) for semantic criteria:
     - "Is the code production-ready?"
     - "Are edge cases handled?"
     - "Does it match the plan?"
   - Green judge can NEVER override a red deterministic check
2. **PoC verification:** `sin-code poc verify` — proof-of-correctness
   - If PoC invariants defined in plan, verify they hold
3. **Oracle check:** `sin-code oracle check` — independent verification
   - LLM judge evaluates claim with evidence
4. **ADW scan:** `sin-code adw scan` — architectural debt watchdogs
   - Detect god modules, circular deps, high coupling in changed code
5. **SCKG dead code:** `sin-code sckg dead_code` — semantic dead code detection
6. **DoDone fallback:** `dodone check` — 11-pillar deterministic check
   - P1: No placeholders (grep TODO/FIXME/panic)
   - P2: Error handling (no empty catch/pass/ignore)
   - P3-P11: Tests, build, artifacts, coverage, invariants, architecture, security, dead code
7. **Interpret exit codes:**
   - Exit 0: WIRKLICH FERTIG → proceed to Stage 7
   - Exit 2: Code incomplete → feed findings to Stage 4, fix, re-run Stage 5+6
   - Exit 3: Tests/Build failed → fix, re-run Stage 5+6
8. **Hooks:** Fire `verify.pass` or `verify.fail`

**Gate:** Exit 0. No exceptions. Lying about the gate is M3 violation.

---

### Stage 7: COMMIT (Auto-Commit + Auto-PR)

**Systems:** auto-commit, auto-PR
**Input:** Verified codebase (Stage 6 exit 0)
**Output:** Git commit + optional PR

1. **Auto-commit:** Conventional commit with auto-detected prefix
   - `feat:` for new features, `fix:` for bug fixes, `docs:` for docs
   - Commit message includes: goal ID, pipeline stages passed, verification evidence
   - Example: `feat: add user auth (goal #42, pipeline: 10/10 stages passed, dodone exit 0)`
2. **Auto-PR (optional):** `sin-code auto-pr`
   - Creates GitHub PR with plan summary, review findings, verification results
   - Links to goal, ledger entries, session summary
3. **GSD phase update:** `sin-code gsd phase edit $PHASE_ID --status completed`
4. **Goal update:** `sin-code goal complete $GOAL_ID`
5. **Hooks:** Fire `commit.post`, `push.pre`

**Gate:** Commit created, optionally PR opened.

---

### Stage 8: RECORD (Learning + Memory + Cleanup)

**Systems:** ledger, summary, lessons, memory, compress, decision memory
**Input:** Completed pipeline run
**Output:** Persisted learnings, session summary, compressed lessons DB

1. **Ledger finalization:** `sin-code ledger list --session $SESSION_ID`
   - Full audit trail of every tool call, verify result, fusion dispatch
   - `sin-code ledger tools` — tool usage heatmap for this pipeline run
2. **Session summary:** `sin-code summary`
   - Deterministic, rule-based summary (no LLM needed)
   - Includes: tokens used, cost, tools invoked, verification status, stages passed
3. **Lessons recording:**
   - Record any `TypeFailedVerification` from Stage 4/6 failures
   - Record `TypeToolError` from any tool failures
   - These will brief the next pipeline run (Stage 0) to avoid repeating mistakes
4. **Memory store:** `sin-code memory add --insight "<key learning>" --tags pipeline`
   - Store architectural decisions from Stage 1/2
   - Store what worked well and what didn't
5. **Compress lessons:** `sin-code compress plan --target lessons && sin-code compress apply`
   - Deduplicate similar lessons, byte-budget, sort
   - Keep lessons DB lean for fast Stage 0 queries
6. **Decision memory:** Update decisions with outcomes
   - "Decision X was applied in pipeline run Y, result: Z"
7. **AutoDream (optional):** Memory consolidation runs in background
   - Consolidates episodic memories into general patterns

**Gate:** Lessons recorded, memory updated, summary generated.

---

### Stage 9: CI/CD (GitHub Actions Deployment)

**Skill:** `sin-git-workflow`
**Input:** Pushed commit/PR from Stage 7
**Output:** CI pipeline running, deployment triggered

1. **GitHub Actions:** The push from Stage 7 triggers CI
   - CEO-Audit workflow runs (M1: n8n delegator)
   - Ecosystem-sync workflow runs
   - Release workflow (if tagged)
2. **Verify CI passes:** Monitor with `gh run watch` or `sin-code gh run`
   - If CI fails: `sin-code auto-pr` self-healing kicks in
   - Or: feed failures back to Stage 4, fix, re-push
3. **SBOM generation:** `sin-code sbom generate --format spdx-json`
   - Attach to release or PR
4. **Documentation sync:** Update CHANGELOG.md, README.md if behavioral changes
5. **Wiki sync (if applicable):** Sync plan to GitHub Wiki

**Gate:** CI passes (or explicitly waived by user).

---

## Failure Handling

| Stage | Failure | Action | Max Retries |
|---|---|---|---|
| 0 Pre-flight | Doctor fails | Report, user fixes | N/A |
| 1 Grill | Unresolved design | User decides, proceed | N/A |
| 2 Plan | Review rejects | 1 revision, then user decides | 1 |
| 3 GSD | Phase conflict | User resolves, re-init | N/A |
| 4 Execute | Subagent fails | Re-launch + critic repair + fusion | 3 per task |
| 4 Execute | Loop detected | Abort, relaunch with tighter prompt | 2 |
| 4 Execute | Context overflow | Deterministic compaction | automatic |
| 5 Review | BLOCKER found | Fix immediately, re-review | until 0 |
| 6 Done Gate | Exit 2/3 | Feed findings to Stage 4, fix, re-run 5+6 | 3 total |
| 7 Commit | Commit fails | Manual commit | 1 |
| 9 CI/CD | CI fails | Auto-PR self-healing or manual fix | 2 |

**Rewind:** At any stage, `sin-code checkpoint rewind --name "pipeline-stage-N"` 
restores the workspace to that checkpoint. Use when a stage goes catastrophically wrong.

---

## Cost Awareness

| Stage | LLM Calls | Duration | Notes |
|---|---|---|---|
| 0 Pre-flight | 0-1 | <30s | CLI + memory queries |
| 1 Grill | 3-10 | 1-3 min | Interactive with user |
| 2 Plan | 4-5 (or 8-12 with plan-merge) | 2-5 min | Parallel research |
| 3 GSD | 0 | <1s | Pure CLI |
| 4 Execute | 5-50+ (task-dependent) | 5-60 min | Fusion adds N×cost |
| 5 Review | 2-5 | 2-10 min | ceo-audit adds ~3 min |
| 6 Done Gate | 0-2 (stop-gate judge) | <30s | Mostly deterministic |
| 7 Commit | 0 | <1s | Git operations |
| 8 Record | 0-1 | <5s | CLI + compress |
| 9 CI/CD | 0 | 2-10 min | External CI runners |
| **Total** | **14-70+** | **15-90 min** | Fusion + ceo-audit add most cost |

---

## Degradation Rules

| Stage | Can Skip? | Condition |
|---|---|---|
| 0 Pre-flight | No | Always run doctor + goal + checkpoint |
| 1 Grill | Yes | User has clear design (note it, proceed) |
| 2 Plan | No | Always need a plan |
| 3 GSD | Yes | Single-phase tasks (plan v2 handles directly) |
| 4 Execute | No | Code must be written |
| 5 Review | No | CEO review is mandatory |
| 6 Done Gate | No | Machine gate is mandatory (M3) |
| 7 Commit | Yes | User doesn't want auto-commit |
| 8 Record | No | Lessons must be recorded (closed learning loop) |
| 9 CI/CD | Yes | No CI configured or user doesn't want it |

---

## Integration Map

```
                    ┌─────────────────┐
                    │  STAGE 0        │
                    │  PRE-FLIGHT     │
                    │  doctor         │
                    │  lessons query  │
                    │  memory prime   │
                    │  goal+contract  │
                    │  checkpoint     │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  STAGE 1        │
                    │  GRILL          │
                    │  grill-me       │
                    │  decision mem   │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  STAGE 2        │
                    │  PLAN           │
                    │  plan v2        │
                    │  cartographer   │
                    │  episodic mem   │
                    │  plan-merge     │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  STAGE 3        │
                    │  GSD            │
                    │  gsd init       │
                    │  phase add      │
                    │  goal subtasks  │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  STAGE 4        │
                    │  EXECUTE        │
                    │  delegate-sub   │
                    │  worktree iso   │
                    │  conflict pred  │
                    │  fusion on fail │
                    │  critic repair  │
                    │  governor escal │
                    │  compaction     │
                    │  ledger record  │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  STAGE 5        │
                    │  REVIEW         │
                    │  self-review    │
                    │  adversary      │
                    │  complexity     │
                    │  ceo-audit      │
                    │  IBD            │
                    │  security scan  │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  STAGE 6        │
                    │  DONE GATE      │
                    │  stop-gate      │
                    │  GoalContract   │
                    │  PoC + Oracle   │
                    │  ADW + SCKG     │
                    │  dodone check   │
                    └────────┬────────┘
                          exit 0
                             │
                    ┌────────▼────────┐
                    │  STAGE 7        │
                    │  COMMIT         │
                    │  auto-commit    │
                    │  auto-PR        │
                    │  gsd complete   │
                    │  goal complete  │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  STAGE 8        │
                    │  RECORD         │
                    │  ledger final   │
                    │  summary        │
                    │  lessons record │
                    │  memory store   │
                    │  compress       │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  STAGE 9        │
                    │  CI/CD          │
                    │  GitHub Actions │
                    │  SBOM           │
                    │  docs sync      │
                    └─────────────────┘
```

## Verification Checklist

- [ ] Stage 0: Doctor passes, goal ID recorded, checkpoint created
- [ ] Stage 1: Grill synthesized (or explicitly skipped with note)
- [ ] Stage 2: Plan has phases, tasks, done criteria, risks
- [ ] Stage 3: GSD status shows all phases with plans
- [ ] Stage 4: All tasks done, build+test+vet pass, ledger recorded
- [ ] Stage 5: 0 BLOCKER, 0 MAJOR, security scan clean
- [ ] Stage 6: Stop-gate exit 0 (WIRKLICH FERTIG)
- [ ] Stage 7: Commit created, goal completed
- [ ] Stage 8: Lessons recorded, summary generated, compress done
- [ ] Stage 9: CI passes (or waived)
