---
name: skill-process-pipeline
description: >
  Full SIN-Code pipeline — 10 stages, ~60% ecosystem coverage (v3.29.2).
  Pre-flight (doctor+lessons+instinct+memory+rules+context+goal+contract+checkpoint+sandbox+triage)
  → Grill → Plan (+research+web search+spec-driven+cartographer+episodic+plan-merge+compile-spec)
  → GSD (+goal subtasks) → Execute (+testgate+testgen+worktree+conflict+fusion+critic+governor
  +agentteams+autolevel+background+telemetry+compaction+ledger+rules+sandbox+egress)
  → Review (+self-review+adversary+complexity+ceo-audit+IBD+security+codocs+dox+coverdrohne)
  → Done Gate (stop-gate+GoalContract+testgate+mutation+fuzz+PoC+Oracle+ADW+SCKG+compile-spec+cover)
  → Commit (auto-commit+auto-PR) → Record (+ledger+summary+lessons+instinct+memory+compress+tokens+imagegraph+share)
  → CI/CD (+GitHub Actions+SBOM+dox+profile+eval+benchmark). Triggers on "full pipeline", "/pipeline".
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.29.2
required_tools:
  - sin_bash
  - sin_run_loop
  - sin_goal_add
  - sin_sckg
  - sin_orchestrate
  - sin_web_search
lifecycle: native
---

# skill-process-pipeline — Full SIN-Code Stack (10 Stages, v3.29.2)

## Overview

Chains 55+ SIN-Code systems into a single deterministic pipeline. ~60% ecosystem coverage.
Each stage feeds the next. No stage self-declares success without the downstream gate passing.

## When to Use

- User says "full pipeline", "/pipeline", "do everything", "end to end"
- High-stakes task needing the complete quality stack
- Task that should be tracked, verified, committed, and CI-deployed

## When NOT to Use

- Quick one-liner (just do it)
- Only need one stage (use the individual skill)
- Purely informational (no code changes)
- Metric-driven optimization (use `sin-code auto` — autopilot mode)

## Lightweight Alternatives

| Alternative | When | What |
|---|---|---|
| `sin-code prp` | Simple structured task | PRP workflow: draft→planned→implementing→verifying→ready→shipped |
| `sin-code auto` | Optimization tasks | Autopilot: observe→propose→act→verify→measure→keep/revert→learn |
| `plan v2 --lite` | Quick plan + execute | 6-stage flow without full ceremony |
| `dodone check` | Just need the gate | 11-pillar deterministic check, no pipeline |

## The Pipeline (10 Stages)

```
STAGE 0:  PRE-FLIGHT    — doctor, lessons, instinct, memory, rules, context bridge,
                          triage, config, sandbox, egress, lsp, mcp, profile, goal+contract,
                          autolevel, checkpoint, agentteams setup
STAGE 1:  GRILL         — adversarial design interview, decision memory
STAGE 2:  PLAN          — research, web search (M8), spec-driven, cartographer, episodic,
                          plan-merge, compile-spec, modelperf, plan v2
STAGE 3:  GSD           — phases, goal subtasks
STAGE 4:  EXECUTE       — delegate, testgate, testgen, worktree, conflict, fusion, critic,
                          governor, agentteams, autolevel, background, telemetry, compaction,
                          ledger, rules, sandbox, egress, debug-deep on failure
STAGE 5:  REVIEW        — self-review, adversary, complexity, ceo-audit, IBD, security,
                          codocs, dox, coverdrohne
STAGE 6:  DONE GATE     — stop-gate, GoalContract, testgate, mutation, fuzz, PoC, Oracle,
                          ADW, SCKG, compile-spec, cover check
STAGE 7:  COMMIT        — auto-commit, auto-PR, gsd complete, goal complete
STAGE 8:  RECORD        — ledger, summary, lessons, instinct, memory, compress, tokens,
                          imagegraph, share
STAGE 9:  CI/CD         — GitHub Actions, SBOM, dox, profile, eval, benchmark
```

---

### Stage 0: PRE-FLIGHT

**Systems:** doctor, lessons, instinct+RAG, memory, rules, context-bridge, triage, config, sandbox, egress, lsp, mcp-install, profile, goal, GoalContract, autolevel, checkpoint, agentteams, gh-doctor
**Input:** User task description
**Output:** Goal ID with contract, primed context, workspace checkpoint, security baseline

1. **Health check:** `sin-code doctor` — verify Go, config, DBs, MCP, tools, CGO, module-path
   - If any check fails: report, ask user to fix before proceeding
2. **Config validation:** `sin-code config validate` — ensure pipeline config is valid
   - Check: `fusion.enabled`, `test.auto_generate`, `agentloop.context_compaction`, `output.progress`
3. **Lessons query:** Query lessons DB for `TypeFailedVerification` matching the task domain
   - Brief agent: "Past failures to avoid: ..."
4. **Instinct + RAG:** Query instinct store for relevant behavioral patterns
   - RAG selects top-5 most relevant active instincts for the task domain
   - More granular than lessons — per-function, per-pattern learned behaviors
5. **Memory prime:** `sin-code memory prime --query "<task>"` — inject top-K relevant memories
6. **Context bridge:** `sin-context-bridge` — unified query across SCKG + sin-brain + GitNexus + local SQLite in 1 call
   - Gathers all available context sources in one shot
7. **Decision memory:** `sin-code decision list --query "<task>"` — check prior architectural decisions
8. **Triage (if issue-based):** `sin-code triage` — if task originates from a GitHub issue, get priority score
   - Feed triage score into goal priority
9. **Rules loading:** Load `.sin-code/rules/` path-scoped rules
   - Rules scoped to specific path patterns (e.g. `cmd/sin-code/internal/agentloop/**`)
   - Will be injected into subagent prompts based on their file assignments
10. **Autolevel:** Classify task into permission mode (plan/act/yolo) based on prompt regex
    - Determines appropriate permission level for the pipeline run
11. **Agent mode:** Determine specialized mode for subagents (architect/code/review/debug)
12. **Sandbox verification:** Verify OS-level sandbox is available (landlock/seatbelt/bubblewrap)
    - Pipeline runs subagents in headless mode → sandbox is mandatory (M3/M4)
13. **Egress check:** Verify SSRF allowlist is active (blocks private IP ranges)
14. **LSP detect:** `sin-code lsp-config detect` — verify LSP servers for project languages
    - Enables `sin_edit` structural edits and `sin_scout` symbol search
15. **MCP discover:** `sin-code mcp-install discover` — check if required MCP servers are installed
16. **Profile verify:** `sin-code profile verify` — ensure agent profiles are in sync
17. **GitHub auth:** `sin-code gh doctor` — verify GitHub authentication for Stage 7/9
18. **Goal enqueue:** `sin-code goal add --prompt "<task>" --priority P1 --criteria "tests pass" --criteria "build clean" --criteria "no TODO/FIXME"`
    - Record goal ID for tracking throughout pipeline
    - GoalContract activates stop-gate in Stage 6
19. **Agentteams setup:** Initialize file-locked mailbox for inter-agent communication
    - Subagents in same wave can share interface contracts and status
20. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-0"`
21. **Hooks:** Fire `session.start` and `goal.enqueued`
22. **Telemetry:** Enable `output.progress = "stderr"` for real-time NDJSON progress events

**Gate:** Doctor passes, goal ID recorded, checkpoint created, sandbox active, rules loaded.

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

**Skills:** `plan v2`, `skill-multimodal-web-tools`, `skill-code-spec`
**Systems:** research, web search (M8), spec-driven, cartographer, episodic, plan-merge, compile-spec, modelperf, codegraph, vane
**Input:** Grill decisions + task description
**Output:** Execution-ready plan with phases, tasks, risks, done criteria

1. **Research (if needed):** `sin-code research` — autonomous research report for domain context
2. **Web search (M8 mandate):** `sin_web_search` — look up library docs, API references, community solutions
   - Use `context7` for library documentation (Layer 5 of web tools)
   - Use `skill-multimodal-web-tools` as the master research skill
3. **Spec-driven (alternative entry):** If user provides EARS spec:
   - `sin-code spec-driven parse` — extract requirements from EARS syntax
   - `sin-code spec-driven arch` — generate architecture from requirements
   - Feed both into plan
4. **Spec authoring (if no spec):** `sin-code spec` — self-authoring spec layer
   - `*.spec.md` contracts with verify directives
5. **Compile-spec:** If `.sin-code.yml` exists: `sin-code compile-spec` — ensure derived files in sync
6. **Cartographer index:** `sin-code sckg build` — build semantic code graph
   - Query: `sin-code sckg query "<task domain>"` — find hot paths, entry points
   - For non-Go languages: `sin-code codegraph` (multi-language static analysis)
7. **Episodic memory:** Query orchestrator episodic memory for similar past plans
8. **Modelperf:** `sin-code fusion recommend` — get model recommendations for task category
   - Sort providers: recommended first, then rest
9. **Plan v2:**
   - Simple: `plan --lite` (6 stages, S/M/L estimates)
   - Complex: full plan v2 (13 stages, PERT, Monte Carlo, OKRs, risk scoring)
   - From spec: `plan --from-spec` (load spec, decompose to tasks)
10. **Plan-Merge (complex tasks):** If `fusion.enabled = true` and task is complex:
    - N planners generate plans in parallel → judge merges best insights → 1 coder executes
11. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-2"`

**Gate:** Plan has phases, tasks with validation, done criteria, risks. Web search performed (M8).

---

### Stage 3: GSD (Project Lifecycle)

**Skill:** `skill-process-gsd`
**CLI:** `sin-code gsd`
**Input:** Approved plan
**Output:** `.gsd/` state with phases, plans, goal subtasks

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

**Skills:** `delegate-subagents`, `skill-code-build`, `skill-debug-deep` (on failure)
**Systems:** testgate, testgen, worktree, conflict prediction, fusion, critic, governor, agentteams, autolevel, background, telemetry, compaction, ledger, rules, sandbox, egress, rtk, circuitbreaker, watch
**Input:** Phase plan with waves
**Output:** Code changes, test results

1. `sin-code gsd execute $PHASE_ID` — analyze waves (topological sort)
2. **Conflict prediction:** `sin-code goal predict-conflicts` before wave execution
   - If conflicts predicted: reorder tasks or assign non-overlapping files
3. **For each wave (dependency-ordered):**
   a. **Worktree isolation (optional):** `sin-code goal worktree create` per parallel task
      - Each subagent works in isolated git worktree — true parallelism, no file conflicts
   b. **Autolevel + agent mode:** Assign specialized mode per subagent:
      - `architect` for planning, `code` for implementation, `review` for review, `debug` for debugging
   c. **Rules injection:** Each subagent gets path-scoped rules for its assigned files
   d. **Agentteams:** Subagents in same wave share mailbox for interface contracts
   e. Launch all tasks in wave as parallel subagents (`delegate-subagents` + `skill-code-build`)
      - Each subagent gets: full context, file assignment, validation criteria, path-scoped rules
      - Set `required_tools` per subagent (tool coverage enforcement)
      - Session context injection: lessons + instinct + memory primed for each subagent
      - Sandbox + egress filtering active per subagent (M3/M4)
   f. **Test-First (per task):** Each subagent runs after writing code:
      - `sin_test` — run tests with `-race -cover`, structured JSON output
      - `sin_quality_gate` — pipeline: build → vet → test → staticcheck → gosec → govulncheck
      - `sin_test_generate` — auto-generate tests when `test.auto_generate = true`
   g. Wait for all subagents in wave to complete
   h. **Verify each result:** `sin_quality_gate` per task
   i. **On verify.fail:**
      - **Fusion tournament:** fan task to N providers, first to pass verify-gate wins (PoC) or judge evaluates all (Oracle)
      - **Critic repair:** bounded verify-diagnose-retry loop (max 3 iterations per task)
      - **Governor escalation:** single-shot → repair → best-of-N
      - **Debug-deep:** `skill-debug-deep` for persistent failures — facts-first RCA, parallel subagents
   j. **Loop detection:** Monitor for stuck subagents (repeated identical tool calls)
   k. **Watch:** `sin-code watch` — detect file conflicts during parallel execution
   l. `sin-code gsd execute $PHASE_ID --task $TASK_ID --status done`
4. **Background tasks:** Non-critical work (docs generation, benchmarks, SBOMs) via `sin-code background`
   - Fire-and-forget, check results in Stage 8
5. **Context compaction:** Monitor with `sin-code context`
   - If >80%: deterministic compaction (preserve verification evidence)
   - If >90%: LLM compaction (summarize older turns)
6. **RTK:** Wrap long command outputs through `sin-code rtk` to cut token usage 60-90%
7. **Telemetry:** NDJSON progress events (`turn.start`, `tool.pre/post`, `verify.pass/fail`) to stderr
8. After all waves: full `sin_quality_gate` (build + test + vet + staticcheck + gosec + govulncheck)
9. **Ledger:** Every tool call, verify result, fusion dispatch auto-records
10. **Hooks:** Fire `task.complete` per wave, `fusion.dispatch` if fusion used
11. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-4"`

**Gate:** All tasks done, `sin_quality_gate` passes, tests pass, vet clean.

---

### Stage 5: REVIEW (Multi-Layer Quality Review)

**Skills:** `self-review`, `skill-code-codocs`
**Systems:** adversary, complexity, ceo-audit, IBD, security, codocs, dox, coverdrohne, debt
**Input:** All code changes (git diff)
**Output:** Review report with findings (BLOCKER/MAJOR/MINOR/NIT)

1. **Self-review (CEO — MANDATORY):** Load `self-review` skill
   - Reconstruct scope from original task + grill decisions + plan
   - Requirement check: every requirement covered?
   - File-by-file check: read every changed file
   - Run `sin_quality_gate` (build + test + vet + staticcheck + gosec + govulncheck)
   - Severity: BLOCKER / MAJOR / MINOR / NIT
2. **Adversary probes:** `sin-code orchestrate --adversary`
   - Proposes executable attacks on the change; survivors become regression tests
3. **Complexity review:** `sin-code review --complexity`
   - Ponytail 5-tag findings: delete / simplify / rebuild / risk / verify
4. **Debt check:** `sin-code debt stats` — check for sin-debt markers in changed code
5. **CEO-Audit (optional, large changes):** `sin-code ceo-audit --profile QUICK`
   - 48-gate quality audit (security, performance, code quality, deps, tests, docs, compliance)
6. **Intent-Based Diffing:** `sin-code ibd --before pipeline-stage-2 --after HEAD`
   - Compare actual changes against stated intent — flag scope creep or missing changes
7. **Security scan:** `sin-code security scan --path .` — secrets/SAST/SCA/SBOM/container
8. **CoDocs validation:** Check that all changed/new files have `.doc.md` companions
   - Validate that CoDocs references resolve
   - Missing companions = MAJOR finding
9. **Dox tree validation:** `sin-code dox tree` — ensure AGENTS.md hierarchy is healthy
   - Detect broken links, orphans, TODO markers in the hierarchy
10. **Coverage scan:** `sin-code cover scan` — identify coverage gaps in changed code
    - `sin-code cover gaps` — list uncovered functions
    - Optionally `sin-code cover generate` — auto-generate missing tests
11. **Fix all BLOCKER + MAJOR immediately**, re-verify after each fix
12. **Checkpoint:** `sin-code checkpoint create --name "pipeline-stage-5"`

**Gate:** 0 open BLOCKER, 0 open MAJOR, security clean, CoDocs complete, dox tree healthy.

---

### Stage 6: DONE GATE (Machine Verification — M3 SACRED)

**Systems:** stop-gate, GoalContract, testgate, mutation, fuzz, property, PoC, Oracle, ADW, SCKG, compile-spec, cover check, dodone
**Input:** Final codebase state + GoalContract from Stage 0
**Output:** Exit 0 (WIRKLICH FERTIG) or Exit 2/3 (back to Stage 4)

1. **Stop-Gate with GoalContract:**
   - Deterministic checks first (fail-closed: ANY failed check blocks):
     - `sin_quality_gate`: build → vet → test → staticcheck → gosec → govulncheck
     - Predicates: every `--criteria` from Stage 0 GoalContract evaluated
     - Diff-scope: changes stay within allowed globs
   - Then LLM judge (`SIN_EVALUATOR_MODEL`) for semantic criteria
   - Green judge can NEVER override a red deterministic check
2. **Test-First deep verification:**
   - `sin_mutation` — mutation testing (wrap `gremlins unleash` if present)
   - `sin_fuzz` — run `go test -fuzz` targets
   - `sin_property` — property-based tests (`rapid` / `testing/quick`)
3. **Coverage gate:** `sin-code cover check` — fail if coverage drops below threshold
4. **Compile-spec check:** `sin-code compile-spec --check` — fail if derived files out of sync
5. **PoC verification:** `sin-code poc verify` — proof-of-correctness for plan invariants
6. **Oracle check:** `sin-code oracle check` — independent LLM judge with evidence
7. **ADW scan:** `sin-code adw scan` — architectural debt watchdogs (god modules, circular deps)
8. **SCKG dead code:** `sin-code sckg dead_code` — semantic dead code detection
9. **DoDone fallback:** `dodone check` — 11-pillar deterministic check
10. **Interpret exit codes:**
    - Exit 0: WIRKLICH FERTIG → proceed to Stage 7
    - Exit 2: Code incomplete → feed findings to Stage 4, fix, re-run 5+6
    - Exit 3: Tests/Build failed → fix, re-run 5+6
11. **Hooks:** Fire `verify.pass` or `verify.fail`

**Gate:** Exit 0. No exceptions. Lying about the gate is M3 violation. Max 3 loop-backs.

---

### Stage 7: COMMIT (Auto-Commit + Auto-PR)

**Systems:** auto-commit, auto-PR, gh-execute
**Input:** Verified codebase (Stage 6 exit 0)
**Output:** Git commit + optional PR

1. **Auto-commit:** Conventional commit with auto-detected prefix
   - Message includes: goal ID, pipeline stages passed, verification evidence
   - Example: `feat: add user auth (goal #42, pipeline: 10/10 stages, dodone exit 0)`
2. **Auto-PR:** `sin-code auto-pr` — creates GitHub PR with plan summary, review findings, verification results
   - Or: `sin-code gh execute` for mutating GitHub operations
3. **GSD phase update:** `sin-code gsd phase edit $PHASE_ID --status completed`
4. **Goal update:** `sin-code goal complete $GOAL_ID`
5. **Hooks:** Fire `commit.post`, `push.pre`

**Gate:** Commit created, optionally PR opened.

---

### Stage 8: RECORD (Learning + Memory + Cleanup — MANDATORY)

**Systems:** ledger, summary, lessons, instinct, memory, compress, decision, tokens, imagegraph, share, notifications, modelperf, background results
**Input:** Completed pipeline run
**Output:** Persisted learnings, session summary, compressed DBs, shareable report

1. **Ledger finalization:** `sin-code ledger list --session $SESSION_ID`
   - Full audit trail; `sin-code ledger tools` — tool usage heatmap
2. **Session summary:** `sin-code summary` — deterministic, rule-based (no LLM)
   - Tokens used, cost, tools invoked, verification status, stages passed
3. **Token cost:** `sin-code tokens cost` — project total cost + budget alerts
4. **Lessons recording:**
   - Record `TypeFailedVerification` from Stage 4/6 failures
   - Record `TypeToolError` from any tool failures
   - These brief the next pipeline run (Stage 0) to avoid repeating mistakes
5. **Instinct recording:** Record new instincts from the pipeline run
   - Reinforce or contradict existing instincts based on outcomes
   - RAG will retrieve these in future runs
6. **Memory store:** `sin-code memory add --insight "<key learning>" --tags pipeline`
7. **Compress lessons + instincts:** `sin-code compress plan --target lessons && sin-code compress apply`
   - Deduplicate, byte-budget, sort — keep DB lean for fast Stage 0 queries
8. **Decision memory:** Update decisions with outcomes
9. **Modelperf data:** Record model performance from the pipeline run
   - `sin-code fusion benchmark` — updates modelperf.db for future recommendations
10. **Background results:** Check `sin-code background list` for async job results
11. **Imagegraph (optional):** `sin-code image-graph` — generate charts for pipeline report
    - Token usage, cost breakdown, coverage trends, test results over time
12. **Share (optional):** `sin-code share export --format html` — shareable HTML report
13. **Notifications:** `sin-code notifications list` — check for unread pipeline-relevant notifications

**Gate:** Lessons + instincts recorded, memory updated, summary generated, compress done.

---

### Stage 9: CI/CD (GitHub Actions + Continuous Quality)

**Skills:** `sin-git-workflow`, `skill-github-actions`
**Systems:** GitHub Actions, SBOM, dox, profile, eval, benchmark
**Input:** Pushed commit/PR from Stage 7
**Output:** CI pipeline running, quality gates enforced

1. **GitHub Actions:** Push triggers CI
   - CEO-Audit workflow (M1: n8n delegator)
   - Ecosystem-sync workflow (registry/permission/ECOSYSTEM drift)
   - Release workflow (if tagged)
2. **Monitor CI:** `gh run watch` or `sin-code gh run`
   - If CI fails: `sin-code auto-pr` self-healing kicks in
   - Or: feed failures back to Stage 4, fix, re-push
3. **SBOM:** `sin-code sbom generate --format spdx-json` — attach to release/PR
4. **Dox tree:** `sin-code dox tree` — verify AGENTS.md hierarchy in CI
5. **Profile verify:** `sin-code profile verify` — ensure agent profiles in sync
6. **Eval gate (optional):** `sin-code eval run --dataset evals/critical.json --min-pass-rate 0.95`
   - Golden dataset regression gate
7. **Benchmark (optional):** `sin-code benchmark` — run eval golden datasets with scoring report
8. **Documentation sync:** Update CHANGELOG.md, README.md if behavioral changes
9. **Wiki sync (if applicable):** Sync plan to GitHub Wiki

**Gate:** CI passes (or explicitly waived by user).

---

## Failure Handling

| Stage | Failure | Action | Max Retries |
|---|---|---|---|
| 0 Pre-flight | Doctor fails | Report, user fixes | N/A |
| 1 Grill | Unresolved design | User decides, proceed | N/A |
| 2 Plan | Review rejects | 1 revision, then user decides | 1 |
| 3 GSD | Phase conflict | User resolves, re-init | N/A |
| 4 Execute | Subagent fails | Re-launch + critic + fusion + debug-deep | 3 per task |
| 4 Execute | Loop detected | Abort, relaunch with tighter prompt | 2 |
| 4 Execute | Context overflow | Deterministic/LLM compaction | automatic |
| 4 Execute | File conflict | Watch detects, agentteams resolves | 2 |
| 5 Review | BLOCKER found | Fix immediately, re-review | until 0 |
| 5 Review | CoDocs missing | Generate companions | 1 |
| 5 Review | Dox tree broken | Fix hierarchy | 1 |
| 6 Done Gate | Exit 2/3 | Feed findings to Stage 4, fix, re-run 5+6 | 3 total |
| 7 Commit | Commit fails | Manual commit | 1 |
| 9 CI/CD | CI fails | Auto-PR self-healing or manual fix | 2 |

**Rewind:** `sin-code checkpoint rewind --name "pipeline-stage-N"` at any stage.

---

## Cost Awareness

| Stage | LLM Calls | Duration | Notes |
|---|---|---|---|
| 0 Pre-flight | 0-1 | <30s | CLI + memory + instinct queries |
| 1 Grill | 3-10 | 1-3 min | Interactive with user |
| 2 Plan | 4-12 | 2-10 min | + research + web search + plan-merge |
| 3 GSD | 0 | <1s | Pure CLI |
| 4 Execute | 5-50+ | 5-60 min | Fusion adds N×cost; testgate per task |
| 5 Review | 2-5 | 2-10 min | ceo-audit adds ~3 min; cover scan adds ~1 min |
| 6 Done Gate | 0-2 | <30s | Mostly deterministic; mutation/fuzz add time |
| 7 Commit | 0 | <1s | Git operations |
| 8 Record | 0-1 | <10s | CLI + compress + imagegraph |
| 9 CI/CD | 0 | 2-10 min | External CI runners |
| **Total** | **14-80+** | **15-100 min** | Fusion + research + ceo-audit add most cost |

---

## Degradation Rules

| System | Degrades to | Behavior |
|---|---|---|
| `sin-code doctor` | Skip | Proceed without health check |
| `sin-code memory` / `instinct` | Skip | No primed context |
| `sin-context-bridge` | Individual queries | Query each source separately |
| `sin-code fusion` | Single-model retry | No tournament |
| `sin-code sckg` / `codegraph` | Manual file reads | No semantic graph |
| `sin-code orchestrator` | Manual delegation | No critic/adversary/governor |
| `sin-code research` | `sin_web_search` | Basic web search instead of report |
| `sin-code ceo-audit` | self-review only | No 48-gate audit |
| `sin-code security` | Skip | No security scan (warned) |
| `sin-code codocs` | Skip | No doc companion check (warned) |
| `sin-code dox` | Skip | No AGENTS.md tree check (warned) |
| `sin-code cover` | Skip | No coverage gate (warned) |
| `sin-code poc/oracle` | dodone check only | No PoC/Oracle verification |
| `sin-code adw` | Skip | No architecture debt scan |
| `sin-code compile-spec` | Skip | No spec sync check |
| `sin-code ledger` | Skip | No audit trail (warned) |
| `sin-code compress` | Skip | No lesson compression (DB grows) |
| `sin-code imagegraph` | Skip | No visual charts |
| `sin-code share` | Skip | No HTML report |
| `sin-code rtk` | Raw output | Higher token consumption |
| `sin-code background` | Synchronous | Blocks pipeline on non-critical work |
| `sin-code watch` | Manual check | No reactive conflict detection |
| `sin-git-workflow` | Skip | No CI/CD |
| **stop-gate / dodone** | **NEVER SKIP** | **M3 mandate — machine gate is sacred** |
| **self-review** | **NEVER SKIP** | **CEO review is mandatory** |
| **testgate (sin_quality_gate)** | **NEVER SKIP** | **Build+test+vet is mandatory** |
| **sandbox + egress** | **NEVER SKIP** | **M3/M4 mandate — security baseline** |
| **web search (M8)** | **NEVER SKIP** | **M8 mandate — context7 + web search before coding** |

---

## Integration Map

```
                         ┌──────────────────────┐
                         │     STAGE 0          │
                         │    PRE-FLIGHT        │
                         │  doctor + config     │
                         │  lessons + instinct  │
                         │  memory + context    │
                         │  triage + rules      │
                         │  sandbox + egress    │
                         │  lsp + mcp + profile │
                         │  goal + contract     │
                         │  autolevel + checkpoint│
                         │  agentteams + gh     │
                         │  telemetry ON        │
                         └─────────┬────────────┘
                                   │
                         ┌─────────▼────────────┐
                         │     STAGE 1          │
                         │    GRILL             │
                         │  grill-me            │
                         │  decision memory     │
                         └─────────┬────────────┘
                                   │
                         ┌─────────▼────────────┐
                         │     STAGE 2          │
                         │    PLAN              │
                         │  research + web (M8) │
                         │  spec-driven         │
                         │  cartographer (sckg) │
                         │  episodic memory     │
                         │  plan-merge (fusion) │
                         │  compile-spec        │
                         │  modelperf           │
                         │  plan v2             │
                         └─────────┬────────────┘
                                   │
                         ┌─────────▼────────────┐
                         │     STAGE 3          │
                         │    GSD               │
                         │  gsd init + phases   │
                         │  goal subtasks       │
                         └─────────┬────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │        STAGE 4              │
                    │       EXECUTE               │
                    │  delegate + skill-code-build│
                    │  testgate + testgen         │
                    │  worktree + conflict pred   │
                    │  fusion + critic + governor │
                    │  agentteams + autolevel     │
                    │  background + telemetry     │
                    │  compaction + ledger        │
                    │  rules + sandbox + egress   │
                    │  rtk + circuitbreaker       │
                    │  watch + debug-deep (fail)  │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │        STAGE 5              │
                    │       REVIEW                │
                    │  self-review (MANDATORY)    │
                    │  adversary probes           │
                    │  complexity + debt          │
                    │  ceo-audit (48 gates)       │
                    │  IBD (intent-based diff)    │
                    │  security scan              │
                    │  codocs validation          │
                    │  dox tree validation        │
                    │  coverdrohne (coverage)     │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │     STAGE 6 (M3 SACRED)     │
                    │       DONE GATE             │
                    │  stop-gate + GoalContract   │
                    │  testgate (quality pipeline)│
                    │  mutation + fuzz + property │
                    │  cover check (threshold)    │
                    │  compile-spec --check       │
                    │  PoC + Oracle               │
                    │  ADW + SCKG dead code       │
                    │  dodone (11-pillar fallback)│
                    └──────────────┬──────────────┘
                              exit 0 only
                                   │
                    ┌──────────────▼──────────────┐
                    │        STAGE 7              │
                    │       COMMIT                │
                    │  auto-commit + auto-PR      │
                    │  gsd complete + goal done   │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │        STAGE 8 (MANDATORY)  │
                    │       RECORD                │
                    │  ledger + summary           │
                    │  lessons + instinct record  │
                    │  memory + compress          │
                    │  tokens + imagegraph        │
                    │  share + notifications      │
                    │  modelperf + background     │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │        STAGE 9              │
                    │       CI/CD                 │
                    │  GitHub Actions             │
                    │  SBOM + dox + profile       │
                    │  eval + benchmark           │
                    │  docs sync                  │
                    └─────────────────────────────┘
```

## Verification Checklist

- [ ] Stage 0: Doctor passes, goal ID recorded, checkpoint created, sandbox active, rules loaded, telemetry ON
- [ ] Stage 1: Grill synthesized (or explicitly skipped with note)
- [ ] Stage 2: Plan has phases/tasks/done-criteria/risks, web search performed (M8), compile-spec in sync
- [ ] Stage 3: GSD status shows all phases with plans, goal subtasks enqueued
- [ ] Stage 4: All tasks done, `sin_quality_gate` passes, ledger recorded, no stuck loops
- [ ] Stage 5: 0 BLOCKER, 0 MAJOR, security clean, CoDocs complete, dox tree healthy, coverage gaps identified
- [ ] Stage 6: Stop-gate exit 0, coverage threshold met, compile-spec in sync, mutation/fuzz pass
- [ ] Stage 7: Commit created, goal completed
- [ ] Stage 8: Lessons + instincts recorded, summary generated, compress done, cost projected
- [ ] Stage 9: CI passes (or waived), SBOM generated, dox + profile verified
