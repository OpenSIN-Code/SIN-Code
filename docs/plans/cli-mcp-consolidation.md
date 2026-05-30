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

### 1. Server pro CLI (Whitelist statt „nur einer")
Erlaubt sind **zwei** kanonische Server; alles andere wird entfernt:
- **`sin`** — einheitliche Tür (READ/SCKG, VERIFY/Oracle+POC, REMEMBER/SIN-Brain,
  Orchestration, Review). Erzeugt via `sin mcp-config <cli> --write`.
- **`simone`** — kanonischer **WRITE/Edit-Layer** (chirurgische AST-Symbol-Edits:
  `find_symbol`, `replace_symbol_body`, `insert_before/after_symbol`,
  `delete_symbol`, `rename_symbol`, `search_for_pattern`). **Bleibt erhalten —
  das ist der Moat, kein schwaches Tool.**

Pro CLI:
- OpenCode: `opencode.json` → genau die Einträge `sin` + `simone`
  (`type: "local"`). Schwache/redundante MCP-Einträge entfernen.
- Codex: `[mcp_servers.sin]` + `[mcp_servers.simone]`.
- Hermes: `mcp_servers.sin` + `mcp_servers.simone`.

> **Whitelist (nicht ersetzen):** `sin`, `simone`.
> **Zu ersetzen/entfernen:** Serena (Memory → SIN-Brain, Symbol-Tools → Simone)
> und alle schwachen Ad-hoc-MCPs.

### Rollentrennung (verbindlich)
| Schicht | Owner | Hinweis |
|---------|-------|---------|
| READ / Verstehen | SCKG (`sin`) | Knowledge-Graph |
| WRITE / Editieren | **Simone-MCP** | AST-Symbol-Edits — Moat |
| REMEMBER | SIN-Brain (`sin`) | Memory-Cortex |
| VERIFY | Oracle/POC (`sin`) | Quality-Gates |

### 2. Eine `AGENTS.md` als Steuerkanal
`sin agents-md` erzeugt den idempotenten SIN-Block inkl. „wann welches Tool"
und eingebettetem `sin-brain inject`-Output. Redundante Mandate/CLAUDE.md-Teile,
die das duplizieren, entfernen. **Negative Constraints** ergänzen (Red-Zones,
„keine fremden Dateien ändern") — robuster als positive Direktiven.

### 3. stdio-Hygiene als Voraussetzung
WS3 (#3) muss greifen: stdout nur JSON-RPC, Banner auf stderr. Sonst verbinden
die drei CLIs nicht sauber.

## Akzeptanz
- Jede der drei CLIs verbindet sich mit genau den Servern `sin` + `simone`.
- `sin` zeigt 3–7 fokussierte Tools (inkl. Memory-Tools); `simone` die AST-Edit-Tools.
- Keine alten/schwachen MCP-Einträge mehr in den Configs.
- Eine gemeinsame `AGENTS.md` steuert alle drei CLIs.

## Nicht-Ziele
- **Simone-MCP wird NICHT ersetzt** — es ist der kanonische WRITE-Layer (siehe BR-6).
- Keine Entfernung legitimer, vom Nutzer bewusst gewünschter externer MCPs.
- Keine Format-Erfindungen — nur offiziell dokumentierte Config-Formate.
