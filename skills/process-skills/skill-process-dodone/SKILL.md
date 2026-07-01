---
name: skill-process-dodone
description: >-
  Definition-of-Done Enforcer — das unbestechliche Gate gegen KI-Faulheit.
  Erzeugt einen maschinenpruefbaren DoD-Vertrag aus der Task-Beschreibung,
  injiziert ihn als System-Prompt, und validiert ihn deterministisch beim
  Abschluss. Verhindert dass KI "ja ist fertig" sagt wenn es nicht fertig ist.
  Triggers: "definition of done", "dod", "ist das fertig", "done check",
  "dodone", "wann ist etwas fertig", "done-done", "DoD enforcement",
  "quality gate", " completion check", " wirklich fertig", "echt fertig".
license: MIT
lifecycle: native
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 2.0.0
  category: process
  repo: github.com/OpenSIN-Code/SIN-DoDone
  sin-mandate: M3 (verification gate is sacred)
  activation-keyword: dodone
  ecosystem-integration:
    - poc (SIN-Code-PoC-Tool — invariant checks)
    - adw (SIN-Code-ADW-Tool — architectural debt)
    - sckg (SIN-Code-SCKG-Tool — dead code detection)
    - sin-security (SIN-Code-Security-Bundle — 8 security tools)
    - sin-brain (SIN-Brain — DoD memory across sessions)
    - goal-mode (SIN-Code-Goal-Mode-Skill — checkpoint/rollback)
    - grill-me (SIN-Code-Grill-Me-Skill — pre-implementation DoD questioning)
    - orchestration (SIN-Code-Orchestration — parallel DoD checks)
    - check-plan-done (Infra-SIN-OpenCode-Stack — plan with done criteria)
    - self-review (Infra-SIN-OpenCode-Stack — CEO-level review)
required_tools:
  - sin_bash
---

# skill-process-dodone — Definition-of-Done Enforcer

## Das Problem

KIs sagen "ja, ist alles fertig" — und lügen. In ~100% der Fälle ist bei
genauerem Hinsehen mindest ein Kriterium nicht erfüllt: fehlende Tests,
unbehandelte Fehlerpfade, Platzhalter-Code (`// TODO`, `panic(`), totbefundete
Imports, nicht verdrahtete Komponenten, oder stillschweigend weggelassene
Anforderungen. Der Mensch merkt es erst bei intensiven Nachfragen — dann ist
die KI schon längst weiter und der Kontext ist verloren.

**Die Lösung:** Ein **expliziter, maschinenprüfbarer Vertrag** der festlegt,
wann etwas *wirklich* fertig ist — automatisch generiert, automatisch geprüft,
ohne Mensch-oder-KI als Schwachstelle.

## Konzept: DONE-done

Aus der Agile-Alliance-Definition (seit 2002, Bill Wake):

> "Done" bedeutet meistens "fertig mit Programmieren".
> "DONE-done" bedeutet: programmiert + getestet + deployable + dokumentiert +
> alle Edge-Cases behandelt + keine Platzhalter + Build grün.

SIN-DoDone macht DONE-done **maschinell und unausweichlich**.

## Architektur

```
Task-Beschreibung
       │
       ▼
 ┌──────────────────────────┐
 │  1. DoD-Vertrag erzeugen  │  ← templates/dod-contract.yaml
 │  (aus Task + Defaults)    │
 └──────────────────────────┘
       │
       ▼
 ┌──────────────────────────┐
 │  2. System-Prompt injiz.  │  ← templates/system-prompt.md
 │  (Agent weiß: DoD exist.) │
 └──────────────────────────┘
       │
       ▼
 ┌──────────────────────────┐
 │  3. Agent arbeitet        │  (PLAN → ACT → VERIFY)
 │  (mit DoD im Kontext)     │
 └──────────────────────────┘
       │
       ▼
 ┌──────────────────────────┐
 │  4. DoD-Check ausführen    │  ← scripts/dodone-check.sh
 │  (deterministisch, keine   │
 │   KI-Selbsteinschätzung)   │
 └──────────────────────────┘
       │
       ├── Exit 0: WIRKLICH FERTIG → Task darf abgeschlossen werden
       ├── Exit 2: CODE UNVOLLSTÄNDIG → zurück zum Agenten mit Findings
       └── Exit 3: TESTS ROT → zurück zum Agenten mit Fehlerlog
```

## DoD-Vertrag — Die 7 Säulen

Jeder DoD-Vertrag prüft 7 Kategorien. Jede ist **deterministisch** — keine
KI-Selbsteinschätzung, keine "sieht gut aus"-Lügen.

### Säule 1: Keine Platzhalter (Anti-Cheat)
Verbotene Patterns im Code: `TODO`, `FIXME`, `panic(`, `// Logik einfügen`,
`pass  # ...`, `// implement later`, `throw new Error("not implemented")`.

### Säule 2: Fehlerpfade behandelt
Jeder `if err != nil` / `except` / `catch` muss **echte** Behandlung haben —
kein leeres `pass`, kein `_ = err`, kein `// ignore`.

### Säule 3: Tests existieren und sind grün
`go test`, `pytest`, `npm test` — muss exit 0 liefern. Mindestens 1 Test
pro neue Funktion.

### Säule 4: Build/Lint sauber
`go build`, `go vet`, `ruff`, `eslint` — muss ohne Fehler durchlaufen.

### Säule 5: Erforderliche Artefakte vorhanden
README.md, CHANGELOG-Eintrag, API-Doku — je nach Projekt-Config.

### Säule 6: Anforderungen vollständig abgedeckt
Jeder Punkt aus der ursprünglichen Task-Beschreibung muss im Code nachvollziehbar
umgesetzt sein. Wird über eine Checklist im DoD-Vertrag geprüft.

### Säule 7: Kein toter Code
Keine ungenutzten Imports, keine auskommentierten Blöcke, keine Funktionen
die nirgendwo aufgerufen werden.

## DoD-Vertrag Format

```yaml
# dod-contract.yaml — wird pro Task generiert
version: "1.0"
task: "<ursprüngliche Task-Beschreibung>"

pillars:
  no_placeholders:
    enabled: true
    forbidden_patterns:
      - "TODO"
      - "FIXME"
      - "panic("
      - "// Hier Logik einfügen"
      - "pass  #"
      - "throw new Error(\"not implemented\")"

  error_handling:
    enabled: true
    forbidden_ignores:
      - "_ = err"
      - "except.*pass$"
      - "catch.*\\{\\s*\\}"

  tests_pass:
    enabled: true
    command: "go test ./... -v -count=1"
    min_coverage: 0

  build_lint:
    enabled: true
    build_command: "go build ./..."
    lint_command: "go vet ./..."

  required_artifacts:
    enabled: true
    files:
      - "README.md"

  requirements_coverage:
    enabled: true
    requirements:
      - "Feature X implementiert"
      - "Edge Case Y behandelt"
      - "Unit Test für Z geschrieben"

  no_dead_code:
    enabled: true
    check_unused_imports: true
```

## Verwendung

### In SIN-Code / OpenCode (als Skill)

```
User: "Implementiere Feature X und prüfe mit DoDone"
→ Skill aktiviert
→ DoD-Vertrag aus Task generiert
→ System-Prompt injiziert
→ Agent arbeitet
→ Beim "ich bin fertig": dodone-check.sh läuft
→ Exit 0 = wirklich fertig / Exit 2|3 = zurück zum Agenten
```

### Als CLI (standalone)

```bash
# DoD-Vertrag aus Task generieren
dodone generate --task "Implementiere User-Login mit JWT"

# DoD-Check ausführen
dodone check --contract dod-contract.yaml

# Exit-Codes:
# 0 = WIRKLICH FERTIG
# 1 = Config/System-Fehler
# 2 = Code unvollständig (Platzhalter, fehlende Artefakte)
# 3 = Tests/Build fehlgeschlagen
```

## System-Prompt-Injektion

Wenn der Skill aktiv ist, wird folgender Prompt vor der Task an den Agenten
injiziert:

```
[SYSTEM GATEKEEPER — SIN-DoDone]

Ein maschineller DoD-Check wird beim Abschluss ausgeführt.
Du kannst ihn nicht umgehen, nicht überreden, nicht austricksen.

Folgende Kriterien werden deterministisch geprüft:
1. Keine verbotenen Patterns (TODO, FIXME, panic, ...)
2. Alle Fehlerpfade echt behandelt
3. Test-Suite grün
4. Build + Lint sauber
5. Erforderliche Dateien vorhanden
6. Alle Anforderungen abgedeckt
7. Kein toter Code

Bevor du "fertig" sagst, prüfe selbst:
- Sind alle TODO/FIXME entfernt?
- Sind alle error-Pfade echt behandelt?
- Laufen die Tests?
- Ist der Build sauber?

Lügen nützt nichts — der Check ist maschinell.
```

## Integration in SIN-Code Agent Loop

SIN-DoDone ist Teil des **Verify-Gates** (M3):

1. Agent sagt "done" → Verify-Gate wird aktiv
2. SIN-DoDone-Check läuft als Teil des Verify-Gates
3. Exit 0 → Verify-Gate passed → Task darf als "verified" markiert werden
4. Exit 2|3 → Verify-Gate failed → Findings werden als User-Turn re-injiziert
5. Agent muss die Findings beheben und erneut "done" sagen

## Integration in OpenCode CLI

In OpenCode wird der Skill als `/dodone` Slash-Command geladen:

```
/dodone Implementiere Feature X
→ Generiert DoD-Vertrag
→ Injiziert System-Prompt
→ Nach "fertig": Führt Check aus
→ Blockiert Abschluss bis Exit 0
```

## Exit-Code-Tabelle

| Code | Bedeutung | Agent-Aktion |
|------|-----------|-------------|
| 0 | WIRKLICH FERTIG | Task abschließen |
| 1 | Config/System-Fehler | Config prüfen, nicht Code-Fehler |
| 2 | Code unvollständig | Findings beheben, Code vervollständigen |
| 3 | Tests/Build rot | Tests/Build fixen |

## Ecosystem-Integration (v2)

SIN-DoDone v2 integriert **10 OpenSIN-Code Repos** als deterministische DoD-Säulen.
Jede Säule degradiert graceful (SKIP) wenn das Tool nicht installiert ist.

Siehe `context/ecosystem-integration.md` für die vollständige Mapping-Tabelle.

### Pre-Implementation (vor dem Coden)

| Phase | Tool | Was es tut |
|---|---|---|
| DoD-Kriterien grillen | Grill-Me (`grill_start` / `grill_synthesize`) | Adversarisches Interview: "Was sind Erfolgskriterien? Welche Edge-Cases?" |
| DoD als Goal tracken | Goal-Mode (`goal_start` / `goal_checkpoint`) | DoD-Kriterien als Subtasks, Checkpoint vor Risiko-Änderungen |
| DoD im Memory speichern | SIN-Brain (`remember` / `pin`) | DoD-Kriterien als `tier=core, kind=convention, pinned=true` |

### Post-Implementation (DoD-Check Pipeline)

| Säule | Builtin | Ecosystem-Tool | SIN-Code-intern |
|---|---|---|---|
| P1: Platzhalter | grep TODO/FIXME/panic | — | SelfReviewReflector |
| P2: Fehlerpfade | grep _ = err | `poc verify` | — |
| P3: Tests | go test / pytest | `oracle check` | testgate.Run() |
| P4: Build+Lint | go build+vet / ruff | — | testgate.Run() |
| P5: Artefakte | README.md check | — | goalcontract.Baseline |
| P6: Invarianten | — | `poc verify` | verify.Gate (poc mode) |
| P7: Architektur | — | `adw scan` | audit.Auditor |
| P8: Sicherheit | — | `sin-security scan` | SecurityScan |
| P9: Toter Code | go vet | `sckg dead_code` | complexity.Find |

### Post-Check (nach DoD-Check)

| Aktion | Tool | Exit 0 | Exit 2/3 |
|---|---|---|---|
| Goal abschließen | Goal-Mode | `goal_complete()` | `goal_rollback()` |
| Memory updaten | SIN-Brain | `remember("DoD passed")` | `learn_from_error("DoD failed")` |
| Lessons learnen | SIN-Code lessons | — | `Record(TypeFailedVerification)` |

## Bezug zur SIN-Code-Architektur

- **M3 (Verify Gate):** SIN-DoDone ist eine Erweiterung des Verify-Gates.
  Es ersetzt nicht PoC/Oracle, sondern ergänzt sie um deterministische
  DoD-Checks.
- **M4 (Permission Engine):** Der DoD-Check läuft als `sin_bash` mit
  `allow`-Policy (read-only Scan + Test-Run).
- **M6 (SIN tools over naive):** Der Skill nutzt `sin_bash` für die
  deterministischen Checks, nicht eine KI-Selbsteinschätzung.
- **goalcontract:** Der DoD-Vertrag ist kompatibel mit `internal/goalcontract`
  Definition-of-Done Contracts und kann als Stop-Gate-Input dienen.

## Bezug zu anderen Skills

- **self-review:** SIN-DoDone ist die *maschinelle* Ergänzung zum
  *beweisgetriebenen* self-review. self-review macht die CEO-Prüfung,
  SIN-DoDone macht die deterministische Maschinenprüfung.
- **skill-code-lazy:** lazy validiert *was* gebaut wird, SIN-DoDone validiert
  *ob* es fertig ist. Beide respektieren M3.
- **skill-process-grill:** grill hinterfragt das *Design*, SIN-DoDone
  hinterfragt die *Vollständigkeit*.
- **delegate-subagents:** Stellt die Runtime-Infrastruktur (stop-gate, `criteria`
  Parameter auf `sin_run_loop`) bereit, in die DoDone-Verträge einspeisen. Der
  DoD-Vertrag mappt direkt zu `delegate-subagents`' Stop-Gate deterministic-checks.
- **check-plan-done / plan v2:** Definieren `done_criteria` während der Planung.
  DoDone übernimmt diese als `requirements_coverage.requirements` und enforciert
  sie deterministisch beim Abschluss. Data-Flow: plan → DoDone-Vertrag → Check.
- **ceo-audit:** Repo-weiter 47-Gate Audit (3-5 Min). DoDone ist task-spezifisch
  (11 Säulen, Sekunden). Für große Tasks kann ceo-audit als "deep verification"
  nach DoDone laufen.

## Quellen

- Agile Alliance: "Definition of Done" Glossary (2002-Present)
- Bill Wake (2002): "Coaching Drills and Exercises" — DONE-done Konzept
- Scrum.org: "Definition of Done Explained"
- SIN-Code AGENTS.md §3 M3 (Verification Gate)
- SIN-DoDone CLI: github.com/OpenSIN-Code/SIN-DoDone
