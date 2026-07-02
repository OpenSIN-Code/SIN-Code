# Pipeline Context — SIN-Code Full Stack Integration (10 Stages, v3.29.2)

## Systems Integrated (~60% ecosystem coverage, 55+ systems)

| System | Stage | Role |
|---|---|---|
| `sin-code doctor` | 0 | Health check |
| `sin-code config validate` | 0 | Config validation |
| `sin-code memory` | 0, 8 | Prime context, store learnings |
| `sin-code instinct` + RAG | 0, 8 | Behavioral patterns (per-function, per-pattern) |
| `sin-context-bridge` | 0 | Unified SCKG+sin-brain+GitNexus+SQLite query |
| `sin-code decision` | 0, 1, 8 | Query/record/update decisions |
| `sin-code triage` | 0 | Issue prioritization (if issue-based) |
| `sin-code rules` | 0, 4 | Path-scoped rule loader |
| `sin-code sandbox` | 0, 4 | OS-level isolation (M3/M4) |
| `sin-code egress` | 0, 4 | SSRF allowlist filtering |
| `sin-code lsp-config` | 0 | LSP server detection |
| `sin-code mcp-install` | 0 | MCP server discovery |
| `sin-code profile` | 0, 9 | Agent profile verify |
| `sin-code gh doctor` | 0 | GitHub auth verification |
| `sin-code goal` | 0, 3, 7 | Enqueue/subtask/complete |
| `sin-code goalcontract` | 0, 6 | Contract → stop-gate enforcement |
| `sin-code autolevel` | 0, 4 | Permission mode classification |
| `sin-code agentteams` | 0, 4 | Inter-agent mailbox |
| `sin-code checkpoint` | 0-5 | Snapshot before each stage |
| `sin-code telemetry` | 0-8 | NDJSON progress events |
| `grill-me` | 1 | Adversarial design interview |
| `plan v2` | 2 | Research, draft, review, quality score |
| `sin-code research` | 2 | Autonomous research report |
| `sin_web_search` | 2 | Web search (M8 mandate) |
| `skill-multimodal-web-tools` | 2 | Master research skill (5-layer) |
| `context7` | 2 | Library documentation lookup |
| `sin-code spec-driven` | 2 | EARS parser → architecture |
| `sin-code spec` | 2 | Self-authoring spec layer |
| `sin-code compile-spec` | 2, 6 | Declarative spec compiler + check |
| `sin-code sckg` | 2, 6 | Cartographer index, dead code |
| `sin-code codegraph` | 2 | Multi-language static analysis |
| `sin-code orchestrator` | 2, 4, 5 | Episodic, DAG, adversary |
| `sin-code fusion` | 2, 4 | Plan-merge, verify tournament |
| `sin-code modelperf` | 2, 8 | Model recommendations, performance data |
| `sin-code vane` | 2 | Research bridge (if available) |
| `sin-code gsd` | 3, 4, 7 | Phases, waves, complete |
| `delegate-subagents` | 4 | Parallel subagent execution |
| `skill-code-build` | 4 | Implementation skill |
| `sin_test` / `sin_quality_gate` | 4, 6 | Test-first verify loop |
| `sin_test_generate` | 4 | Auto-generate tests |
| `sin-code goal worktree` | 4 | Git worktree isolation |
| `sin-code goal predict-conflicts` | 4 | File conflict prediction |
| `sin-code orchestrator critic` | 4 | Verify-diagnose-repair loop |
| `sin-code orchestrator governor` | 4 | Escalating compute ladder |
| `skill-debug-deep` | 4 | Enterprise debugging on failure |
| `sin-code background` | 4, 8 | Async fire-and-forget jobs |
| `sin-code context` | 4 | Context window monitoring |
| `sin-code compress` | 4, 8 | Compaction, lesson compress |
| `sin-code rtk` | 4 | Token optimization (60-90% cut) |
| `sin-code watch` | 4 | File conflict detection |
| `sin-code ledger` | 4-8 | Audit trail, tool heatmaps |
| `self-review` | 5 | CEO-grade review (MANDATORY) |
| `sin-code review --complexity` | 5 | Ponytail 5-tag findings |
| `sin-code debt` | 5 | sin-debt marker check |
| `sin-code ceo-audit` | 5 | 48-gate quality audit |
| `sin-code ibd` | 5 | Intent-based diffing |
| `sin-code security` | 5, 6 | Secrets/SAST/SCA/SBOM/container |
| `sin-code codocs` | 5 | Documentation companion validation |
| `sin-code dox` | 5, 9 | AGENTS.md hierarchy validation |
| `sin-code cover` | 5, 6 | Coverage scan, gaps, check, generate |
| `sin-code stopgate` | 6 | Deterministic + LLM judge |
| `sin_mutation` | 6 | Mutation testing |
| `sin_fuzz` | 6 | Fuzz testing |
| `sin_property` | 6 | Property-based testing |
| `sin-code poc` | 6 | Proof-of-correctness |
| `sin-code oracle` | 6 | Independent verification |
| `sin-code adw` | 6 | Architectural debt watchdogs |
| `skill-process-dodone` | 6 | 11-pillar deterministic fallback |
| `sin-code auto-pr` | 7 | Self-healing PR creation |
| `sin-code summary` | 8 | Deterministic session summary |
| `sin-code tokens` | 8 | Cost projection + budget alerts |
| `sin-code imagegraph` | 8 | Visual charts for report |
| `sin-code share` | 8 | HTML report export |
| `sin-code notifications` | 8 | Unread notification check |
| `sin-git-workflow` | 9 | GitHub Actions deployment |
| `sin-code sbom` | 9 | Software Bill of Materials |
| `sin-code eval` | 9 | Golden dataset regression gate |
| `sin-code benchmark` | 9 | Eval scoring report |

## NEVER-SKIP Systems (Hard Gates)

| System | Mandate | Reason |
|---|---|---|
| stop-gate / dodone | M3 | Verification gate is sacred |
| self-review | — | CEO review is mandatory |
| sin_quality_gate | — | Build+test+vet is mandatory |
| sandbox + egress | M3/M4 | Security baseline |
| web search (M8) | M8 | context7 + web search before coding |

## Loop-Back Logic

```
Stage 6 fails (exit 2/3)
  → findings fed back to Stage 4 (Execute)
  → fix issues (critic repair, debug-deep, fusion)
  → re-run Stage 5 (Review) + Stage 6 (Done Gate)
  → max 3 loop-back iterations
  → if still failing: escalate to user
```

## Lightweight Alternatives

| Alternative | When to use | Scope |
|---|---|---|
| `sin-code prp` | Simple structured task | draft→planned→implementing→verifying→ready→shipped |
| `sin-code auto` | Metric-driven optimization | observe→propose→act→verify→measure→keep/revert→learn |
| `plan v2 --lite` | Quick plan + execute | 6-stage flow without ceremony |
| `dodone check` | Just need the gate | 11-pillar deterministic check |
