---
name: skill-process-self-review
description: >
  Erzwingt eine kompromisslose, beweisgetriebene Endkontrolle ("Ultra-CEO-Review")
  der gesamten Arbeit aus der aktuellen Session UND aller an Subagents delegierten
  Aufgaben. Einsetzen, bevor eine Aufgabe als "fertig" gemeldet wird, wenn der User
  Review / Abnahme / Qualitätskontrolle verlangt, nach mehrstufigen Implementierungen,
  nach Delegation an Subagents, oder bei Phrasen wie "prüfe deine Arbeit",
  "ist das vollständig", "review", "Abnahme", "Endkontrolle", "auf Fehler prüfen",
  "self-review", "führe self-review aus".
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.26.0
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/self-review"
required_tools:
  - sin_scout
  - sin_bash
  - sin_read
lifecycle: external
---

# skill-process-self-review — Ultra-CEO-Endkontrolle

## Haltung (nicht verhandelbar)

Du bist nicht der wohlwollende Entwickler, der seine eigene Arbeit abnimmt.
Du bist ein **Ultra-CEO**, der ein Auslieferungs-Veto hat und persönlich haftet,
wenn fehlerhafte Arbeit rausgeht. Deine Standardannahme lautet:
**"Diese Arbeit ist kaputt, bis das Gegenteil bewiesen ist."**

- Du suchst aktiv Gründe, die Arbeit **abzulehnen**, nicht sie durchzuwinken.
- "Sieht gut aus", "sollte funktionieren", "müsste passen" sind **verboten**.
- Jede Aussage braucht einen **Beleg**: Dateiinhalt, Diff oder echte Tool-Ausgabe.
- Du delegierst die Schuld nicht an Subagents. Ihr Output ist **deine** Verantwortung.

Lies `references/ceo-doctrine.md` und wende die Doktrin auf jedes Finding an.

## Wann zwingend ausführen

- Bevor du eine nicht-triviale Aufgabe als abgeschlossen meldest.
- Nachdem Subagents Teilaufgaben zurückgeliefert haben.
- Wenn der User Review, Abnahme, Qualitätskontrolle oder Fehlerprüfung verlangt.
- Nach mehrstufigen Änderungen über mehrere Dateien.

## Grundregeln

1. **Keine Annahmen.** Lies die realen Dateien neu ein. Vertraue weder deinem
   Gedächtnis noch dem, was du zu geschrieben haben glaubst.
2. **Subagent-Output ist ungeprüft und verdächtig.** Lies jede von einem Subagent
   erstellte/geänderte Datei vollständig.
3. **Beweise statt Behauptungen.** Real ausführen, nicht "vorschlagen".
4. **Findings werden dokumentiert**, nicht stillschweigend übergangen.

## Ablauf (strikt in dieser Reihenfolge)

### Schritt 1 — Arbeitsumfang rekonstruieren
- Liste alle Aufgaben dieser Session (eigene + delegierte).
- Liste alle erstellten/geänderten/gelöschten Dateien.
- Gleiche mit `git status` / `git diff` (via `sin_bash`) gegen dein Gedächtnis ab.
  Jede Differenz ist selbst schon ein Finding.

### Schritt 2 — Anforderungsabgleich
Pro ursprünglicher User-Anforderung:
- Vollständig / teilweise / gar nicht umgesetzt?
- Scope Creep (nicht Verlangtes eingebaut)?
- Stillschweigend Weggelassenes?

### Schritt 3 — Datei-für-Datei-Prüfung
Lies jede geänderte Datei vollständig (via `sin_read`) gegen `references/checklist.md`.
Besonders: Platzhalter/TODO/FIXME, Dummy-Daten, halbfertige Funktionen,
nicht verdrahtete Komponenten, tote Imports, Fehlerbehandlung, Edge Cases,
Konsistenz zwischen Subagent-Ergebnissen.

### Schritt 4 — Verifikation ausführen (automatisiert)
Führe `scripts/verify.sh` aus (via `sin_bash`). Das Script erkennt Build/Typecheck,
Lint und Tests automatisch (Node/pnpm/npm/yarn, Python, Go, Rust, Make) und
protokolliert die echten Ergebnisse. Ein roter Build/Test ist ein sofortiger BLOCKER.
Ein grüner Lauf ist **kein** Beweis für korrektes Verhalten — nur ein notwendiges Minimum.

### Schritt 5 — Severity-Bewertung (CEO-Maßstab)
- **BLOCKER** — kaputt / Anforderung verfehlt / Build/Test rot.
- **MAJOR** — läuft, aber fehlerhaft, unsicher oder lückenhaft.
- **MINOR** — Qualität, Lesbarkeit, Konsistenz.
- **NIT** — Stil/Kosmetik.

### Schritt 6 — Bericht, Korrektur, Re-Verifikation
- Gib den Bericht aus `references/report-template.md` aus.
- Behebe alle BLOCKER und MAJOR sofort → zurück zu Schritt 4 (re-verifizieren).
- MINOR/NIT: beheben oder dem User explizit zur Entscheidung vorlegen.
- Wiederhole die Schleife, bis 0 offene BLOCKER/MAJOR.

## Abschlusskriterium (das CEO-Veto)

Du darfst NUR "fertig" melden, wenn ALLE gelten:
- 0 offene BLOCKER und 0 offene MAJOR, jeweils re-verifiziert.
- `scripts/verify.sh` zuletzt erfolgreich gelaufen (echte Ausgabe gezeigt).
- Jede ursprüngliche Anforderung nachweislich abgedeckt (mit Beleg).
- Der Bericht wurde ausgegeben.

Ist auch nur eine Bedingung verletzt, lautet dein Status **NICHT ABGESCHLOSSEN**,
und du sagst dem User klar, was noch fehlt — keine Beschönigung.

## Maschinelle Ergänzung: SIN-DoDone

self-review ist die *beweisgetriebene* CEO-Prüfung (Mensch/LLM urteilt).
SIN-DoDone (`skill-process-dodone`) ist die *maschinelle* Ergänzung:
deterministische Checks (grep TODO/FIXME, `go test`, `go build`, etc.)
mit Exit-Codes — keine KI-Selbsteinschätzung möglich.

**Empfohlene Pipeline:**
1. self-review (CEO liest Diffs, bewertet Scope/Design/Konsistenz)
2. `dodone check` (Maschine checkt deterministisch: Platzhalter, Tests, Build)
3. Erst wenn BEIDE grün: Task ist WIRKLICH FERTIG

DoDone fängt was self-review verfehlen kann: skalen-unabhängige,
wiederholbare, deterministische Pattern-Matches über den gesamten Codebase.
