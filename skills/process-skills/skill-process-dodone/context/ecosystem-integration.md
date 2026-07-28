# Ecosystem Integration Map — SIN-DoDone v2

## Vollständige Inventur aller OpenSIN-Code Repos und ihrer DoD-Relevanz

### Tier 1: Direkt integriert (Go binary calls these as subprocesses)

| Tool | Repo | CLI | Exit-Code-Vertrag | DoD-Säule |
|---|---|---|---|---|
| **PoC** | SIN-Code-PoC-Tool | `poc verify <path>` | 0=clean, 1=violations | P6: Invarianten |
| **ADW** | SIN-Code-ADW-Tool | `adw scan <path>` | 0=green, 1=debt | P7: Architektur |
| **sin-security** | SIN-Code-Security-Bundle | `sin-security scan . --fail-on critical` | 0=pass, 1=fail | P8: Sicherheit |
| **SCKG** | SIN-Code-SCKG-Tool | `sckg dead_code . --threshold 0.8` | 0=pass, 1=dead code | P9: Toter Code |

### Tier 2: Verfügbar via SIN-Code CLI (sin-code serve MCP adapter)

| Tool | Repo | MCP-Präfix | DoD-Anwendung |
|---|---|---|---|
| **Oracle** | SIN-Code-Oracle-Tool | `oracle__*` | Test-Abdeckung prüfen: `oracle check <src> --against <tests>` |
| **IBD** | SIN-Code-IBD-Tool | `ibd__*` | Intent-basierter Diff: `ibd diff <target> --before <old> --after <new>` |
| **EFM** | SIN-Code-EFM-Tool | `efm__*` | Ephemeral Test-Env: `efsm setup <name> --api X --test-cmd "pytest"` |

### Tier 3: SIN-Code interne Komponenten (im SIN-Code Monorepo)

| Komponente | Pfad | API | DoD-Anwendung |
|---|---|---|---|
| **Verify Gate** | `internal/verify/` | `verify.NewGate(mode, poc, oracle)` | Dispatch PoC/Oracle Runner |
| **Stop Gate** | `internal/stopgate/` | `stopgate.New(ws).Evaluate(ctx, contract, snap)` | Hybrid det+LLM completion authority |
| **Goal Contract** | `internal/goalcontract/` | `goalcontract.Resolve(opts)` | DoD-Vertrag aus Task+Baseline bauen |
| **Test Gate** | `internal/testgate/` | `testgate.Run(ctx, cfg)` | Quality-Gate: build→vet→test→staticcheck→gosec→govulncheck |
| **Complexity** | `internal/complexity/` | `complexity.Find(opts)` | Ponytail 5-Tag Analyse (delete/stdlib/native/yagni/shrink) |
| **Audit Engine** | `internal/audit/` | `audit.NewAuditor(nil).Audit(ctx, root, opts)` | 48 CEO-Audit-Gates |
| **Sin Dept** | `internal/sindept/` | `sindept.ParseDir(root, opts)` + `Policy.RunCheck()` | sin-debt Marker + Rot-Risk Gate |
| **Tool Coverage** | `internal/agentloop/toolcoverage.go` | `NewToolCoverageEnforcer(req, forb).Check()` | Required/Forbidden Tools |
| **Lessons** | `internal/lessons/` | `lessons.Open(path)` → `Record()` / `BriefingForContext()` | Closed Learning Loop |
| **Self-Review** | `internal/agentloop/loop_selfreview.go` | `scanChangedFiles(cfg)` | Regex-Scan geänderter Dateien nach TODO/FIXME/stub |

### Tier 4: Ecosystem Skills (MCP Server)

| Skill | Repo | MCP-Tools | DoD-Anwendung |
|---|---|---|---|
| **SIN-Brain** | SIN-Brain | `remember`, `recall`, `pin`, `inject`, `learn_from_error`, `enforce_conventions` | DoD-Kriterien als `tier=core, kind=convention, pinned=true` speichern. Verifikationsergebnisse in `EPISODIC` loggen. `learn_from_error()` bei DoD-Fail. |
| **Goal Mode** | SIN-Code-Goal-Mode-Skill | `goal_start`, `goal_status`, `goal_checkpoint`, `goal_rollback`, `goal_subtask`, `goal_report` | DoD-Kriterien als Subtasks. Checkpoint vor Risiko-Änderungen. Rollback bei DoD-Fail. |
| **Grill Me** | SIN-Code-Grill-Me-Skill | `grill_start`, `grill_next_question`, `grill_record_answer`, `grill_synthesize` | DoD-Kriterien vor Implementierung grillen. "Was sind die Erfolgskriterien? Wie wissen wir dass es funktioniert?" |
| **Orchestration** | SIN-Code-Orchestration | `orchestrate_tasks`, `run_workflow` | DoD-Checks parallel als DAG dispatchen. `Oracle` verifiziert每个Task-Output. |
| **Slash** | SIN-Code (bundled) | `sin slash` plus legacy `slash_dispatch`, `slash_register`, `slash_list` compatibility | `/dodone` als gebündelten SIN-Code-Slash-Command registrieren und dispatchen. |

### Tier 5: Infra-SIN-OpenCode-Stack (Commands + Agents)

| Komponente | Pfad | DoD-Anwendung |
|---|---|---|
| **check-plan-done** | `commands/check-plan-done.md` + `skills/check-plan-done/SKILL.md` | 5-Stage Plan-und-Execute mit Done-Criteria Gate |
| **dodone** | `commands/dodone.md` | `/dodone` Command-Stub (lädt diesen Skill) |
| **sin-solo agent** | `agents/sin-solo/` | `VALIDATE_IMMEDIATELY` — validiert nach jeder Änderung |
| **sin-zeus agent** | `agents/sin-zeus/` | Fleet-Commander — delegiert + koordiniert via GitHub |
| **Blueprint-Mandates** | `agents-instructions/blueprint-mandates/` | 35 Mandate (Governance-Regeln) |

## Fusion-Architektur: Wie alles zusammenarbeitet

```
User: /dodone "Implementiere Feature X"
    │
    ▼
[Infra: dodone.md] lädt skill-process-dodone
    │
    ├──► [Grill-Me] grillt DoD-Kriterien VOR Implementierung
    │    "Was sind Erfolgskriterien? Welche Edge-Cases? Trust-Boundary?"
    │    → synthesize() = decisions + assumptions
    │
    ├──► [Goal-Mode] erstellt Goal mit DoD-Kriterien als Subtasks
    │    goal_start("Feature X", subtasks=[...])
    │    goal_checkpoint() vor Implementierung
    │
    ├──► [SIN-Brain] speichert DoD-Kriterien im Core-Memory
    │    remember("DoD: Feature X", kind=convention, tier=core, pinned=true)
    │
    ▼
Agent implementiert (PLAN → ACT)
    │
    ▼
DoD-Check-Pipeline läuft (alle deterministisch):
    │
    ├── P1: grep TODO/FIXME/panic          ← builtin
    ├── P2: grep _ = err / except: pass    ← builtin
    ├── P3: go test / pytest / npm test    ← builtin
    ├── P4: go build + vet / ruff / eslint ← builtin
    ├── P5: README.md existiert            ← builtin
    ├── P6: poc verify .                   ← ecosystem (PoC-Tool)
    ├── P7: adw scan .                     ← ecosystem (ADW-Tool)
    ├── P8: sin-security scan --fail-on critical ← ecosystem (Security-Bundle)
    └── P9: sckg dead_code --threshold 0.8 ← ecosystem (SCKG-Tool)
    │
    ├── Exit 0: WIRKLICH FERTIG
    │   ├── Goal-Mode: goal_complete()
    │   └── SIN-Brain: remember("DoD passed", kind=decision)
    │
    └── Exit 2|3: NICHT FERTIG
        ├── Findings als User-Turn re-injiziert
        ├── Goal-Mode: goal_rollback() zum Checkpoint
        ├── SIN-Brain: learn_from_error("DoD failed: <findings>")
        └── Lessons: Record(TypeFailedVerification) für nächste Session
```

## Mapping: DoD-Säule → Ecosystem-Tool → SIN-Code-Komponente

| DoD-Säule | Builtin-Check | Ecosystem-Tool | SIN-Code-intern |
|---|---|---|---|
| P1: Keine Platzhalter | `grep -rn TODO FIXME panic` | — | `SelfReviewReflector.scanChangedFiles()` |
| P2: Fehlerpfade | `grep _ = err / except: pass` | `poc verify` (invariant rules) | — |
| P3: Tests grün | `go test / pytest / npm test` | `oracle check` (coverage) | `testgate.Run()` |
| P4: Build+Lint | `go build + vet / ruff` | — | `testgate.Run()` (build+vet steps) |
| P5: Artefakte | `os.Stat(README.md)` | — | `goalcontract.Baseline()` (changelog check) |
| P6: Invarianten | — | `poc verify` | `verify.Gate.Run()` (poc mode) |
| P7: Architektur | — | `adw scan` | `audit.Auditor.Audit()` (complexity gates) |
| P8: Sicherheit | — | `sin-security scan` | `SecurityScan()` (secrets/SAST/SCA) |
| P9: Toter Code | `go vet` (unused) | `sckg dead_code` | `complexity.Find()` (delete tag) |

## Was SIN-DoDone NICHT ersetzt

- **M3 Verify Gate (PoC/Oracle):** SIN-DoDone ergänzt, ersetzt nicht. PoC/Oracle prüft Logik-Korrektheit, SIN-DoDone prüft Vollständigkeit.
- **self-review Skill:** self-review macht die CEO-Prüfung (beweisgetrieben, LLM-gestützt), SIN-DoDone macht die Maschinenprüfung (deterministisch, grep+exit-code).
- **skill-code-lazy:** lazy validiert *was* gebaut wird, SIN-DoDone validiert *ob* es fertig ist.
- **CEO Audit:** ceo-audit prüft 48 Gates repo-weit, SIN-DoDone prüft 9 Säulen task-spezifisch.
- **check-plan-done:** check-plan-done macht Plan-und-Execute, SIN-DoDone macht nur das Done-Gate am Ende.
