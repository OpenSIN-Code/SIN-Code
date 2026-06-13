# `internal/vane` — Vane Self-Hosted AI-Answering Bridge

## Was

HTTP-Bridge von SIN-Code zu einer **selbstgehosteten Vane AI-Answering-Engine**
(MIT-lizenziert, [github.com/thevahber/vane](https://github.com/thevahber/vane)).
Vane ist eine Perplexica-ähnliche Open-Source-"ask-the-web"-Engine mit
Focus-Modi (webSearch / academicSearch / writingAssistant /
wolframAlphaSearch / youtubeSearch / redditSearch) und liefert Antworten
mit zitierfähigen Quellen zurück. SIN-Code ruft Vane über den
`/api/search`-Endpoint auf und reicht das Ergebnis an den Agent-Layer
weiter — als MCP-Tool (`vane_research`) und als Cobra-Subcommand
(`sin-code vane`).

## Warum NIE vendored

**Mandate M2 (single static Go binary)** verbietet das Einchecken einer
Node/Python-Engine in `internal/`. Vane läuft deshalb als **separater
Prozess** (typischerweise via Docker: `docker run -p 3000:3000
ghcr.io/thevahber/vane`) und dieser Go-Paket implementiert **nur den
Bridge-Layer** — stdlib-only (`net/http`, `encoding/json`, `os`,
`path/filepath`, `bufio`, `io`, `context`, `sync`, `time`).

## Files

| File | Lines | Verantwortung |
|------|-------|---------------|
| `vane.go` | ~430 | Config-Persistierung (`vane.json`), typed `Client`, `Search`/`Healthy`, `FormatAnswer`, `RegisterMCP` Merger |
| `mcpserver.go` | ~375 | JSON-RPC 2.0 stdio MCP-Server (`Serve` / `NewServer` / `NewServerWithIO`), 2 Tools (`vane_research`, `vane_health`) |
| `vane_test.go` | ~755 | Hermetische Tests (httptest-Mock + io.Pipe-Stdio), race-clean, >70% Coverage |

## Setup

```bash
# 1. Vane-Instanz separat starten (Docker):
docker run -d --name vane -p 3000:3000 ghcr.io/thevahber/vane:latest

# 2. Bridge registrieren (von einem anderen Subagent: `vane_cmd.go`):
sin-code vane setup                       # legt $SIN_CODE_HOME/vane.json an
sin-code vane setup --base-url http://... # override
sin-code vane setup --chat-model claude-3-5-sonnet

# 3. MCP-Server in mcp.json mergen (passiert automatisch in `vane setup`):
sin-code vane install                     # mcp.json + vane.json
```

Beim Setup wird `os.Executable()` als `command` in `mcp.json` geschrieben —
dadurch funktioniert die Bridge ohne `sin-code` im PATH (z.B. von
`go run ./cmd/sin-code vane serve` aus).

## Graceful Degradation Contract

Wenn die Vane-Instanz **nicht erreichbar** ist (Docker-Container down,
Port falsch, Timeout), gilt:

| Layer | Verhalten |
|-------|-----------|
| `Client.Healthy(ctx)` | Returnt `error: vane: unreachable: ...` |
| `Client.Search(ctx, ...)` | Returnt `error: vane: post: ...` |
| `vane_research` MCP-Tool | Returnt `isError: true` mit Body `"vane: post: ...\n\nFallback: use the websearch ecosystem skill instead."` |
| `vane_health` MCP-Tool | Returnt `isError: true` mit `vane_health: ...` |

Der Agent bekommt also **niemals** einen fatalen JSON-RPC-Fehler
(`-32603 internal error`) — er bekommt eine freundliche
Fallback-Empfehlung und kann selbst entscheiden, ob er den
`websearch` Ecosystem-Skill nachladen soll. Das ist die
**NIE-CRASHE-Regel** für Tool-Layer: Tool-Fehler sind Daten, keine
Exceptions.

## Footguns

1. **Vane muss separat laufen.** Wer `sin-code vane` benutzt ohne
   Docker-Container bekommt 5 Sekunden Wartezeit + eine freundliche
   Fehlermeldung. Agent wird das dem User sagen, aber die Experience
   ist "tot" bis Vane läuft.

2. **`VANE_API_URL` schlägt `vane.json`.** Env-Variable hat
   **höchste** Priorität. CI/Docker-Setups können so den Endpunkt
   überschreiben ohne Disk-Edit.

3. **`SIN_CODE_HOME` und `Home()`-Logik identisch zu `superpowers`.**
   Beide Pakete teilen denselben Root, sodass ein einziger
   `SIN_CODE_HOME=/tmp/x` Env-Override beide gleichzeitig in eine
   Test-Sandbox schiebt.

4. **Vane-Response kann `message` ODER `answer` haben.** Je nach
   Vane-Version. Der Parser normalisiert auf `Answer.Message` und
   bevorzugt `message` (aktueller Vertrag).

5. **Keine Quellen = keine "## Cited Sources" Sektion.** Wenn Vane
   eine reine Text-Antwort liefert, wird `FormatAnswer` *exakt den
   Text* zurückgeben — kein leerer Header.

6. **MCP protocolVersion = "2025-06-18".** Falls ein älterer Client
   `initialize` mit strikter Version-Prüfung schickt, könnte er die
   Verbindung verweigern. Bei Unsicherheit: `sin-code mcp status` zeigt
   die ausgehandelte Version an.

## Verwandte Docs

- `superpowers.doc.md` — gleiche Home()/ConfigPath() Pattern
- `AGENTS.md` §3 M2 — single-binary mandate
- `AGENTS.md` §3 M7 — race-clean requirement
- `ECOSYSTEM.md` — vane als optionaler Skill, nicht Kern
