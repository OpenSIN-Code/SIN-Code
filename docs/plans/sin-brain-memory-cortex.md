# Plan: SIN-Brain — Memory-Cortex & automatische Tool-Calls

> Ziel: Ein neues Repo **`OpenSIN-Code/SIN-Brain`** als persistentes, selbst-
> editierendes, evidenzgestütztes Gedächtnis für den SIN-Code-Agenten-Stack.
> Es schließt den **Closed-Learning-Loop**, der heute fehlt, und ersetzt die
> passiven, dateibasierten Memory-Quellen (Serena-Memories, lose `MEMORY.md`).
> Scope-Regel: ausschließlich (a) automatische Tool-Calls, (b) Memory-System,
> (c) Coding-Quality-Best-Practices. Nichts darüber hinaus.

## Statusanalyse (Ist)

- Memory ist **passiv**: `.serena/memories/*.md` werden gelesen, aber nie
  automatisch geschrieben oder verdichtet. Kein Lern-Loop über Sessions.
- `sin serve` exponiert keine Memory-Tools. Agenten können nicht „erinnern".
- Verdikte von Oracle/POC/IBD/SCKG/ADW fließen nirgends zurück → jedes Subsystem
  urteilt isoliert, ohne gemeinsames Gedächtnis.
- Bundle-Tracking #9-WS8 („Persistent cross-task learning") ist eine Mini-Form
  dessen, was SIN-Brain vollständig liefert. **SIN-Brain implementiert WS8.**

## Vergleich mit dem Stand der Technik (Recherche)

| Muster | Vorbild | Übernahme in SIN-Brain |
| --- | --- | --- |
| Self-editing memory als Tool-Call | Letta / MemGPT | `remember`/`recall`/`forget` MCP-Tools |
| Sleep-time / Background-Reflection | Letta `enable_sleeptime`, Codex Consolidation | `sin-brain reflect` (asynchron) |
| Memory-Injection beim Start | Hermes (`MEMORY.md`/`USER.md`) | `sin-brain inject` (evidence-aware) |
| Hierarchische Tiers (core/recall/archival) | Letta | 4-Tier-Modell (+ Evidence Graph) |
| Bitemporaler Knowledge-Graph | Zep / Graphiti | Evidence-Graph mit Gültigkeitsfenstern |
| Roh-Episoden statt nur LLM-Extrakt | MemMachine vs. Mem0 | Archival speichert Roh + Verdichtung |

## Architektur — vier Tiers

| Tier | Inhalt | Speicher | Auto-Call |
| --- | --- | --- | --- |
| **Core** | Aktive Repo-Konventionen, „immer/nie"-Regeln, Task-Constraints | System-Prompt-Injektion (≤ ~1500 tok) | Start-Hook |
| **Recall** | Letzte Sessions, jüngste Fixes/Fehler, Diffs | SQLite (FTS5) + Vektor | `recall()` |
| **Archival** | Vollständiges Episoden-Log (roh) + Verdichtungen | SQLite/Blob | `recall(scope=archival)` |
| **Evidence Graph** | Bitemporaler Graph: Code-Entitäten + Verdikte (Oracle/POC/IBD/SCKG/ADW) | Graph (Kùzu/SQLite) | gespeist von `sin`-Subsystemen |

Der **Evidence Graph** ist die Meta-Innovation: er macht aus sechs isolierten
Subsystemen ein lernendes System mit gemeinsamem Gedächtnis.

## Die drei automatischen Call-Mechanismen

1. **`sin-brain inject`** (deterministisch, Start-Hook) — rendert Core-Memory +
   für Branch/Files relevante Evidence-Fakten als Markdown auf stdout → wird in
   `AGENTS.md` / System-Prompt eingespeist. *(Hermes-Pattern, evidence-aware.)*
2. **MCP-Memory-Tools** (Agent-initiiert) in `sin serve`: `recall`, `remember`,
   `forget`, `pin`, `link_evidence`. Schmal, verb-first, enum-constrained.
   *(Letta-Pattern, MCP-Best-Practice.)*
3. **`sin-brain reflect`** (asynchron, Sleep-time) — nach Session-Ende: ein
   Reflection-Subagent extrahiert „gut/schlecht", dedupliziert, verdichtet
   Recall→Core, schreibt Pitfalls in den Graph. Nie im Hauptpfad.
   *(Letta sleep-time + Codex consolidation.)*

## Closed Loop (der eigentliche Hebel)

`Oracle/POC-Verdikt` → `link_evidence` → `reflect` verdichtet zu Convention/Pitfall
→ `inject` beim nächsten Start → besserer Plan. So lernt der Agent über Sessions.

## Repo-Struktur (SIN-Brain)

```
SIN-Brain/
  src/sin_brain/
    tiers/        core.py · recall.py · archival.py · evidence_graph.py
    store/        sqlite.py (FTS5) · vector.py · graph.py (bitemporal)
    pipeline/     extract.py · consolidate.py · dedupe.py   # reflect
    inject.py     # start-hook renderer (evidence-aware)
    mcp_tools.py  # recall/remember/forget/pin/link_evidence
    cli.py        # sin-brain inject|recall|remember|reflect|status
  adapters/       opencode_hook.* · codex_agents_md.* · hermes_preset.*
  tests/
  docs/plans/     01..06 (siehe SIN-Brain-Repo)
```

Gleiche Konventionen wie die übrigen Subsysteme: defensive Imports, Graceful
Degradation, `pyproject.toml` editable-install → `sin status` erkennt es.

## Workstreams (Detailpläne im SIN-Brain-Repo)

- **SB-1 — Core/Recall/Archival Tiers + Stores** (`docs/plans/01-core-tiers.md`)
- **SB-2 — Memory-MCP-Tools** (`docs/plans/02-memory-mcp-tools.md`)
- **SB-3 — Reflect-Pipeline (sleep-time)** (`docs/plans/03-reflect-pipeline.md`)
- **SB-4 — Evidence Graph (bitemporal)** (`docs/plans/04-evidence-graph.md`)
- **SB-5 — `inject` Start-Hook** (`docs/plans/05-inject-start-hook.md`)
- **SB-6 — CLI-Adapter (opencode/codex/hermes)** (`docs/plans/06-cli-adapters.md`)

## Bundle-/CLI-seitige Begleitpläne

- `docs/plans/bundle-memory-tools-integration.md` — Memory-Tools in `sin serve`.
- `docs/plans/opencode-automatic-calls.md` — Hooks für inject/reflect/gates.
- `docs/plans/cli-mcp-consolidation.md` — schwache MCPs durch einen `sin`-Server ersetzen.
- `docs/plans/serena-memory-migration.md` — Serena-Memories → SIN-Brain migrieren.

## Reihenfolge

SB-1 → SB-2 → SB-5 (`inject`) → SB-3 (`reflect`) → SB-4 (Evidence Graph) → SB-6
(Adapter). Bundle-Integration (Memory-Tools) parallel zu SB-2. OpenCode-Hooks
nach SB-5/SB-3. Serena-Migration zuletzt.

## Nicht-Ziele

- Keine neuen Coding-Subsysteme/Algorithmen jenseits von Memory.
- Keine eigenständige Web-UI (Review-Interface bleibt zuständig).
- Kein paralleles zweites Memory: nach Migration ist SIN-Brain Single Source of Truth.
