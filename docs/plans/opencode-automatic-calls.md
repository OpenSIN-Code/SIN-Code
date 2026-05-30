# Plan: Automatische Calls in OpenCode (inject · reflect · gates)

> Ziel: „Automatische Tool-Calls" wie bei Hermes auch in der OpenCode-CLI
> realisieren — über die bestehenden `.opencode/plugins` und `hooks`.
> Drei Auslöser: Start (deterministisch), Agent-initiiert (MCP), Background (sleep-time).

## Statusanalyse (Ist)

`.opencode/plugins/*.js` + `hooks/pcpm-*.sh` existieren bereits (PCPM, graphify).
Sie laden aber kein Memory automatisch und triggern keine Verdikt-Rückkopplung.

## Änderungen

### 1. Start-Hook → `sin-brain inject`
`hooks/pcpm-before-run.sh` ruft `sin-brain inject --branch <b> --files <changed>`
und schreibt das Ergebnis in den SIN-Block der `AGENTS.md` (zwischen
`<!-- sin:start -->/<!-- sin:end -->`). Damit ist Memory present, bevor der erste
Tool-Call passiert. *(Hermes-Pattern.)*

### 2. After-Run-Hook → `reflect` (async) + Quality-Gate
`hooks/pcpm-after-run.sh`:
- startet `sin-brain reflect --session <id> &` (nicht blockierend, sleep-time).
- ruft `sin verify_tests` (Oracle) als deterministisches Gate nach File-Writes;
  bei Rot wird das Verdikt via `link_evidence` persistiert.

### 3. graphify-Plugin → `link_evidence`
Das bestehende graphify-Plugin schreibt Graph-Updates künftig in den
Evidence-Graph (über `link_evidence`) statt in eine isolierte Struktur.

### 4. Deterministische Gate-Kette (Best-Practice)
Nach jedem File-Write: Syntax → Lint (`ruff`) → Types → betroffene Tests.
Fehler kompoundieren nicht. Verdikte landen im Evidence-Graph.

## Akzeptanz
- Neue Session lädt automatisch Core-Memory in den Kontext (ohne manuellen Call).
- Nach Session-Ende existiert mindestens ein neuer/aktualisierter Memory-Eintrag.
- Rote Gates blockieren den Merge und sind im Graph nachvollziehbar.

## Nicht-Ziele
- Keine Änderung am Modell-/Prompt-Routing von OpenCode selbst.
- Keine neuen Plugins jenseits von Memory/Gates.
