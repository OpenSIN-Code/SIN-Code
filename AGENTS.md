# AGENTS.md — SIN-Code Master Blueprint

> Single source of truth for ALL agents (human or AI) working on this repository.
> Read this file completely before making any change. If reality and this file
> diverge, fix the divergence in the same PR (code or doc — whichever is wrong).
>
> **Last verified against main:** commit `7962b73` (v3.0.0, 2026-06-12) — after
> Bundle → SIN-Code rename + module-path migration. Tool inventory and repo
> layout in sections 6 and 10 are sourced from `go test ./...` and
> `cmd/sin-code/main.go` AddCommand list.

---

## 1. What this repository IS

**SIN-Code** (formerly `SIN-Code-Bundle`) is the flagship product of the
OpenSIN-Code organization: a **verification-first coding agent** shipped as a
single Go binary (`sin-code`), with a Python companion package (`sin` /
`sin serve`).

It is simultaneously:

1. **A coding-agent CLI** (`sin-code chat`, `sin-code -p "..."`) — interactive
   TUI/REPL and headless one-shot mode, like Claude Code / Codex CLI, but with
   a mandatory correctness gate (PoC proof or Oracle check) before any task is
   reported done.
2. **A unified MCP server** (`sin-code serve` / `sin-serve`) — 44+ semantic
   tools consumable by ANY agent (Claude Code, Codex, opencode, our own loop,
   WebUI-v2).
3. **A multi-agent orchestrator** — DAG dispatcher with critic, adversary,
   governor, episodic memory, confidence scoring, blame/impact analysis,
   cartographer.

**Unique selling point (never compromise this):** every completed task MUST
pass the verification gate (PoC proof or Oracle check) before the agent
reports success. No other coding agent in the world enforces this.

## 2. What this repository is NOT

- NOT a fork of opencode. `OpenSIN-Code/OpenSIN-Code` was an opencode fork and
  is ARCHIVED (Phase 1, 2026-06-12). Never copy code from it, never reference
  it as a dependency.
- NOT related to Code-Swarm. That is a separate product, out of scope.
- NOT a place to vendor tool implementations that live in their own repos
  (see ecosystem map, section 5).
- NOT the WebUI. WebUI-v2 lives in its own repo
  (`/Users/jeremy/dev/sin-code-web-ui-v2`) and is wired via
  `sin-code serve` over stdio + `@ai-sdk/mcp`. The WebUI PR cycle is owned by
  a separate agent — never edit WebUI-v2 from this repo's agent loop.

---

## 3. Hard mandates (violations block merge)

### M1 — CI/CD: n8n delegator ONLY
NEVER run build/test/lint on normal GitHub Actions runners. This org uses
n8n + `OpenSIN-AI/sin-github-action` exclusively (webhook secret
`N8N_CI_WEBHOOK_URL`); all real work executes on the OCI free-tier VM.
The only permitted `runs-on: ubuntu-latest` job is the ~2s curl delegation
step itself. Docs: docs.opensin.ai/best-practices/ci-cd-n8n

### M2 — Single binary
`sin-code` ships as ONE static Go binary. `CGO_ENABLED=0`. SQLite via
`modernc.org/sqlite` only. No runtime dependencies beyond the binary itself.

### M3 — Verification gate is sacred
The agent loop must never report task success while `verify_mode != "off"`
and the gate has not passed. Default `verify_mode` is `"poc"`.

### M4 — Permission engine gates everything destructive
Every tool call goes through the permission engine
(`allow` / `ask` / `deny`). In headless mode, `ask` resolves to `deny`
unless `--yolo` is passed.

### M5 — Module path
`github.com/OpenSIN-Code/SIN-Code` (since v3.0.0). The old path
`.../SIN-Code-Bundle` must not appear in any new code, config, or doc.

### M6 — SIN tools over naive built-ins
The tool router always prefers semantic SIN tools over naive equivalents:
`sin_edit` over string-replace, SCKG navigation over blind file reads,
EFM environments over ad-hoc mocks.

### M7 — Race-free concurrency
The orchestrator and any goroutine-using subsystem MUST be verified under
`go test -race` before merge. The 2026-06-12 v3.0.0 migration surfaced three
real races in `internal/orchestrator/dispatcher.go`; treat any unguarded
shared-field mutation as a merge blocker.

---

## 4. Architecture (target, v3.x)

```
SIN-Code-WebUI-v2 (separate repo, Next.js 16)
  AI SDK 6 + @ai-sdk/mcp (stdio)
  │  spawns
  ▼
SIN-CODE-CLI (this repo, cmd/sin-code)
  ├── sin-code chat    ← interactive REPL/TUI
  ├── sin-code -p ".."  ← headless one-shot (stable JSON contract)
  ├── sin-code serve   ← unified MCP server (44+ tools)
  ├── sin-code sessions list|show|rm
  ├── sin-code tui     ← standalone TUI binary
  ├── sin-code webui   ← WebUI serve mode
  └── 30+ subcommands: discover, execute, map, grasp, scout, harvest,
        orchestrate, ibd, poc, sckg, adw, oracle, efm, security, sbom,
        config, self-update, plugin, read, write, edit, lsp, index, todo,
        memory, notifications, serve, …

         │
         ▼
  ┌──────────────────────────────────────┐
  │      AGENT LOOP (agentloop)          │
  │  PLAN → ACT → VERIFY → DONE          │
  │                                      │
  │  Permission engine: allow/ask/deny   │
  │  Hook engine: tool.pre/post/verify.* │
  │  Verify gate: PoC / Oracle (M3)      │
  │  Sessions: SQLite, resumable         │
  │  MCP-Client: external servers        │
  └──────────────────────────────────────┘
         │           │           │
         ▼           ▼           ▼
  internal/llm   Orchestrator   MCP-Client
  (OpenAI-       (dag,          (simone, browser,
   compatible,    dispatcher,    orchestration,
   NIM, gateway)  critic,        all *-Skill)
                  adversary,
                  governor,
                  episodic)
```

Agent loop state machine:

```
PLAN ─► ACT (tool calls via router+permissions) ─► model claims done?
                                                       │
                                                       ▼
                                              VERIFY (PoC/Oracle)
                                                       │ pass
                                                       ▼
                                              DONE
                                              (persist session,
                                               summary,
                                               verified=true,
                                               turns=N)
                                              ▲ failure
                                              └─ VERIFICATION FAILED
                                                 (report fed back
                                                  as user turn)
```

---

## 5. Ecosystem map (org-level, fixed)

| Layer | Repo | Relationship to this repo |
|---|---|---|
| **Agent + Tool plane** | **SIN-Code (this repo)** | The product |
| Web frontend | `SIN-Code-WebUI-v2` (path: `/Users/jeremy/dev/sin-code-web-ui-v2`) | Spawns `sin-code serve` over stdio; follows our releases |
| Multi-agent fan-out | `SIN-Code-Orchestration` | Consumed as external MCP server |
| Semantic tools | `SCKG-Tool`, `IBD-Tool`, `PoC-Tool`, `ADW-Tool`, `EFM-Tool`, `Oracle-Tool` | Backends invoked by our tools |
| Code intelligence | `Simone-MCP` | External MCP server (AST/LSP) |
| Browser automation | `SIN-Browser-Tools` | External MCP server (106 tools) |
| Skills | all `*-Skill` repos (12 total) | External MCP servers |
| Distribution | `homebrew-sin` | goreleaser pushes formula `sin-code` |
| Infra | `Infra-SIN-OpenCode-Stack` | Deployment of serve mode |
| **Archived — never use** | `OpenSIN-Code` (opencode fork), `SIN-Code-Bundle-Web`, 6 long-name duplicates, `coder-SIN-Qwen` | Do not reference |
| **Out of scope** | `Code-Swarm` | Separate product |

---

## 6. Repository layout (verified `7962b73`)

```
SIN-Code/
├── AGENTS.md                  ← this file (single source of truth)
├── README.md
├── CHANGELOG.md
├── Makefile
├── go.mod                     ← module github.com/OpenSIN-Code/SIN-Code
├── .goreleaser.yaml
├── .github/workflows/
│   ├── ceo-audit.yml          ← n8n delegation (mandate M1)
│   └── sin-code-release.yml   ← goreleaser + brew tap
├── install.sh
├── Formula/sin-code.rb        ← homebrew formula (goreleaser-managed)
│
├── cmd/
│   ├── sin-code/              ← MAIN BINARY
│   │   ├── main.go            ← cobra root; registers all 30+ subcommands
│   │   ├── tui.go             ← TUI entry
│   │   ├── webui_cmd.go       ← WebUI serve mode
│   │   ├── internal/          ← 10 packages
│   │   │   ├── agentloop/     ← PLAN→ACT→VERIFY→DONE loop         [C1, target]
│   │   │   ├── session/       ← resumable SQLite sessions          [C2, target]
│   │   │   ├── permission/    ← allow/ask/deny engine              [C4, target]
│   │   │   ├── verify/        ← mandatory PoC/Oracle gate          [C3, target]
│   │   │   ├── mcpclient/     ← external MCP server consumption    [C5, target]
│   │   │   ├── hooks/         ← lifecycle automation engine        [C7, target]
│   │   │   ├── commands/      ← custom slash commands              [C8, target]
│   │   │   ├── llm/           ← provider layer (OpenAI-compatible, NIM, gateway)
│   │   │   ├── orchestrator/  ← dag, dispatcher, critic, adversary, governor,
│   │   │   │                    episodic, confidence, contract, blame, impact,
│   │   │   │                    cartographer
│   │   │   ├── memory/        ← store, search, embed, model
│   │   │   ├── lsp/           ← LSP client
│   │   │   ├── notifications/
│   │   │   ├── todo/          ← SQLite-backed todo store
│   │   │   ├── plugins/       ← plugin loader
│   │   │   ├── sandbox/       ← platform-sandbox
│   │   │   ├── attachments/
│   │   │   └── webui/         ← embedded webui server
│   │   └── (target) chat_cmd.go, session_cmd.go
│   ├── sin-tui/               ← standalone TUI binary
│   └── SIN-Code-Container-Tool-Go, SIN-Code-SAST-Tool,
│       SIN-Code-SBOM-Generator, SIN-Code-SBOM-Generator-Go,
│       SIN-Code-SCA-Tool-Go, SIN-Code-Secrets-Scanner
│
├── src/sin_code_bundle/       ← Python companion: `sin` CLI + `sin-serve`
├── tests/                     ← Go + Python tests
├── docs/
│   ├── ARCHITECTURE.md
│   ├── AGENT-LOOP.md
│   ├── MCP-TOOLS.md
│   └── HOOKS.md               ← PR C7 deliverable
└── scripts/
    ├── org-cleanup.sh         ← phase 1 (idempotent)
    └── promote-to-sin-code.sh ← phase 2 (rename + migration)
```

Note: target modules (`agentloop/`, `session/`, `permission/`, `verify/`,
`mcpclient/`, `hooks/`, `commands/`, `chat_cmd.go`, `session_cmd.go`) are part
of the C1–C8 roadmap (section 8). If a path is missing, it is a TODO —
create it there, nowhere else.

---

## 7. Configuration contract

User-level `~/.config/sin-code/config.json`, overridden by project-level
`./.sin/config.json` (deep-merge, project wins):

```json
{
  "model": "anthropic/claude-opus-4.6",
  "provider": "gateway",
  "api_key_env": "AI_GATEWAY_API_KEY",
  "max_turns": 80,
  "verify_mode": "poc",
  "agent": "default",
  "mcp_servers": [
    { "name": "simone",  "transport": "stdio", "command": "simone-mcp" },
    { "name": "browser", "transport": "stdio", "command": "sin-browser-mcp" },
    { "name": "orchestration", "transport": "http", "url": "http://localhost:8732/mcp" }
  ],
  "permissions": [
    { "tool": "sin_read",   "policy": "allow" },
    { "tool": "sin_edit",   "policy": "allow" },
    { "tool": "sckg_*",     "policy": "allow" },
    { "tool": "sin_bash",   "policy": "ask"   },
    { "tool": "browser_*",  "policy": "ask"   },
    { "tool": "*",          "policy": "ask"   }
  ],
  "hooks": [
    {
      "event": "tool.post",
      "matcher": "sin_edit",
      "type": "command",
      "command": "gofmt -w $(jq -r '.data.args.path // empty') 2>/dev/null || true"
    },
    {
      "event": "push.pre",
      "type": "command",
      "command": "sin-code secrets scan --staged --quiet || { echo 'secret detected — push blocked'; exit 2; }"
    },
    {
      "event": "verify.fail",
      "type": "webhook",
      "url": "https://n8n.example.com/webhook/sin-verify-fail"
    },
    {
      "event": "task.complete",
      "type": "webhook",
      "url": "https://n8n.example.com/webhook/sin-task-done"
    },
    {
      "event": "adversary.finding",
      "type": "webhook",
      "url": "https://n8n.example.com/webhook/sin-security-finding"
    },
    {
      "event": "session.start",
      "type": "prompt",
      "text": "Org mandates: n8n CI only, conventional commits, never reduce test coverage."
    }
  ]
}
```

Session DB: `~/.local/share/sin-code/sessions.db` (SQLite, modernc).
Headless JSON contract (stable API — never break without major bump):

```json
{ "session_id": "…", "summary": "…", "verified": true, "turns": 12 }
```

---

## 8. Roadmap (C-gaps) — work strictly in this order

| ID | Gap | Definition of done |
|----|-----|--------------------|
| C1 | `sin-code chat` + `-p` headless entry | TUI/REPL session starts; `-p "..." --json` returns the JSON contract; reuses internal/llm + orchestrator, NO parallel new loop |
| C2 | Resumable sessions | `--resume <id>` continues with full history; `sin-code sessions list/show/rm` work; stored in sessions.db |
| C3 | Verification gate | Loop refuses "done" until PoC/Oracle passes; failure report fed back as user turn; `verify_mode` in AgentConfig, default `poc` |
| C4 | `ask` permission state | Interactive prompt in TUI; headless: ask⇒deny unless `--yolo`; covered by tests |
| C5 | MCP client manager | External servers from config are connected, their tools merged into the router with `server__tool` namespacing |
| C6 | WebUI-v2 alignment | session_id passthrough + VerifiedBadge in WebUI-v2; package.json name fixed (**separate repo PR** by WebUI's local agent) |
| C7 | Hook engine | `internal/hooks` per docs/HOOKS.md; wired into agentloop (tool.pre/post, verify.*, task.complete), permission (permission.ask), and git commands (commit/push.pre); blocking via exit 2; config key `hooks`; ≥80% test coverage |
| C8 | Custom slash commands | `.sin/commands/*.md` + `~/.config/sin-code/commands/*.md`; frontmatter (description/agent/verify_mode); `/help` in REPL; tests |

Release plan: C1–C5 ⇒ tag `v3.1.0`. C7–C8 ⇒ tag `v3.2.0`. C6 ⇒ separate
WebUI-v2 PR. Each C-gap = one PR, each PR includes tests.

---

## 9. Development workflow

- Go 1.23+. Before EVERY commit: `make lint test` (golangci-lint,
  `go test ./... -race -cover`).
- Conventional commits (`feat:`, `fix:`, `docs:`, `feat!:` for breaking).
- Releases: tag push `vX.Y.Z` ⇒ goreleaser builds linux/darwin/windows ×
  amd64/arm64 and updates `homebrew-sin` formula.
- Never reduce test count or coverage. New loop code targets ≥80% coverage.
- Python side (`src/sin_code_bundle`): ruff + pytest, same PR discipline.
- Docs: every behavioral change updates docs/ + CHANGELOG.md in the same PR.

---

## 10. Naming and stability rules

- Binary: `sin-code`. Brew formula: `sin-code`. MCP server name: `sin`.
- The 44+ MCP tool names below are a public API — renaming any is a
  breaking change (major bump + deprecation alias for one minor cycle).

### MCP tool inventory (verified `7962b73`)

```
sin_read, sin_write, sin_edit
sin_execute, sin_discover, sin_grasp, sin_map, sin_scout, sin_harvest
sin_orchestrate, sin_ibd, sin_poc, sin_sckg, sin_adw, sin_oracle, sin_efm
sin_lsp_servers
sin_index
sin_memory_add, sin_memory_list, sin_memory_search, sin_memory_stats,
sin_memory_prime
sin_todo_add, sin_todo_list, sin_todo_search, sin_todo_show, sin_todo_stats,
sin_todo_complete, sin_todo_claim, sin_todo_dep_add, sin_todo_deps,
sin_todo_ready, sin_todo_blocked, sin_todo_prime
sin_notifications_list, sin_notifications_mark_read, sin_notifications_stats
sin_orchestrator_run, sin_orchestrator_agents, sin_orchestrator_plan
sin_agent_doctor, sin_agent_set, sin_agent_show
```

### CLI subcommands (verified `cmd/sin-code/main.go`)

`discover, execute, map, grasp, scout, harvest, orchestrate, ibd, poc, sckg,
adw, oracle, efm, serve, security, sbom, config, self-update, tui, webui,
read, write, edit, lsp, plugin, index, memory, todo, notifications,
orchestrator_run, orchestrator_agents, orchestrator_plan`

### String "SIN-Code-Bundle" usage

The string "SIN-Code-Bundle" may only appear in CHANGELOG history and
migration notes — never in code, config, or new docs (mandate M5).

---

## 11. For external AI agents (Claude Code, Codex, etc.) working here

1. Read sections 3 (mandates) and 8 (roadmap) before any edit.
2. Use the repo's own tools on itself when available (`sin-code serve`
   dogfooding).
3. One C-gap or one issue per PR. No drive-by refactors.
4. If you cannot satisfy a mandate, STOP and report — do not work around it.
5. WebUI-v2 is OUT OF SCOPE for this repo's agent loop. Edits to
   `/Users/jeremy/dev/sin-code-web-ui-v2` belong to that repo's local agent.
