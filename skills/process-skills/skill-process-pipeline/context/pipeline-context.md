# Pipeline Context — SIN-Code Full Stack Integration (10 Stages)

## Skill Chain

```
Stage 0: PRE-FLIGHT    — doctor, lessons, memory, goal+contract, checkpoint
Stage 1: GRILL         — grill-me, decision memory
Stage 2: PLAN          — plan v2, cartographer, episodic memory, plan-merge
Stage 3: GSD           — gsd init, phase add, goal subtasks
Stage 4: EXECUTE       — delegate-subagents, worktree, fusion, critic, governor, compaction, ledger
Stage 5: REVIEW        — self-review, adversary, complexity, ceo-audit, IBD, security
Stage 6: DONE GATE     — stop-gate, GoalContract, PoC, Oracle, ADW, SCKG, dodone
Stage 7: COMMIT        — auto-commit, auto-PR, gsd complete, goal complete
Stage 8: RECORD        — ledger, summary, lessons, memory, compress
Stage 9: CI/CD         — GitHub Actions, SBOM, docs sync
```

## Systems Integrated (~40% ecosystem coverage)

| System | Stage | Role |
|---|---|---|
| `sin-code doctor` | 0 | Health check before starting |
| `sin-code memory` | 0, 8 | Prime context (0), store learnings (8) |
| `sin-code decision` | 0, 1, 8 | Query decisions (0), record (1), update outcomes (8) |
| `sin-code goal` | 0, 3, 7 | Enqueue with contract (0), subtasks (3), complete (7) |
| `sin-code goalcontract` | 0, 6 | Contract enqueued (0), enforced by stop-gate (6) |
| `sin-code checkpoint` | 0-5 | Snapshot before each stage, rewind on failure |
| `grill-me` | 1 | Adversarial design interview |
| `plan v2` | 2 | Research, draft, review, quality score |
| `sin-code sckg` | 2, 6 | Cartographer index (2), dead code detection (6) |
| `sin-code orchestrator` | 2, 4, 5 | Episodic memory (2), DAG dispatch (4), adversary (5) |
| `sin-code fusion` | 2, 4 | Plan-merge (2), verify tournament on fail (4) |
| `sin-code gsd` | 3, 4, 7 | Phases (3), wave execution (4), phase complete (7) |
| `delegate-subagents` | 4 | Parallel subagent execution per wave |
| `sin-code goal worktree` | 4 | Git worktree isolation for parallel tasks |
| `sin-code goal predict-conflicts` | 4 | Predict file conflicts before wave execution |
| `sin-code orchestrator critic` | 4 | Bounded verify-diagnose-repair loop |
| `sin-code orchestrator governor` | 4 | Escalating compute: single-shot → repair → best-of-N |
| `sin-code context` | 4 | Context window monitoring |
| `sin-code compress` | 4, 8 | Compaction during execution (4), lessons compress (8) |
| `sin-code ledger` | 4-8 | Audit trail, tool heatmaps, finalization |
| `self-review` | 5 | CEO-grade evidence-driven review |
| `sin-code review --complexity` | 5 | Ponytail 5-tag complexity findings |
| `sin-code ceo-audit` | 5 | 48-gate quality audit |
| `sin-code ibd` | 5 | Intent-based diffing (changes vs plan) |
| `sin-code security` | 5, 6 | Secrets/SAST/SCA/SBOM/container scan |
| `sin-code debt` | 5 | sin-debt marker check |
| `sin-code stopgate` | 6 | Deterministic + LLM judge completion gate |
| `sin-code poc` | 6 | Proof-of-correctness verification |
| `sin-code oracle` | 6 | Independent verification with evidence |
| `sin-code adw` | 6 | Architectural debt watchdogs |
| `skill-process-dodone` | 6 | 11-pillar deterministic check (fallback) |
| `sin-code auto-pr` | 7 | Self-healing PR creation |
| `sin-code summary` | 8 | Deterministic session summary |
| `sin-code lessons` | 0, 4, 8 | Query (0), record failures (4), compress (8) |
| `sin-git-workflow` | 9 | GitHub Actions deployment |
| `sin-code sbom` | 9 | Software Bill of Materials |

## Loop-Back Logic

```
Stage 6 fails (exit 2/3)
  → findings fed back to Stage 4 (Execute)
  → fix issues
  → re-run Stage 5 (Review) + Stage 6 (Done Gate)
  → max 3 loop-back iterations
  → if still failing after 3: escalate to user
```

## Checkpoint Strategy

| Checkpoint | Created | Rewind use case |
|---|---|---|
| `pipeline-stage-0` | Before any code changes | Catastrophic failure — start over |
| `pipeline-stage-2` | After plan approved | Bad plan execution — re-plan |
| `pipeline-stage-3` | After GSD setup | GSD corruption — re-init phases |
| `pipeline-stage-4` | After all code written | Review/verify failure — keep code, re-verify |
| `pipeline-stage-5` | After review passes | DoDone failure — keep review, re-check |

## Degradation (when systems are unavailable)

| System | Degrades to | Behavior |
|---|---|---|
| `sin-code doctor` | Skip | Proceed without health check |
| `sin-code memory` | Skip | No primed context |
| `sin-code fusion` | Single-model retry | No tournament, same model retries |
| `sin-code sckg` | Manual file reads | No semantic graph |
| `sin-code orchestrator` | Manual delegation | No critic/adversary/governor |
| `sin-code ceo-audit` | self-review only | No 48-gate audit |
| `sin-code security` | Skip | No security scan |
| `sin-code poc/oracle` | dodone check only | No PoC/Oracle verification |
| `sin-code adw` | Skip | No architecture debt scan |
| `sin-code ledger` | Skip | No audit trail (warned) |
| `sin-code compress` | Skip | No lesson compression (DB grows) |
| `sin-git-workflow` | Skip | No CI/CD |
| **dodone / stop-gate** | **NEVER SKIP** | **M3 mandate — machine gate is sacred** |
| **self-review** | **NEVER SKIP** | **CEO review is mandatory** |
