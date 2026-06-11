# agent_engine — SIN Python Agent Subsystem

Autonome Plan/Execute/Verify/Repair-Engine, pure stdlib, drop-in für
`sin-code-bundle`. 16 Module + 2 Test-Suites = 34 grüne Tests.

## Schichten (unten → oben)

| Schicht | Datei | Zweck |
|---|---|---|
| **Daten** | `types.py` | `AgentTask`, `Plan`, `Step`, `StepResult`, `Verdict` |
| **Planung** | `planner.py` | DAG-Build + Critical-Path + Failure-Propagation |
| **Tool-Dispatch** | `router.py` | Per-Tool Circuit-Breaker + Decorrelated-Jitter-Backoff |
| **Ausführung** | `executor.py` | Parallel-Worktree-Executor, Write-Fence, On-Step-Terminal-Hook |
| **Verifikation** | `verifier.py` | 4-Stages: ADW → Semantic-Diff → Lint → Tests |
| **Telemetrie** | `telemetry.py` | JSONL-Eventlog + Live-Counter |
| **Gedächtnis** | `memory_bridge.py` | FTS5-Lessons-DB (`.sin/agent-memory.db`) |
| **Werkzeuge** | `builtin_tools.py` | sin_bash, sin_read/write/edit/search — sandboxed |
| **Loop** | `loop.py` | recall → plan → execute → verify → repair → remember |
| **Repair** | `repair.py` | Deterministic + LLM-Tiers, Tool-Whitelist, JSON-Extract |
| **Compactor** | `compactor.py` | HOT/WARM/COLD-Rolling-Window für lange Sessions |
| **Synthese** | `synthesizer.py` | Goal→DAG, Repo-Survey, Critique-Pass, Garbage-Fallback |
| **Watch** | `watch.py` | Live-Dashboard, ANSI, tail -f, TTY-detect |
| **Checkpoint** | `checkpoint.py` | Crash-safe Journal + Git-Tree-Fingerprint, fsync pro Step |
| **Policy** | `policy_sandbox.py` | Deny-gewinnt-immer, Audit-Trail, Dry-Run |
| **Delegation** | `delegate.py` | Sub-Agent mit Tiefenlimit + Budget-Vererbung |
| **Insights** | `insights.py` | Telemetry→priorisierte Empfehlungen, Prompt-Inject |
| **CLI** | `cli.py` | `sin agent {run,resume,recall,stats,synth,watch,insights,policy-check}` |

## Anti-Failure-Invariants

1. **Alle Edits transaktional** — FsTxn + txnnicht in Python (über
   `sin_bash` + Worktree), aber jede Repo-Mutation läuft durch
   `git worktree add` → isoliert.
2. **Verification ist notwendig aber nicht hinreichend** — MergePolicy
   aus dem Go-Orchestrator (separat) entscheidet über Auto-Merge.
3. **Tool-Aufrufe sind bounded** — Circuit-Breaker, max_retries,
   Timeout, Redaction.
4. **Pläne sind DAG-validiert** — Zyklen, unbekannte Deps, doppelte
   IDs werden zur Build-Zeit (nicht zur Run-Zeit) abgewiesen.
5. **Lessons werden cross-session erinnert** — FTS5-Injektion in
   jeden neuen Plan.
6. **Crash-Recovery ist fsync'd** — Journal-Eintrag pro terminalem
   Step-Zustand, Torn-Last-Line-tolerant.
7. **Delegation ist fork-bomben-sicher** — Tiefe + Budget geometrisch
   schrumpfend, fresh router per Kind.
8. **Policy ist deny-by-exception mit audit trail** — keine
   Bypass-Magie.

## CLI-Surface (Typer)

```bash
sin agent synth --goal "refactor auth" --repo . --out plan.json
sin agent run --goal "..." --plan plan.json --repo . --parallel 4
sin agent resume --goal "..." --plan plan.json --task-id <id>
sin agent recall --goal "..."
sin agent stats
sin agent watch [--once]   # Live-Dashboard
sin agent insights [--prompt]
sin agent policy-check --tool sin_bash --args '{"cmd":"rm -rf /"}'
```

## Verifikation

- 34/34 Tests grün (`python3 -m pytest tests/agent_engine/`)
- 0 externe Dependencies (nur Python 3.11+ stdlib + typer)
- Hermetisch: kein Netzwerk, keine Mock-Frameworks nötig
