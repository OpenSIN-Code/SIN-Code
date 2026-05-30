# Plan: Memory-Tools in `sin serve` integrieren

> Ziel: Die SIN-Brain-Memory-Fähigkeiten als MCP-Tools im bestehenden
> `sin serve` registrieren, damit OpenCode/Codex/Hermes sie automatisch rufen.
> Baut auf WS1 (#1, MCP-Tool-Surface) auf. Voraussetzung: SIN-Brain SB-2.

## Statusanalyse (Ist)

`sin serve` registriert die Coding-Subsysteme defensiv (try/except ImportError).
Es gibt **keine** Memory-Tools. Agenten können weder erinnern noch lernen.

## Änderungen

### 1. Tool-Registrierung (defensiv)
In der `serve`-Registrierung zusätzlich, jeweils nur falls `sin_brain` importierbar:
- `recall(query, scope="recall"|"archival"|"graph", k=5)` → Tier-Suche.
- `remember(content, kind, ttl_days=None, scope="repo"|"user")` →
  self-editing write. `kind ∈ {decision, convention, fix, pitfall, preference}`.
- `forget(id)` / `pin(id)` → Memory-Hygiene.
- `link_evidence(entity, verdict, source)` → Verdikt eines Subsystems an eine
  Code-Entität im Evidence-Graph knüpfen. `source ∈ {oracle, poc, ibd, sckg, adw}`.

Jedes Tool delegiert an `sin_brain.mcp_tools`. Fehlt das Paket, wird das Tool
ohne Crash übersprungen (Graceful Degradation, wie bei den übrigen Tools).

### 2. `sin status`
`sin-brain` als Subsystem aufnehmen (installiert / nicht installiert) inkl.
Memory-DB-Pfad und Tier-Größen (Anzahl Core-Blöcke, Recall-Einträge).

### 3. Schmale, token-effiziente Tool-Definitionen
Verb-first, enum-constrained Parameter, knappe Descriptions (MCP-Best-Practice:
jede Tool-Definition kostet Tokens pro Turn). Output-Kompression: `recall`
liefert IDs + Snippets, nicht ganze Dokumente.

## Akzeptanz
- `sin serve` listet `recall`, `remember`, `forget`, `pin`, `link_evidence`,
  sobald `sin-brain` installiert ist; ohne Paket startet der Server normal weiter.
- `sin status` zeigt `sin-brain` mit Tier-Kennzahlen.
- Subsysteme können via `link_evidence` Verdikte persistieren.

## Nicht-Ziele
- Keine Memory-Logik im Bundle selbst (lebt in SIN-Brain).
- Keine neuen Transporte (stdio bleibt, siehe WS3 #3).
