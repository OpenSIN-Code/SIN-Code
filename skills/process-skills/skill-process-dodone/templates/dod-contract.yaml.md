# DoD Contract Template — kopieren und ausfüllen

## Basis-Template (für alle Task-Typen)

```yaml
# dod-contract.yaml
# SIN-DoDone Definition-of-Done Vertrag
# Generiert von: skill-process-dodone
# Task: <TASK_BESCHREIBUNG>

version: "1.0"
task: "< ursprüngliche Task-Beschreibung hier >"
project_name: "< Projekt-Name >"

pillars:
  # Säule 1: Keine Platzhalter (Anti-Cheat)
  no_placeholders:
    enabled: true
    forbidden_patterns:
      - "TODO"
      - "FIXME"
      - "panic("
      - "// Hier Logik einfügen"
      - "pass  #"
      - "throw new Error(\"not implemented\")"
      - "NotImplemented"
      - "unimplemented!"

  # Säule 2: Fehlerpfade echt behandelt
  error_handling:
    enabled: true
    forbidden_ignores:
      - "_ = err"
      - "except.*:\\s*pass"
      - "catch.*\\{\\s*\\}"
      - "\\.unwrap()"  # Rust — warn only

  # Säule 3: Tests grün
  tests_pass:
    enabled: true
    auto_detect: true  # auto-detect language from project files
    # command: "go test ./... -v -count=1"  # override if needed
    min_test_count: 1

  # Säule 4: Build + Lint sauber
  build_lint:
    enabled: true
    auto_detect: true
    # build_command: "go build ./..."
    # lint_command: "go vet ./..."

  # Säule 5: Erforderliche Artefakte
  required_artifacts:
    enabled: true
    files:
      - "README.md"
    # additional files per task type:
    # - "CHANGELOG.md"
    # - "docs/api.md"
    # - "LICENSE"

  # Säule 6: Anforderungen abgedeckt
  requirements_coverage:
    enabled: true
    requirements:
      # Eine Zeile pro Anforderung aus der Task-Beschreibung
      # Der Agent muss jede Anforderung mit file:line belegen
      - "Anforderung 1: <beschreibung>"
      - "Anforderung 2: <beschreibung>"
      - "Anforderung 3: <beschreibung>"

  # Säule 7: Kein toter Code
  no_dead_code:
    enabled: true
    check_unused_imports: true
    # exclude_dirs:
    #   - "vendor"
    #   - "node_modules"
    #   - ".git"
```

## System-Prompt Template

```markdown
[SYSTEM GATEKEEPER — SIN-DoDone — {project_name}]

Ein maschineller DoD-Check wird beim Abschluss ausgeführt.
Du kannst ihn nicht umgehen, nicht überreden, nicht austricksen.

Folgende Kriterien werden deterministisch geprüft:
1. Keine verbotenen Patterns (TODO, FIXME, panic, ...)
2. Alle Fehlerpfade echt behandelt (kein _ = err, kein except: pass)
3. Test-Suite grün (auto-detected: {test_command})
4. Build + Lint sauber (auto-detected: {build_command})
5. Erforderliche Dateien vorhanden: {required_files}
6. Alle Anforderungen abgedeckt: {requirements_count} Anforderungen
7. Kein toter Code (keine ungenutzten Imports)

VERBOTENE BEHAUPTUNGEN:
- "sollte funktionieren" → Stattdessen: führe den Check aus
- "müsste passen" → Stattdessen: führe den Test aus
- "sieht gut aus" → Stattdessen: zeige den Build-Output
- "ist alles fertig" → Stattdessen: DoD-Check muss Exit 0 liefern

Bevor du "fertig" sagst, selbst prüfen:
- grep -rn 'TODO\|FIXME\|panic(' --include='*.go' .
- go test ./... -v -count=1
- go build ./... && go vet ./...

Lügen nützt nichts — der Check ist maschinell.
```

## DoD-Check Report Template

```markdown
# SIN-DoDone Report

**Task:** {task}
**Datum:** {timestamp}
**Exit-Code:** {exit_code}

## Säule 1: Keine Platzhalter
- Status: {PASS|FAIL}
- Befunde: {count} verbotene Patterns gefunden
{findings_list}

## Säule 2: Fehlerpfade
- Status: {PASS|FAIL}
- Befunde: {count} ignorierte Fehlerpfade
{findings_list}

## Säule 3: Tests
- Status: {PASS|FAIL}
- Command: {test_command}
- Dauer: {duration}
- Output: {output_lines} Zeilen
{output_excerpt}

## Säule 4: Build + Lint
- Build: {PASS|FAIL}
- Lint: {PASS|FAIL}

## Säule 5: Artefakte
- Status: {PASS|FAIL}
- Fehlend: {missing_files}

## Säule 6: Anforderungen
- Status: {PASS|FAIL}
- Abgedeckt: {covered}/{total}
{requirements_list}

## Säule 7: Toter Code
- Status: {PASS|FAIL}
- Befunde: {count} ungenutzte Symbole

## Ergebnis
{exit_code == 0 ? "WIRKLICH FERTIG — alle 7 Säulen bestanden" : "NICHT FERTIG — siehe Befunde oben"}
```
