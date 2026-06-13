# AGENTS.md — SIN-Code Master Blueprint

> Single source of truth for ALL agents (human or AI) working on this repository.
> Read this file completely before making any change. If reality and this file
> diverge, fix the divergence in the same PR (code or doc — whichever is wrong).
>
> **Last verified against main:** commit pending (v3.9.0, 2026-06-13) —
> GitHub bridge release. Tool inventory and repo layout in sections 6 and
> 10 are sourced from `go test ./...` and `cmd/sin-code/main.go` AddCommand list.

---

## 1. What this repository IS

**SIN-Code** (formerly `SIN-Code-Bundle`) is the flagship product of the
OpenSIN-Code organization: a **verification-first, self-improving coding
agent** shipped as a single Go binary (`sin-code`), with a Python companion
package (`sin` / `sin serve`).

It is simultaneously:

1. **A coding-agent CLI** (`sin-code chat`, `sin-code -p "..."`) — interactive
   REPL/TUI and headless one-shot mode, like Claude Code / Codex CLI, but
   with a mandatory correctness gate before any task is reported done.
2. **A unified MCP server** (`sin-code serve` / `sin-serve`) — 44+ semantic
   tools consumable by ANY agent (Claude Code, Codex, opencode, our own
   loop, WebUI-v2).
3. **A multi-agent orchestrator** — DAG dispatcher with critic, adversary,
   governor, episodic memory, confidence scoring, blame/impact analysis,
   cartographer.
4. **A bounded-autonomous system** (v3.5.0) — goal queue, cron/file triggers,
   skill-lifecycle manager; the daemon runs goals end-to-end with hard safety
   invariants.

**Unique selling point (never compromise this):** every completed task MUST
pass the verification gate (PoC proof or Oracle check) before the agent
reports success. No other coding agent in the world enforces this.

## 2. What this repository is NOT

- NOT a fork of opencode. `OpenSIN-Code/OpenSIN-Code` was an opencode fork
  and is ARCHIVED (Phase 1, 2026-06-12). Never copy code from it, never
  reference it as a dependency.
- NOT related to Code-Swarm. That is a separate product, out of scope.
- NOT a place to vendor tool implementations that live in their own repos
  (see ecosystem map, section 5).
- NOT the WebUI. WebUI-v2 lives in its own repo
  (`/Users/jeremy/dev/sin-code-web-ui-v2`) and is wired via
  `sin-code serve` over stdio + `@ai-sdk/mcp`. The WebUI PR cycle is owned
  by a separate agent — never edit WebUI-v2 from this repo's agent loop.

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
and the gate has not passed. Default `verify_mode` is `"poc"`. The
**daemon refuses to start without `--verify-cmd`** (autonomy requires a
gate).

### M4 — Permission engine gates everything destructive
Every tool call goes through the permission engine
(`allow` / `ask` / `deny`). In headless mode, `ask` resolves to `deny`
unless `--yolo` is passed. **The daemon is always headless** — it cannot
self-escalate permissions.

**v3.9.0 bridge additions** (default policies, see
`cmd/sin-code/internal/permission_defaults.go`):

| Tool pattern | Policy | Layer | Reason |
|---|---|---|---|
| `vane__*` | allow | research (Bridged-External) | read-only citations, sandboxed |
| `superpowers__*` | allow | methodology (obra) | already local, just registered |
| `dox__*` | allow | context (agent0ai) | protocol check, read-only |
| `gh_query` | allow | contributing (Bridged-External) | read-only: issue/pr/release/workflow-run/repo |
| `gh_health` | allow | contributing (Bridged-External) | PATH + auth probe, no side effects |
| `gh_execute` | ask | contributing (Bridged-External) | mutating ops require human confirmation (M4) |

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

## 4. Architecture (v3.5.0)

```
SIN-Code-WebUI-v2 (separate repo, Next.js 16)
  AI SDK 6 + @ai-sdk/mcp (stdio)
  │  spawns
  ▼
SIN-CODE-CLI (this repo, cmd/sin-code)
  ├─ sin-code chat          ← interactive REPL + headless one-shot
  ├─ sin-code sessions      ← list/show/rm/fork resumable sessions
  ├─ sin-code mcp           ← list|status|call (debug ecosystem skills)
  ├─ sin-code goal          ← enqueue autonomous goals (v3.5.0)
  ├─ sin-code daemon        ← autonomous worker: lease → verify → learn
  ├─ sin-code skill         ← install/status ecosystem skills (v3.5.0)
  ├─ sin-code serve         ← unified MCP server (44+ tools)
  ├─ sin-code tui           ← standalone TUI binary
  ├─ sin-code webui         ← WebUI serve mode
  ├─ sin-code gh            ← v3.9.0: GitHub bridge (3-tier policy)
  └─ 36 subcommands

         │
         ▼
  ┌──────────────────────────────────────────┐
  │      AGENT LOOP (agentloop)              │
  │  PLAN → ACT → VERIFY → DONE              │
  │  • Permission engine (allow/ask/deny)    │
  │  • Hook engine (24 lifecycle events)     │
  │  • Verify Gate (PoC/Oracle, M3)          │
  │  • Sessions: SQLite, resumable           │
  │  • Lessons: closed learning loop (v3.4)  │
  │  • MCP-Client: external servers           │
  └──────────────────────────────────────────┘
         │            │            │
         ▼            ▼            ▼
  internal/llm   Orchestrator    MCP-Client
  (provider)     (DAG)           (12 skills + Symfony)
                  │
                  ├─ goal queue (autonomy, v3.5.0)
                  └─ skill manager (v3.5.0)
```

Agent loop state machine + learning loop:

```
PLAN ─► ACT ─► model claims done?
                          │
                          ▼
                VERIFY (PoC/Oracle)  ← M3
                          │ pass            │ fail
                          ▼                ▼
                        DONE        VERIFICATION FAILED
                          │          (report fed back
                          ▼           as user turn — retry)
                ┌──────────────────┐
                │ learning loop on │  ← v3.4.0: lessons pkg
                │ verify.fail /    │     records failures,
                │ tool.error       │     briefing on next run
                └──────────────────┘
```

---

## 5. Ecosystem map (org-level, fixed)

See `ECOSYSTEM.md` for the complete inventory with integration status and
sync rules. Single-line summary:

| Layer | Repos | Integration |
|---|---|---|
| **Agent + Tool plane** | **SIN-Code (this repo)** | The product |
| Web frontend | SIN-Code-WebUI-v2 (`/Users/jeremy/dev/sin-code-web-ui-v2`) | Spawns `sin-code serve`; follows releases |
| Multi-agent fan-out | SIN-Code-Orchestration | External MCP server |
| Verification & analysis (issue #28) | SCKG-Tool, IBD-Tool, PoC-Tool, ADW-Tool, EFM-Tool, Oracle-Tool, Symfony-Lens | CLI subprocess + MCP |
| Code intelligence | Simone-MCP | External MCP server (AST/LSP) |
| Browser automation | SIN-Browser-Tools | External MCP server (106 tools) |
| Skills (12) | websearch, scheduler, goalmode, grillme, marketplace, codocs, contextbridge, honcho, frontend, mcpbuilder, browser, simone | External MCP servers (registry.go) |
| LLM backends | coder-SIN-Qwen, SIN-Code-FireworksAI-OpenCode-Config | Agent profiles (profiles/*.toml) |
| Distribution | homebrew-sin | goreleaser pushes formula `sin-code` |
| Infra | Infra-SIN-OpenCode-Stack | Deployment |
| **Archived — never use** | OpenSIN-Code, SIN-Code-Bundle-Web, 6 long-name duplicates, coder-SIN-Qwen | Do not reference |
| **Out of scope** | Code-Swarm | Separate product |

---

## 6. Repository layout (verified `c06cf18`)

```
SIN-Code/
├── AGENTS.md                  ← this file (single source of truth)
├── README.md
├── CHANGELOG.md
├── ECOSYSTEM.md               ← complete org inventory + sync rules
├── go.mod                     ← module github.com/OpenSIN-Code/SIN-Code
├── .goreleaser.yaml
├── .github/workflows/
│   ├── ceo-audit.yml          ← n8n delegation (mandate M1)
│   ├── sin-code-release.yml   ← goreleaser + brew tap
│   └── ecosystem-sync.yml     ← prevents registry/permission/ECOSYSTEM drift
├── install.sh
├── profiles/                   ← v3.4.0: agent profile TOML files
│   ├── fireworks.toml
│   └── qwen-relay.toml
├── docs/
│   ├── HOOKS.md
│   ├── LEARNING.md
│   ├── WEBUI.md                ← WebUI-v2 backend contract
│   └── mcp.json.example
│
├── cmd/
│   ├── sin-code/              ← MAIN BINARY (36 subcommands — v3.9.0)
│   │   ├── main.go            ← cobra root; AddCommand for all subcommands
│   │   ├── tui.go, webui_cmd.go
│   │   ├── chat_cmd.go        ← v3.4.0: chat + -p headless
│   │   ├── chat_tools.go      ← builtin toolset
│   │   ├── chat_tools_extra.go ← v3.5.0: sin_git_*, sin_test, sin_http_get
│   │   ├── chat_mcp.go        ← combinedTool/combinedSpecs
│   │   ├── session_cmd.go     ← sessions list/show/rm/fork
│   │   ├── mcp_cmd.go         ← mcp list|status|call (debug)
│   │   ├── goal_cmd.go        ← v3.5.0: goal add|list
│   │   ├── daemon_cmd.go      ← v3.5.0: autonomous worker
│   │   ├── skill_cmd.go       ← v3.5.0: skill install|status
│   │   ├── superpowers_cmd.go   ← v3.7.0: obra/superpowers integration
│   │   ├── vane_cmd.go          ← v3.8.0: Vane HTTP-bridge subcommand (NewVaneCmd)
│   │   ├── stack_cmd.go         ← v3.8.0: unified install/doctor coordinator (NewStackCmd)
│   │   ├── permission_defaults.go ← C4: default rules + MCP prefix policy
│   │   └── internal/          ← 17 packages (v3.8.0)
│   │       ├── agentloop/     ← PLAN→ACT→VERIFY→DONE loop
│   │       ├── session/       ← SQLite-backed resumable sessions
│   │       ├── permission/    ← allow/ask/deny engine
│   │       ├── verify/        ← mandatory PoC/Oracle gate
│   │       ├── mcpclient/     ← external MCP consumption
│   │       ├── hooks/         ← 24 lifecycle events
│   │       ├── commands/      ← custom slash commands
│   │       ├── lessons/       ← v3.4.0: closed learning loop
│   │       ├── autonomy/      ← v3.5.0: goal queue + triggers
│   │       ├── skillmgr/      ← v3.5.0: install/verify skills
│   │       ├── loopbuilder/   ← v3.4.0: shared factory (DRY)
│   │       ├── vane/          ← v3.8.0: HTTP bridge to ItzCrazyKns/Vane (internal/vane)
│   │       ├── stack/         ← v3.8.0: unified install/doctor across 3 layers
│   │       ├── llm/           ← provider layer
│   │       ├── orchestrator/  ← DAG, critic, adversary, governor, ...
│   │       ├── memory/        ← (existing) store/search/embed
│   │       ├── lsp/, notifications/, todo/, plugins/, sandbox/, attachments/, webui/
│   ├── sin-tui/               ← standalone TUI binary
│   └── SIN-Code-Container-Tool-Go, SIN-Code-SAST-Tool,
│       SIN-Code-SBOM-Generator, SIN-Code-SBOM-Generator-Go,
│       SIN-Code-SCA-Tool-Go, SIN-Code-Secrets-Scanner
│
├── src/sin_code_bundle/       ← Python companion: `sin` CLI + `sin-serve`
├── tests/                     ← Go + Python tests
└── scripts/                   ← org-cleanup.sh, promote-to-sin-code.sh
```

---

## 7. Configuration contract

User-level `~/.config/sin-code/config.json`, overridden by project-level
`./.sin/config.json` (deep-merge, project wins). See section 7 in the
**previous** version of AGENTS.md for the full JSON schema, or read
`docs/mcp.json.example` for the MCP-specific shape.

Session DB: `~/.local/share/sin-code/sessions.db` (SQLite, modernc).
Lessons DB: `~/.local/share/sin-code/lessons.db` (SQLite, modernc).
Goal Queue DB: `~/.local/share/sin-code/goals.db` (SQLite, modernc).

Headless JSON contract (stable API — never break without major bump):

```json
{ "session_id": "…", "summary": "…", "verified": true, "turns": 12 }
```

---

## 8. Roadmap (versions 3.0.0 – 3.9.0)

| Version | Status | Contents |
|---|---|---|
| v3.0.0 | ✅ SHIPPED | Bundle→SIN-Code rename, module-path migration, race-fix |
| v3.1.0 | ✅ SHIPPED | C1-C5: agentloop, session, verify, permission, mcpclient |
| v3.2.0 | ✅ SHIPPED | C7 hooks (24 events) + C8 slash commands |
| v3.4.0 | ✅ SHIPPED | Einstein Layer: closed learning loop, loopbuilder, MCP wiring, ECOSYSTEM.md |
| v3.5.0 | ✅ SHIPPED | Bounded Autonomy: goal queue, triggers, skillmgr, daemon |
| v3.6.0 | ✅ SHIPPED | Swarm mode, bootstrap-skill (self-extending), TUI v3.3.1 embed, WebUI-v2 HTTP API |
| v3.7.0 | ✅ SHIPPED | `sin-code superpowers` — obra/superpowers integration with supply-chain pinning + review-before-trust updates |
| v3.8.0 | ✅ SHIPPED | Vane HTTP-bridge (`vane__*` research), Stack consolidation (`stack install/doctor` across superpowers+dox+vane), 33 → 35 subcommands, Bridged-External + stdio MCP architecture, 47/47 ecosystem-sync gates green |
| v3.9.0 | ✅ SHIPPED | GitHub bridge (`internal/ghbridge/`, 3-tier policy: allow/ask/forbidden), `gh` subcommand (setup/doctor/run/surface/serve), 3 MCP tools (`gh_query` allow, `gh_health` allow, `gh_execute` ask), 35 → 36 subcommands, issue-first contributing workflow now agent-executable |

Each release tag ⇒ goreleaser builds linux/darwin/windows × amd64/arm64,
updates `homebrew-sin` formula, and ships to GitHub Releases.

---

## 9. Development workflow

- Go 1.23+. Before EVERY commit: `go build ./... && go test ./... -race -count=1`.
- Conventional commits (`feat:`, `fix:`, `docs:`, `feat!:` for breaking).
- Releases: tag push `vX.Y.Z` ⇒ goreleaser builds multi-arch + updates brew.
- Never reduce test count or coverage. New loop code targets ≥80% coverage.
- Python side (`src/sin_code_bundle`): ruff + pytest, same PR discipline.
- Docs: every behavioral change updates docs/ + CHANGELOG.md in the same PR.
- AGENTS.md + ECOSYSTEM.md are kept in sync with the codebase (CI
  ecosystem-sync.yml enforces registry↔permission↔ECOSYSTEM agreement).

---

## 10. Naming and stability rules

- Binary: `sin-code`. Brew formula: `sin-code`. MCP server name: `sin`.
- The 44+ MCP tool names are a public API — renaming any is a breaking
  change (major bump + deprecation alias for one minor cycle).
- Tool prefixes for external MCP servers use `server__tool` namespacing
  (e.g. `websearch__search`, `browser__navigate`).
- The string "SIN-Code-Bundle" may only appear in CHANGELOG history and
  migration notes — never in code, config, or new docs (mandate M5).

### CLI subcommands (verified `cmd/sin-code/main.go`, v3.5.0)

```
Core:      discover, execute, map, grasp, scout, harvest, orchestrate,
           ibd, poc, sckg, adw, oracle, efm
Agents:    chat, sessions, mcp, goal, daemon, skill, superpowers,
           vane, stack, gh
Frontend:  serve, tui, webui
Lifecycle: memory, knowledge, todo, notifications, orchestrator_run,
           orchestrator_agents, orchestrator_plan
Utility:   read, write, edit, lsp, plugin, index, security, sbom,
           config, self-update
``` (v3.9.0: 36 subcommands, up from 35 in v3.8.0)

### Hook events (verified `internal/hooks/hooks.go`, v3.5.0)

24 events: `session.{start,resume,end}`, `turn.{start,end}`,
`tool.{pre,post,denied,error}`, `permission.ask`, `verify.{pre,pass,fail}`,
`agent.{spawn,complete}`, `critic.reject`, `adversary.finding`,
`governor.block`, `memory.{write,compact}`, `commit.{pre,post}`,
`push.pre`, `task.{complete,abort}`, `compaction.pre`,
`goal.{enqueued,started,verified,exhausted}` (v3.5.0),
`trigger.fired` (v3.5.0), `skill.{installed,failed}` (v3.5.0).

---

## 11. For external AI agents (Claude Code, Codex, etc.) working here

1. Read sections 3 (mandates) and 8 (roadmap) before any edit.
2. Use the repo's own tools on itself when available (`sin-code serve`
   dogfooding).
3. One C-gap or one issue per PR. No drive-by refactors.
4. If you cannot satisfy a mandate, STOP and report — do not work around it.
5. WebUI-v2 is OUT OF SCOPE for this repo's agent loop. Edits to
   `/Users/jeremy/dev/sin-code-web-ui-v2` belong to that repo's local agent.
