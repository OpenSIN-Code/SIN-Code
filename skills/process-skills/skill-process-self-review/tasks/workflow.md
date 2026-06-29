# Workflow

1. Arbeitsumfang rekonstruieren (git status/diff)
2. Anforderungsabgleich (jede User-Anforderung prüfen)
3. Datei-für-Datei-Prüfung (gegen checklist.md)
4. Verifikation ausführen (scripts/verify.sh)
5. Severity-Bewertung (BLOCKER/MAJOR/MINOR/NIT)
6. Bericht + Korrektur + Re-Verifikation (Schleife bis 0 offene BLOCKER/MAJOR)

Abgeschlossen nur bei: 0 BLOCKER + 0 MAJOR + verify.sh grün + Bericht ausgegeben.
