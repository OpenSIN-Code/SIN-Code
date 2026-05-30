# Plan: Simone-MCP als kanonischer WRITE-/Edit-Layer

> Ziel: Simone-MCP (`Delqhi/Simone-MCP`) explizit als **den** Edit-Layer des
> SIN-Stacks verankern, mit SCKG füttern und in den Lern-Loop von SIN-Brain
> einhängen. Simone wird NICHT ersetzt — es ist der Moat gegenüber
> text-diff-basierten Agenten (Aider/Cline/OpenHands).

## Statusanalyse (Ist)

Simone-MCP ist bereits im Stack (`bin/activate_simone`, A2A-Agent auf Port 8000,
`docs/components/simone-mcp.md`) und bietet chirurgische AST-Symbol-Edits:
`find_symbol`, `replace_symbol_body`, `insert_before_symbol`,
`insert_after_symbol`, `delete_symbol`, `rename_symbol`, `search_for_pattern`
(tree-sitter, multi-language). Bekannte Roadmap-Lücke (v0.2.0): „Symbol-graph
index for repo-wide refactors" — genau das kann SCKG liefern.

## Rolle im Gesamtsystem

| Schicht | Owner |
|---------|-------|
| READ / Verstehen | SCKG (`sin`) |
| **WRITE / Editieren** | **Simone-MCP** |
| REMEMBER | SIN-Brain (`sin`) |
| VERIFY | Oracle/POC (`sin`) |

## Änderungen

### 1. Config-Whitelist (mit `cli-mcp-consolidation.md`)
`simone` wird neben `sin` als zweiter kanonischer MCP-Server in allen drei CLIs
geführt. `sin mcp-config <cli>` gibt optional auch den `simone`-Eintrag aus
(Flag `--with-simone`, Default an).

### 2. SCKG → Simone Symbol-Index (schließt Simone-Roadmap-Lücke)
SCKG exportiert seinen Symbol-Graph in ein von Simone konsumierbares Format,
sodass repo-weite Refactors (z. B. `rename_symbol` über alle Referenzen)
graph-gestützt statt rein lokal laufen. Definiert ein stabiles Austauschschema.

### 3. Simone-Edits → SIN-Brain Evidence-Graph
Nach jedem mutierenden Simone-Call wird ein `link_evidence`-Event erzeugt
(welches Symbol, welche Datei, welcher Edit-Typ, anschließendes Oracle-Verdikt).
Damit fließen Edits in den Lern-Loop (bessere Pitfalls/Conventions).

### 4. Routing-Regel in AGENTS.md
„Jede Code-Änderung erfolgt über Simone-Symbol-Tools, nicht über rohe
Text-Patches." Negative Constraint ergänzen: keine Ad-hoc-Text-Diffs für
strukturierte Edits.

## Akzeptanz
- `simone` erscheint in den generierten Configs aller drei CLIs.
- SCKG liefert einen Symbol-Index, den ein Simone-Refactor nutzen kann (Smoke-Test).
- Mutierender Simone-Call erzeugt ein `link_evidence`-Event in SIN-Brain.
- AGENTS.md enthält die WRITE-→-Simone-Routing-Regel.

## Nicht-Ziele
- Keine Neuimplementierung von Simones Edit-Engine.
- Keine Änderung an Simones A2A-Schnittstelle über das Nötige hinaus.
- Kein Ersetzen von Simone durch SCKG (READ ≠ WRITE).
