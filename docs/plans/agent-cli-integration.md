# Plan: Agent-CLI-Integration (OpenCode · Codex · Hermes)

> Ziel: Den SIN-Code-Stack über MCP vollständig und reibungslos für die drei
> Agent-CLIs **OpenCode**, **OpenAI Codex** und **Hermes** nutzbar machen.
> Scope-Regel: ausschließlich Lücken schließen, die diese CLIs verbessern.
> Nichts Spekulatives, nichts Doppeltes.

## Statusanalyse (Ist)

`sin` verdrahtet SCKG, IBD, POC, EFSM, ADW, Oracle. Der MCP-Server (`sin serve`)
exponiert jedoch nur **3** Tools (`impact`, `semantic_diff`, `architectural_debt`).
`SIN-Code-Orchestration` und `SIN-Code-Review-Interface` sind nicht verdrahtet.

## Workstreams

### WS1 — MCP-Tool-Surface vervollständigen  (Issue #1)
Alle vorhandenen Fähigkeiten als MCP-Tools im `sin serve` registrieren, jeweils
defensiv (try/except ImportError), damit Graceful Degradation erhalten bleibt:
- `verify_tests` → Oracle (`VerificationOracle.verify`)
- `prove` → POC (Proof-of-Correctness)
- `mock_env` → EFSM (ephemere Mock-Umgebung hoch-/runterfahren)
- `orchestrate` / `task_status` → Orchestration (Shared Context Store)
- `semantic_review` → Review-Interface (IBD-Intent + Risk in einem Aufruf)
Akzeptanz: `sin serve` listet alle Tools installierter Subsysteme; fehlende werden
ohne Crash übersprungen.

### WS2 — `sin mcp-config <client>` Generator  (Issue #2)
Neuer Befehl, der eine fertige, einfügbare MCP-Client-Konfiguration ausgibt:
- `opencode`  → JSON: Key `mcp`, `type: "local"`, `command: ["sin","serve"]`, `environment`, `enabled`
- `codex`     → TOML: `[mcp_servers.sin]` mit `command = "sin"`, `args = ["serve"]`, `[mcp_servers.sin.env]`
- `hermes`    → YAML: `mcp_servers.sin` mit `command`/`args`, optional `tools.include`
Flags: `--stdout` (default) bzw. `--write` (in die jeweilige Config-Datei mergen).
Akzeptanz: Ausgabe ist 1:1 gültig für die offiziell dokumentierten Formate.

### WS3 — stdio-Hygiene für `sin serve`  (Issue #3)
- Alle Status-/Banner-Ausgaben auf **stderr** (nie stdout) — stdout ist reserviert
  für den JSON-RPC-Stream.
- Irreführendes `--port` aus dem stdio-Pfad entfernen bzw. nur für HTTP-Transport
  zulassen.
Akzeptanz: `sin serve | head` gibt kein Nicht-Protokoll-Rauschen auf stdout aus;
opencode/codex/hermes verbinden ohne Handshake-Fehler.

### WS4 — `sin agents-md` Generator  (Issue #4)
Erzeugt/aktualisiert eine `AGENTS.md` im Repo (von OpenCode & Codex auto-gelesen),
die dem Agenten erklärt, **wann** welches SIN-Tool zu rufen ist
(z. B. „vor Refactor: `impact`", „nach Diff: `semantic_review`", „vor Merge:
`verify_tests`"). Idempotent zwischen Markern `<!-- sin:start -->/<!-- sin:end -->`.
Akzeptanz: erneuter Aufruf verändert nur den SIN-Block, nicht den Rest der Datei.

### WS5 — Neue Subsysteme verdrahten  (Issue #5)
`SIN-Code-Orchestration` und `SIN-Code-Review-Interface` in `status` aufnehmen,
in `pyproject.toml` als editable-Install-Reihenfolge dokumentieren, und in
`serve` (über WS1) berücksichtigen.
Akzeptanz: `sin status` zeigt beide an (installiert/nicht installiert).

## Reihenfolge
WS5 → WS1 → WS3 → WS2 → WS4 (Verdrahtung zuerst, dann Tools, dann saubere I/O,
dann Onboarding-Helfer).

## Nicht-Ziele
- Keine neuen Subsysteme/Algorithmen.
- Keine HTTP-/SSE-Transporte, solange die Ziel-CLIs stdio-lokal nutzen (außer Hermes-HTTP als reine Doku-Notiz).
- Keine Web-UI-Erweiterungen am Review-Interface.
