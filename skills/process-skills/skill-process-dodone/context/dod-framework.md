# DoD Framework — Die 7 Säulen der Vollständigkeit

## Ursprung

Das "Definition of Done" Konzept stammt aus der Agile/Scrum-Welt (seit ~2002).
Die Agile Alliance definiert es als:

> "A list of criteria that must be met before a product increment is
> considered 'done'. Failure to meet these criteria at the end of a sprint
> normally implies that the work should not be counted toward velocity."

Die Kernproblematik — direkt aus der Agile Alliance:

> "Software developers have a reputation for being somewhat careless when
> answering the question 'are you done with this feature'? In fairness, this
> is an ambiguous question — it can mean 'done programming' and this is
> generally what a developer will have in mind when answering. However, the
> meaning of interest is usually 'are you done programming, creating test
> data, actually testing, ensuring it's deployable, documenting...'?"

> "Proverbially, to get an answer to that, the question to ask is:
> 'I know that you are done, but are you DONE-done?'"

**SIN-DoDone überträgt DONE-done auf KI-Agenten.** KI lügen häufiger als
Menschen — sie sagen "fertig" weil das die erwartete Antwort ist, nicht weil
es wahr ist.

## Die 7 Säulen

### Säule 1: Keine Platzhalter (Anti-Cheat-Pattern)

KI-Agenten neigen zu:
- `// TODO: implement this`
- `// FIXME: handle edge case`
- `panic("not implemented")`
- `pass  # TODO`
- `throw new Error("not implemented")`
- `// Hier Logik einfügen`
- Leere Funktionskörper mit nur `return nil`

**Check:** `grep -rn` über alle Source-Files nach verbotenen Patterns.
**Result:** Deterministisch — Pattern gefunden = FAIL, nicht gefunden = PASS.

### Säule 2: Fehlerpfade echt behandelt

KI-Agenten ignorieren gerne Fehler:
- Go: `_ = err` oder `if err != nil { return nil }` (ohne Logging/Propagation)
- Python: `except: pass` oder `except Exception: pass`
- JS: `catch (e) { }` oder `catch (e) { console.log(e) }` (nur log, keine Recovery)
- Rust: `unwrap()` ohne Fehlerbehandlung

**Check:** Regex-Pattern-Scan nach Ignoranz-Patterns + manuelle Audit-Liste.
**Result:** Semi-deterministisch — Pattern-Scan ist maschinell, Quality-Review
kann vom self-review-Skill übernommen werden.

### Säule 3: Tests existieren und sind grün

Die stärkste Säule. Ein grüner Test beweist, dass der Code *etwas* tut.
Kein Test = keine Beweis = nicht fertig.

**Check:** `go test ./... -v -count=1` / `pytest -v` / `npm test --verbose`
**Result:** Exit-Code des Test-Runners — 0 = PASS, ≠0 = FAIL.

### Säule 4: Build/Lint sauber

Ein roter Build = nicht fertig. Ein roter Lint = wahrscheinlich nicht fertig.

**Check:** `go build ./...` + `go vet ./...` / `ruff check .` / `eslint .`
**Result:** Exit-Code — 0 = PASS, ≠0 = FAIL.

### Säule 5: Erforderliche Artefakte vorhanden

Manche Dinge müssen existieren, damit ein Projekt "fertig" ist:
- README.md (immer)
- CHANGELOG-Eintrag (bei behavioral changes)
- API-Dokumentation (bei neuen Endpoints)
- Migration-Files (bei DB-Änderungen)
- .gitignore (bei neuen Artefakten)

**Check:** `os.Stat()` auf jede erforderliche Datei.
**Result:** Deterministisch — existiert = PASS, fehlt = FAIL.

### Säule 6: Anforderungen vollständig abgedeckt

Die wichtigste Säule gegen "ja ist alles fertig"-Lügen. Jede Anforderung
aus der ursprünglichen Task-Beschreibung muss nachvollziehbar umgesetzt sein.

**Methode:** Der DoD-Vertrag enthält eine Requirements-Checklist. Der Agent
muss jede Anforderung mit einem Datei:Zeile-Beleg verknüpfen. Der Check
verifiziert, dass der Beleg existiert.

**Result:** Deterministisch — Beleg existiert = PASS, fehlt = FAIL.

### Säule 7: Kein toter Code

- Keine ungenutzten Imports
- Keine auskommentierten Code-Blöcke (außer sin-debt-Marker)
- Keine Funktionen die nirgendwo aufgerufen werden

**Check:** Language-spezifische Tools (`go vet`, `ruff --select F401`, etc.)
**Result:** Tool-basiert — deterministisch.

## Priorisierung

Wenn Zeit/Token-Budget begrenzt ist, priorisiere:

1. **Säule 3** (Tests grün) — stärkster Beweis
2. **Säule 1** (Keine Platzhalter) — schnellster Check, höchste Trefferquote
3. **Säule 4** (Build/Lint) — deterministisch, schnell
4. **Säule 5** (Artefakte) — deterministisch, schnell
5. **Säule 2** (Fehlerpfade) — semi-deterministisch
6. **Säule 6** (Anforderungen) — braucht DoD-Vertrag
7. **Säule 7** (Toter Code) — nice-to-have

## Was SIN-DoDone NICHT prüft

- **Logik-Korrektheit:** Ob der Code das *Richtige* tut, prüfen PoC/Oracle (M3).
- **Design-Qualität:** Ob der Code *gut* ist, prüft skill-code-lazy / review.
- **Sicherheit:** Ob der Code *sicher* ist, prüft security_scan (Säule ≠ DoD).
- **Performance:** Ob der Code *schnell* ist, prüft benchmark.

SIN-DoDone prüft nur: **Ist es VOLLSTÄNDIG?** — nicht ob es gut/schnell/sicher ist.
