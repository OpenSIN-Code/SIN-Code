# Review-Checkliste

## Vollständigkeit
- [ ] Jede User-Anforderung ist umgesetzt (nicht nur angefangen).
- [ ] Keine TODO / FIXME / "später" / auskommentierter Platzhaltercode.
- [ ] Keine Dummy-/Mock-/Hardcoded-Daten, wo echte Logik gefordert war.
- [ ] Alle neuen Komponenten/Module sind eingebunden und erreichbar.
- [ ] Keine toten Dateien, ungenutzten Exports oder verwaisten Imports.

## Korrektheit
- [ ] Logik tut, was sie behauptet (Zeile für Zeile gelesen).
- [ ] Edge Cases: leere Werte, null/undefined, leere Listen, Grenzwerte.
- [ ] Fehlerpfade behandelt (try/catch, Rückgabewerte, Statuscodes).
- [ ] Keine Off-by-one-, Race-Condition- oder async/await-Fehler.
- [ ] Datentypen/Schnittstellen passen über Dateigrenzen zusammen.

## Subagent-Konsistenz
- [ ] Ergebnisse mehrerer Subagents passen widerspruchsfrei zusammen.
- [ ] Einheitliche Benennung, Typen, Datei-/Ordnerstruktur.
- [ ] Keine doppelte oder konkurrierende Implementierung derselben Sache.
- [ ] Keine widersprüchlichen Annahmen über gemeinsame Schnittstellen.

## Sicherheit
- [ ] Keine Secrets/Keys im Code.
- [ ] Eingaben validiert/sanitisiert; parametrisierte Queries.
- [ ] Auth-/Zugriffsprüfungen vorhanden, wo nötig.

## Qualität
- [ ] Lesbar, sinnvoll benannt, keine offensichtliche Duplizierung.
- [ ] Keine deaktivierten Tests/Lint-Regeln zum "Grünmachen".
- [ ] Kommentare beschreiben das Warum, nicht offensichtliches Wie.

## Verifikation (real ausgeführt via scripts/verify.sh)
- [ ] Build/Typecheck sauber.
- [ ] Linter ohne neue Fehler.
- [ ] Tests grün (oder Fehlen begründet).
- [ ] git diff stimmt mit beabsichtigtem Umfang überein.
