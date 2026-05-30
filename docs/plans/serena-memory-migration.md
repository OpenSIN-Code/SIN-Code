# Plan: Serena-Memories → SIN-Brain migrieren

> Ziel: Die passiven `.serena/memories/*.md` als einmalige Seed-Quelle in
> SIN-Brain importieren und danach SIN-Brain als Single Source of Truth setzen.
> Kein paralleles, passives Zweit-Memory mehr.

## Statusanalyse (Ist)

`.serena/memories/*.md` werden gelesen, aber nie automatisch fortgeschrieben.
Es entsteht Drift zwischen Notizen und tatsächlichem Code-/Entscheidungsstand.

## Änderungen

### 1. Migrations-Skript (`scripts/migrate_serena.py` in SIN-Brain)
- Liest alle `.serena/memories/*.md`, klassifiziert grob nach `kind`
  (convention/decision/preference) heuristisch + optional LLM-gestützt.
- Schreibt sie via `remember(..., scope="repo")` in Recall/Core.
- Idempotent: Hash-basierte Dedupe, erneuter Lauf erzeugt keine Duplikate.

### 2. Read-only-Schalter
Nach erfolgreicher Migration werden die Serena-Dateien als read-only Seed
markiert (oder nach `docs/legacy/serena-seed/` archiviert). Schreiben erfolgt nur
noch über SIN-Brain.

### 3. Verifikation
`sin status` zeigt die importierten Einträge; Stichprobe via `sin-brain recall`.

## Akzeptanz
- Alle relevanten Serena-Inhalte sind in SIN-Brain abrufbar.
- Kein Code-Pfad schreibt mehr aktiv nach `.serena/memories/`.
- Wiederholter Migrationslauf ist idempotent.

## Nicht-Ziele
- Kein Löschen der Originale (Archiv statt Delete, zur Sicherheit).
- Keine Migration nicht-memory-bezogener Serena-Konfiguration.
