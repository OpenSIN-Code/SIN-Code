# `internal/ghbridge` — Allowlisted GitHub CLI (gh) Bridge

## Was

`internal/ghbridge` ist die **sichere Brücke** vom SIN-Code-Agent-Layer
zum offiziellen **GitHub CLI (`gh`)**. Der Agent soll Issues, PRs,
Releases, Workflow-Runs und Repo-Metadaten abfragen und — nach expliziter
ask-policy-Bestätigung — auch mutieren können. **Ohne** dabei jemals
`gh auth`, `gh secret set`, `gh api ...` oder `gh repo delete` aus Versehen
oder durch Prompt-Injection aufrufen zu können.

Das Paket implementiert einen **3-Tier Verb Allowlist**:

| Tier | Beispiele | Tool | Policy |
|---|---|---|---|
| `read-only` | `gh issue list`, `gh pr view`, `gh search issues` | `gh_query` | `allow` |
| `mutating` | `gh issue create`, `gh pr merge`, `gh issue close` | `gh_execute` | `ask` |
| `forbidden` | `gh auth ...`, `gh secret ...`, `gh api ...`, `gh config ...`, `gh repo delete`, `gh extension ...` | (keines) | `deny` |

Die Klassifizierung ist **fail-closed**: jede unbekannte Eingabe
fällt in `TierForbidden` und der `Runner` wird **nie** aufgerufen.

## Warum NIE vendored

**Mandate M2 (single static Go binary, CGo-free, stdlib-only)**
verbietet das Einchecken der gh-CLI (~50 MB Go-Binary mit Cgo-Deps) in
`internal/`. `gh` läuft als **separater Prozess**, den der User via
`brew install gh` / `apt install gh` / offizielles MSI installiert.
Dieser Go-Paket implementiert nur den **Bridge-Layer** — stdlib-only
(`os/exec`, `bufio`, `encoding/json`, `context`, `time`, `sort`,
`strings`, `path/filepath`, `io`, `sync`).

## 3-Tier-Architektur-Diagramm

```
   ┌──────────────────────────────────────────────────────────┐
   │          Agent Loop (sin-code chat / mcp)                 │
   │  Modell entscheidet: "issue list" oder "issue create"     │
   └──────────────────────────────────────────────────────────┘
                              │ tools/call
                              ▼
   ┌──────────────────────────────────────────────────────────┐
   │  internal/ghbridge (MCP-Stdio-Server)                     │
   │  ┌────────────────┐   ┌────────────────┐                 │
   │  │ gh_query       │   │ gh_execute     │                 │
   │  │ (read-only)    │   │ (mutating)     │                 │
   │  └────────────────┘   └────────────────┘                 │
   │           │                    │                          │
   │           └────────┬───────────┘                          │
   │                    ▼                                      │
   │         Classify(args)  ← FAIL-CLOSED                     │
   │         ┌─────────┼─────────┐                             │
   │         ▼         ▼         ▼                             │
   │      ReadOnly  Mutating  Forbidden                       │
   │         │         │         │                             │
   │         │         │         └─→  isError=true             │
   │         │         │              (Runner NEVER called)    │
   │         │         ▼                                      │
   │         │      ask-policy? ──no──→ isError=true          │
   │         │         │                                      │
   │         ▼         ▼                                      │
   │              Runner (Runner interface)                    │
   │              └─→ ExecRunner → os/exec("gh", args...)     │
   └──────────────────────────────────────────────────────────┘
                              │
                              ▼
                   gh (extern, Homebrew/apt)
                              │
                              ▼
                  https://api.github.com
```

## Files

| File | Lines | Verantwortung |
|------|-------|---------------|
| `ghbridge.go` | ~350 | `Tier` enum, `Runner` interface, `ExecRunner`, `Bridge.Health`/`Execute`, `Classify` (the security core), allowlist maps, `truncate` |
| `mcpserver.go` | ~480 | JSON-RPC 2.0 stdio MCP-Server (`Serve` / `NewServer` / `NewServerWithIO`), 3 Tools (`gh_query`, `gh_execute`, `gh_health`), `dispatch` mit tool/tier Cross-Check, `RegisterMCP` Merger |
| `ghbridge_test.go` | ~660 | Hermetische Tests (fake-Runner, bytes.Buffer-Stdio, t.TempDir), race-clean, >70% Coverage |
| `ghbridge.doc.md` | (this) | Architektur, Setup, Footguns |

## Setup

```bash
# 1. gh-CLI separat installieren (NICHT in internal/!)
brew install gh        # macOS
sudo apt install gh    # Debian/Ubuntu
winget install gh      # Windows

# 2. Authentifizieren (manuell, einmalig)
gh auth login

# 3. Bridge registrieren (von einem anderen Subagent: gh_cmd.go):
sin-code gh setup                       # merged "gh" server in mcp.json
sin-code gh doctor                      # prüft: gh installiert? auth OK?
```

Beim Setup wird `os.Executable()` als `command` in `mcp.json`
geschrieben — die Bridge funktioniert dadurch ohne `sin-code` im
PATH (z.B. von `go run ./cmd/sin-code gh serve` aus).

## Defense in Depth

Die Sicherheit der Bridge ruht auf **drei unabhängigen Schichten**:

1. **`forbiddenTokens`-Scan über ALLE args-Positionen** (nicht nur
   `args[1]`). Verhindert z.B. `["issue", "list", "delete"]` oder
   `["repo", "view", "--json", "api"]`.
2. **`allowedGroups`-Whitelist** für `args[0]`. Verhindert
   `["gist", "list"]`, `["codespace", "create"]` etc.
3. **`forbiddenTokens`-Recheck** auf `args[1]`. Defense in depth
   (Schritt 1 hätte es bereits gefangen).

**Resultat:** `TestExecuteForbiddenNeverRuns` beweist, dass der
`Runner` für **jede** der 7 verbotenen Eingaben 0× aufgerufen wird
(atomic-Increment-Zähler). Das ist die kritische Sicherheits-Invariante.

## Footguns

1. **`gh api` ist HARD-BLOCKED.** `gh api` ist die "nuclear option"
   — es umgeht jeden gh-Wrapper, weil es direkt REST-Calls absetzt.
   Wer `gh api repos/foo/bar/issues` braucht, soll die
   `gh issue list` / `gh pr view` Tools benutzen. Wer einen
   `gh api`-Use-Case hat, der nicht abgedeckt ist, soll ein Issue
   öffnen, damit der Verb explizit erlaubt werden kann.

2. **`gh config` ist HARD-BLOCKED.** Konfigurationsänderungen gehen
   über das `$SIN_CODE_HOME/gh.json` Config-File (eigener Subagent:
   `gh_cmd.go`), nicht über `gh config set`. Verhindert, dass ein
   LLM die gh-Installation umkonfiguriert (`gh config set editor vim`
   etc.).

3. **`gh auth` ist HARD-BLOCKED.** Authentifizierung ist ein
   manueller One-Time-Step (`gh auth login`). Die Bridge
   verifiziert den Status via `gh auth status` in `Health()`, aber
   niemals via `gh auth login` / `gh auth logout` / `gh auth
   refresh`.

4. **ask-policy vs. non-interactive chat.** `gh_execute` ist im
   Headless-Modus (`Permission: ask`) immer `deny` ohne `--yolo`.
   Im interaktiven TUI fragt die Permission-Engine den User. In
   **beiden** Fällen MUSS der User explizit bestätigen, bevor
   `gh_execute` läuft. Die `Tier`-Klassifizierung garantiert, dass
   `gh_query` auch ohne Bestätigung läuft (read-only).

5. **`gh run watch` und `gh workflow run --watch` können lange
   laufen.** Beide klassifizieren als `mutating` (sie können
   Seiteneffekte triggern). Der 60s-Default-Timeout in `Execute`
   wird sie nach 60 Sekunden killen — das ist by design (Agent-Slot
   soll nicht 10 Minuten blockiert sein). Wer wirklich watchen
   will, soll `gh run watch` direkt im Terminal aufrufen, nicht
   via Bridge.

6. **`gh` muss separat installiert sein.** `Health()` schlägt
   fehl, wenn `gh` nicht im PATH ist. Die Fehlermeldung nennt
   explizit "gh binary not found on PATH" — Agent kann dem User
   `brew install gh` empfehlen.

7. **Forbidden-Token im TAIL der args.** `["issue", "list",
   "delete"]` blockt. Wer das testen will: `TestClassifyForbiddenTokenInTail`.

## Graceful Degradation Contract

Wenn der `Runner` fehlschlägt (gh-CLI nicht installiert, User nicht
authentifiziert, Network-Time-out), gilt:

| Layer | Verhalten |
|-------|-----------|
| `Bridge.Health(ctx)` | Returnt `error: ghbridge: ...` |
| `Bridge.Execute(ctx, args)` | Returnt `("", tier, error)` mit stderr im error-Wrap |
| `gh_query` / `gh_execute` MCP-Tool | Returnt `isError: true` mit Body `"gh_query: ghbridge: gh issue list: fatal: ..."` |
| `gh_health` MCP-Tool | Returnt `isError: true` mit `gh_health: ghbridge: ...` |

Der Agent bekommt also **niemals** einen fatalen JSON-RPC-Fehler
(`-32603 internal error`) — er bekommt eine freundliche
Fehlermeldung und kann selbst entscheiden, ob er den User nach
`gh auth login` fragen oder den Fallback zu manueller
Issue/PR-Bearbeitung wählen soll. Das ist die **NIE-CRASHE-Regel**
für Tool-Layer: Tool-Fehler sind Daten, keine Exceptions.

## Verwandte Docs

- `vane.doc.md` — gleiche Home()/ConfigPath()/writeJSONAtomic-Pattern
- `superpowers.doc.md` — gleiche MCP-Server-Registry-Semantik
- `AGENTS.md` §3 M2 — single-binary mandate
- `AGENTS.md` §3 M7 — race-clean requirement
- `ECOSYSTEM.md` — gh-bridge als optionaler Skill, nicht Kern
