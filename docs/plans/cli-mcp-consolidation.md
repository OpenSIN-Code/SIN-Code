# Plan: CLI-/MCP-Konsolidierung — schwache Tools ersetzen

> Ziel: Alte, kleine, parallele MCP-Server und passive Tool-CLIs vollständig
> durch **einen** `sin`-MCP-Server ersetzen. Jede Ziel-CLI verbindet sich mit
> genau einem Server, liest eine `AGENTS.md`, teilt ein Gedächtnis.
> Baut auf WS2 (#2, `sin mcp-config`) und WS4 (#4, `sin agents-md`) auf.

## Statusanalyse (Ist)

`.opencode/opencode.json` und vergleichbare Configs enthalten potenziell mehrere
schwache MCP-Einträge. MCP-Best-Practice 2026: 3–5 Server, 3–7 Tools — jede
Tool-Definition kostet 500–1000 Tokens pro Turn und verschlechtert das Reasoning.

## Änderungen

### 1. Einheitlicher Server pro CLI
- OpenCode: `opencode.json` → genau **ein** Eintrag `sin`
  (`type: "local"`, `command: ["sin","serve"]`), erzeugt via
  `sin mcp-config opencode --write`. Schwache/redundante MCP-Einträge entfernen.
- Codex: `[mcp_servers.sin]` via `sin mcp-config codex --write`.
- Hermes: `mcp_servers.sin` via `sin mcp-config hermes --write`.

### 2. Eine `AGENTS.md` als Steuerkanal
`sin agents-md` erzeugt den idempotenten SIN-Block inkl. „wann welches Tool"
und eingebettetem `sin-brain inject`-Output. Redundante Mandate/CLAUDE.md-Teile,
die das duplizieren, entfernen. **Negative Constraints** ergänzen (Red-Zones,
„keine fremden Dateien ändern") — robuster als positive Direktiven.

### 3. stdio-Hygiene als Voraussetzung
WS3 (#3) muss greifen: stdout nur JSON-RPC, Banner auf stderr. Sonst verbinden
die drei CLIs nicht sauber.

## Akzeptanz
- Jede der drei CLIs verbindet sich mit nur dem `sin`-Server und sieht 3–7
  fokussierte Tools (inkl. Memory-Tools).
- Keine alten/schwachen MCP-Einträge mehr in den Configs.
- Eine gemeinsame `AGENTS.md` steuert alle drei CLIs.

## Nicht-Ziele
- Keine Entfernung legitimer, vom Nutzer bewusst gewünschter externer MCPs.
- Keine Format-Erfindungen — nur offiziell dokumentierte Config-Formate.
