# SIN-Code

> The verification-first, self-improving AI agent for code — learns from mistakes, never repeats a failure, and acts autonomously within hard safety invariants.

[![test-gate](https://img.shields.io/badge/test--gate-passing-brightgreen)](#)
[![ecosystem-sync](https://img.shields.io/badge/ecosystem--sync-passing-brightgreen)](#)
[![version](https://img.shields.io/badge/version-v3.5.0-blue)](https://github.com/OpenSIN-Code/SIN-Code/releases)

## Features

**Verification-First (M1/M3):** Every change passes a PoC/Oracle gate before completion — no unverified code ships.

**Closed Learning Loop (v3.4.0):** Failed verifications and tool errors accumulate in a persistent knowledge base. The agent queries this before each run and never repeats a recorded mistake.

**Bounded Autonomy (v3.5.0):** Goal queue + cron/file triggers + skill-lifecycle manager + autonomous daemon. Three hard safety invariants: no gate → no daemon; headless → ask=deny; budget exhausted → hook summons the human.

**Self-Extending (v3.4.0+):** `sin_bootstrap_skill` writes Python MCP servers from natural-language specs, tests them, and deploys them on the fly (defense-in-depth: requires `SIN_ALLOW_BOOTSTRAP=1`).

**Time-Travel Debugging:** Fork any session at any turn to explore parallel solution paths (`sin-code session fork <id> <turn>`).

**Multi-Agent Orchestration:** 31 subcommands, 44+ MCP tools, 12 ecosystem skill servers (websearch, browser automation, goal mode, rollback, …), permission gates (allow/ask/deny), deterministic lifecycle hooks (24 events).

## Quick Start

```bash
# Install
go install github.com/OpenSIN-Code/SIN-Code/cmd/sin-code@latest

# Interactive REPL
sin-code chat

# Headless one-shot (stable JSON contract)
sin-code chat -p "refactor auth to use Argon2" --json
# {"session_id":"...","summary":"...","verified":true,"turns":3}

# Autonomous worker (requires verification command — M3)
sin-code goal add "run the test suite and fix any failures" --priority 5
sin-code daemon --verify-cmd "go test ./... -count=1"

# Manage sessions
sin-code sessions list
sin-code chat --resume <session-id>
sin-code session fork <session-id> 8

# Inspect the ecosystem
sin-code mcp list      # configured MCP servers
sin-code mcp status    # reachability + tool counts
sin-code skill status  # installed skill repos
sin-code knowledge list # accumulated lessons

# Swarm mode (planned for v3.6.0)
sin-code swarm -p "optimize this" --agents fast,precise,creative
```

## Architecture

```
┌─────────────────────────┐
│ sin-code chat/swarm/    │  CLI/TUI/WebUI-v2 (Next.js)
│ daemon/serve            │
└────────────┬────────────┘
             │
             ▼
┌──────────────────────────────────────┐
│ agentloop (PLAN→ACT→VERIFY→DONE)    │
│ • Provider (OpenAI-compatible)       │
│ • Permission (allow/ask/deny, M4)    │
│ • Hooks (24 events)                  │
│ • Verify Gate (PoC/Oracle, M3)       │
│ • Lessons (closed learning loop)     │
│ • Sessions (resumable SQLite)        │
└──────┬──────┬──────┬─────────┬───────┘
       │      │      │         │
       ▼      ▼      ▼         ▼
   ┌────┐  ┌────┐  ┌────┐  ┌──────┐
   │MCP │  │Loc │  │Ses │  │Goals │  (autonomy)
   │Srv │  │Tool│  │s   │  │Queue │
   └────┘  └────┘  └────┘  └──────┘
     12      6+   SQLite    SQLite
   skills builtin  (CGo-free) (CGo-free)
```

## Hard Mandates (M1–M7)

- **M1:** n8n-CI only — never run build/test on normal GitHub runners
- **M2:** Single static Go binary, `CGO_ENABLED=0`, `modernc.org/sqlite`
- **M3:** Verification gate (PoC/Oracle) is sacred; default `verify_mode = poc`
- **M4:** Permission engine gates all destructive ops; headless ask=deny
- **M5:** Module path `github.com/OpenSIN-Code/SIN-Code` (since v3.0.0)
- **M6:** SIN tools over naive built-ins (sin_edit, SCKG, EFM, …)
- **M7:** `go test -race` race-free; the 2026-06-12 v3.0.0 migration surfaced
  three real races in `Dispatcher.runOne` and we treat any unguarded
  shared-field mutation as a merge blocker.

## Repository Layout

```
SIN-Code/
├── cmd/sin-code/            ← MAIN BINARY (31 subcommands)
│   ├── main.go
│   ├── chat_cmd.go          ← chat + -p headless
│   ├── session_cmd.go       ← sessions list/show/rm/fork
│   ├── mcp_cmd.go           ← MCP debug (list|status|call)
│   ├── goal_cmd.go          ← autonomous goal queue
│   ├── daemon_cmd.go        ← autonomous worker
│   ├── skill_cmd.go         ← ecosystem skill management
│   └── internal/            ← 15 packages
│       ├── agentloop/       ← PLAN→ACT→VERIFY→DONE loop
│       ├── session/, verify/, permission/  ← C2/C3/C4
│       ├── mcpclient/       ← C5: external MCP consumption
│       ├── hooks/           ← C7: 24 lifecycle events
│       ├── commands/        ← C8: custom slash commands
│       ├── lessons/         ← v3.4.0: closed learning loop
│       ├── autonomy/        ← v3.5.0: goal queue + triggers
│       ├── skillmgr/        ← v3.5.0: install/verify skills
│       ├── loopbuilder/     ← v3.4.0: shared factory (DRY)
│       └── llm/, orchestrator/, memory/, lsp/, todo/, ...
├── ECOSYSTEM.md             ← complete org inventory
├── AGENTS.md                ← master blueprint
├── profiles/                ← agent profile TOML files
├── docs/                    ← HOOKS.md, LEARNING.md, WEBUI.md
└── .github/workflows/       ← ci + ecosystem-sync + release
```

## Documentation

- [AGENTS.md](AGENTS.md) — master blueprint (mandates, roadmap, layout)
- [ECOSYSTEM.md](ECOSYSTEM.md) — all 24 ACTIVE repos + integration status
- [docs/HOOKS.md](docs/HOOKS.md) — 24 lifecycle events reference
- [docs/LEARNING.md](docs/LEARNING.md) — closed learning loop architecture
- [docs/WEBUI.md](docs/WEBUI.md) — WebUI-v2 backend contract
- [CHANGELOG.md](CHANGELOG.md) — version history

## Contributing

1. Every new feature lands as a GitHub issue FIRST (reference in commit)
2. Conventional commits (`feat:`, `fix:`, `docs:`, `test:`)
3. All tests pass: `go build ./... && go test ./... -race -count=1`
4. Core packages ≥70% coverage (target ≥85%)
5. Update ECOSYSTEM.md if adding/removing a repo (CI `ecosystem-sync.yml`
   enforces registry↔permission_defaults↔ECOSYSTEM agreement)

## License

MIT — see [LICENSE](LICENSE).

---

> **Einstein:** "Insanity is doing the same thing and expecting different results."
> **SIN-Code v3.5.0:** The agent that learns, evolves, and never forgets.
