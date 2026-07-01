# DoD Tasks — Typische Task-Typen und ihre Checklisten

## Task: Feature implementieren

### DoD-Vertrag
```yaml
version: "1.0"
task: "<feature-beschreibung>"

pillars:
  no_placeholders:
    enabled: true
    forbidden_patterns:
      - "TODO"
      - "FIXME"
      - "panic("
      - "// Hier Logik"
      - "pass  #"

  error_handling:
    enabled: true

  tests_pass:
    enabled: true
    auto_detect: true
    min_test_count: 1

  build_lint:
    enabled: true
    auto_detect: true

  required_artifacts:
    enabled: true
    files:
      - "README.md"

  requirements_coverage:
    enabled: true
    requirements:
      - "Feature-Funktionalität implementiert"
      - "Edge Cases behandelt"
      - "Unit Tests geschrieben"

  no_dead_code:
    enabled: true
```

## Task: Bug fixen

### DoD-Vertrag
```yaml
version: "1.0"
task: "Bug: <beschreibung>"

pillars:
  no_placeholders:
    enabled: true

  error_handling:
    enabled: true

  tests_pass:
    enabled: true
    auto_detect: true
    min_test_count: 1
    # WICHTIG: Test muss den Bug reproduzieren bevor der Fix angewendet wird
    # und danach grün sein

  build_lint:
    enabled: true
    auto_detect: true

  required_artifacts:
    enabled: false  # Bug-Fix braucht meist keine neuen Artefakte

  requirements_coverage:
    enabled: true
    requirements:
      - "Bug-Ursache identifiziert"
      - "Fix implementiert"
      - "Regression-Test geschrieben"
      - "Fix verifiziert (Test grün)"
```

## Task: Refactoring

### DoD-Vertrag
```yaml
version: "1.0"
task: "Refactor: <beschreibung>"

pillars:
  no_placeholders:
    enabled: true

  error_handling:
    enabled: false  # Refactoring ändert keine Logik

  tests_pass:
    enabled: true
    auto_detect: true
    # WICHTIG: Alle bestehenden Tests müssen weiterhin grün sein

  build_lint:
    enabled: true
    auto_detect: true

  required_artifacts:
    enabled: false

  requirements_coverage:
    enabled: true
    requirements:
      - "Bestehende Tests weiterhin grün"
      - "Code-Struktur verbessert"
      - "Keine Funktionalität verloren"

  no_dead_code:
    enabled: true
    # Refactoring sollte toten Code entfernen, nicht hinzufügen
```

## Task: Neue Skill/Tool erstellen

### DoD-Vertrag
```yaml
version: "1.0"
task: "Skill/Tool: <beschreibung>"

pillars:
  no_placeholders:
    enabled: true

  error_handling:
    enabled: true

  tests_pass:
    enabled: true
    auto_detect: true
    min_test_count: 3

  build_lint:
    enabled: true
    auto_detect: true

  required_artifacts:
    enabled: true
    files:
      - "README.md"
      - "LICENSE"

  requirements_coverage:
    enabled: true
    requirements:
      - "SKILL.md mit Frontmatter"
      - "context/ Verzeichnis"
      - "frameworks/ Verzeichnis"
      - "tasks/ Verzeichnis"
      - "templates/ Verzeichnis"
      - "Beispiele funktionsfähig"

  no_dead_code:
    enabled: true
```

## Task: Dokumentation schreiben

### DoD-Vertrag
```yaml
version: "1.0"
task: "Docs: <beschreibung>"

pillars:
  no_placeholders:
    enabled: true
    forbidden_patterns:
      - "TODO"
      - "TBD"
      - "TBA"
      - "FIXME"

  error_handling:
    enabled: false  # Docs haben keine Error-Pfade

  tests_pass:
    enabled: false  # Docs haben keine Tests

  build_lint:
    enabled: true
    # Markdown linting falls verfügbar
    lint_command: "markdownlint . --config .markdownlint.json 2>/dev/null || true"

  required_artifacts:
    enabled: true
    files:
      - "README.md"

  requirements_coverage:
    enabled: true
    requirements:
      - "Alle Features dokumentiert"
      - "Beispiele funktionieren"
      - "Installation erklärt"

  no_dead_code:
    enabled: false  # Docs haben keinen toten Code
```

## Custom Task

Der Agent generiert den DoD-Vertrag basierend auf der Task-Beschreibung:
1. Analysiere die Task-Beschreibung
2. Wähle den passenden Task-Typ (oder Mischung)
3. Fülle die requirements_coverage mit den konkreten Anforderungen
4. Passe die forbidden_patterns an die Sprache an
5. Speichere als `dod-contract.yaml` im Workspace
