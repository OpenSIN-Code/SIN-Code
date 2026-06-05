# EXPLAINED — Detaillierte Erklärungen

Diese Sektion enthält ausführliche Hintergrund-Erklärungen zu Konzepten,
die in den SIN-Code-Bundle-Workflows eine Rolle spielen. Jedes Dokument
ist auf Deutsch, detailreich und für Maintainer gedacht, die verstehen
wollen, *warum* etwas so funktioniert wie es funktioniert.

## Dokumente

| Datei | Thema | Wann lesen? |
|---|---|---|
| [EXPLAINED-pypi-trusted-publisher.md](./EXPLAINED-pypi-trusted-publisher.md) | Wie PyPI Trusted Publishing funktioniert, warum unser `release.yml` derzeit fehlschlägt, und wie der Maintainer es in 1 Minute einmalig fixt. | Wenn du ein Release auf PyPI pushen willst, oder der `release.yml` Job failed. |
| [EXPLAINED-stale-bulk-deploy-tags.md](./EXPLAINED-stale-bulk-deploy-tags.md) | Was der `v0.5.0-ceo-audit-v3` Tag auf 19 OpenSIN-Code Repos bedeutet, warum er kein Versions-Tag ist, und wie man die echte Repo-Version findet. | Wenn du `git tag` machst und dich wunderst, was der seltsame Tag ist. |

## Konvention für neue EXPLAINED-*.md

- Dateinamen beginnen mit `EXPLAINED-<topic-slug>.md`
- Sprache: Deutsch (User schreibt Deutsch)
- Inhalt: technisch korrekt, mit Beispielen, am Ende immer ein TL;DR
- Keine Code-Änderungen — nur Doku
- Werden in dieser README verlinkt (Tabelle oben)
