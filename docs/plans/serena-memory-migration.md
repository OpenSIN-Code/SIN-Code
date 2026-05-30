# Plan: Serena vollständig ablösen (Memory → SIN-Brain, Symbol-Tools → Simone)

> Ziel: Serena hat **zwei** Rollen, die beide besser woanders aufgehoben sind:
> 1. **Memory** (`.serena/memories/*.md`, passiv) → **SIN-Brain** (aktiv, self-editing).
> 2. **Symbol-/Code-Tools** (`find_symbol`, `replace_symbol_body`, …) → **Simone-MCP**
>    (chirurgische AST-Edits, bereits im Stack).
> Nach der Migration gibt es kein paralleles passives Memory und keine doppelte
> Symbol-Tool-Schicht mehr. Serena wird entfernt; **Simone bleibt der WRITE-Layer.**

## Statusanalyse (Ist)

`.serena/memories/*.md` werden gelesen, aber nie automatisch fortgeschrieben →
Drift. Zusätzlich überlappen Serenas Symbol-Tools mit Simone-MCP (Redundanz im
Edit-Layer, doppelte Tool-Definitionen kosten Tokens und verwirren das Routing).

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

### 3. Symbol-/Code-Tools auf Simone umverdrahten
- Alle Aufrufer/Hooks, die Serenas `find_symbol`/`replace_symbol_body`/
  `insert_*`/`delete_symbol`/`rename_symbol`/`search_for_pattern` nutzen, auf
  die entsprechenden **Simone-MCP**-Tools umstellen (1:1-Mapping dokumentieren).
- Serena-MCP-Eintrag aus den Configs entfernen (siehe `cli-mcp-consolidation.md`).
- AGENTS.md: „WRITE/Edit → immer Simone" als Routing-Regel verankern.

### 4. Verifikation
`sin status` zeigt die importierten Memory-Einträge; Stichprobe via
`sin-brain recall`. Edit-Smoke-Test läuft ausschließlich über Simone.

## Akzeptanz
- Alle relevanten Serena-Memory-Inhalte sind in SIN-Brain abrufbar.
- Kein Code-Pfad schreibt mehr aktiv nach `.serena/memories/`.
- Keine Serena-Symbol-Tools mehr in Configs/Hooks; Edits laufen über Simone.
- 1:1-Mapping Serena-Tool → Simone-Tool ist dokumentiert.
- Wiederholter Migrationslauf ist idempotent.

## Nicht-Ziele
- **Simone-MCP wird NICHT angefasst/ersetzt** — es übernimmt Serenas Edit-Rolle.
- Kein Löschen der Originale (Archiv statt Delete, zur Sicherheit).
- Keine Migration nicht-memory-bezogener Serena-Konfiguration.
