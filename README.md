# SIN-Code

> The verification-first, self-improving AI agent for code — learns from mistakes, never repeats a failure, and acts autonomously within hard safety invariants.

[![test-gate](https://img.shields.io/badge/test--gate-passing-brightgreen)](#)
[![ecosystem-sync](https://img.shields.io/badge/ecosystem--sync-passing-brightgreen)](#)
[![version](https://img.shields.io/badge/version-v3.8.0-blue)](https://github.com/OpenSIN-Code/SIN-Code/releases)

## Features

**Verification-First (M1/M3):** Every change passes a PoC/Oracle gate before completion — no unverified code ships.

**Closed Learning Loop (v3.4.0):** Failed verifications and tool errors accumulate in a persistent knowledge base. The agent queries this before each run and never repeats a recorded mistake.

**Bounded Autonomy (v3.5.0):** Goal queue + cron/file triggers + skill-lifecycle manager + autonomous daemon. Three hard safety invariants: no gate → no daemon; headless → ask=deny; budget exhausted → hook summons the human.

**Self-Extending (v3.4.0+):** `sin_bootstrap_skill` writes Python MCP servers from natural-language specs, tests them, and deploys them on the fly (defense-in-depth: requires `SIN_ALLOW_BOOTSTRAP=1`).

**Time-Travel Debugging:** Fork any session at any turn to explore parallel solution paths (`sin-code session fork <id> <turn>`).

**Multi-Agent Orchestration:** 31 subcommands, 44+ MCP tools, 12 ecosystem skill servers (websearch, browser automation, goal mode, rollback, …), permission gates (allow/ask/deny), deterministic lifecycle hooks (24 events).

**Swarm Mode (v3.6.0):** N agent profiles race on the same prompt with diverse strategies (different models, temperatures, tool sets); first verified solution wins. Three hard safety invariants: no gate → no daemon; headless → ask=deny; budget exhausted → hook summons the human.

**Self-Extending Agent (v3.6.0):** `sin_bootstrap_skill` writes Python MCP servers from natural-language specs and registers them in `.sin-code/mcp.json`. Defense-in-depth: requires `SIN_ALLOW_BOOTSTRAP=1` for headless use.

**Methodology Skills (v3.7.0):** `sin-code superpowers` integrates obra/superpowers (MIT) — the TDD/debugging/planning workflow library. Skills are pinned to a reviewed upstream SHA, overlaid with SIN-Code tool mappings (sin_bash, sin_preflight, orchestrate, etc.), and served as MCP tools. `sin-code superpowers update` shows the upstream skill diff before applying — review-before-trust, because skill content flows into your agent context.

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

# Swarm mode (v3.6.0)
sin-code swarm -p "optimize this" --agents fast,precise,creative
# First verified wins, others are cancelled

# Bootstrap a new skill (v3.6.0, headless requires SIN_ALLOW_BOOTSTRAP=1)
SIN_ALLOW_BOOTSTRAP=1 sin-code chat -p "use sin_bootstrap_skill to add a json-fmt tool"

# Methodology skills (v3.7.0)
sin-code superpowers install   # clone, pin, overlay, register MCP
sin-code superpowers init      # inject Superpowers prompt into AGENTS.md
sin-code superpowers update     # show upstream skill diff (review-first)
sin-code superpowers update --yes   # apply + re-pin
sin-code superpowers find "debug a failing test"   # auto-match a skill
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


## Methodology Stack (v3.8.0)

```
┌─────────────────────────────────────────────────────────────┐
│  LAYER 1 — Context      dox (agent0ai/dox)         MIT      │
│  LAYER 2 — Methodology  superpowers (obra)         MIT      │
│  LAYER 3 — Research     vane (ItzCrazyKns)         MIT      │
│  LAYER 4 — Tools        sin-code (this repo)       MIT      │
│                                                             │
│  COORDINATOR:  sin-code stack install|doctor               │
│                (idempotent, --json, per-layer degrade)      │
└─────────────────────────────────────────────────────────────┘
```

Install/doctor flow:

```bash
# One-shot: install every methodology-stack layer
sin-code stack install

# Per-layer health report
sin-code stack doctor --json
# {"superpowers":{"installed":true,"pinned":"abc1234"},
#  "dox":       {"installed":true,"version":"1.2.3"},
#  "vane":      {"reachable":true,"engine":"vane-0.4.0"}}

# Or interact with each layer directly
sin-code superpowers install           # methodology (obra)
sin-code dox check                     # protocol conformance (agent0ai)
sin-code vane install                  # citation-backed research (ItzCrazyKns)
sin-code vane search "tradeoffs of LRU vs 2-tier cooldown"
```

### Bridged External Tools (Never Vendored)

| Tool | Upstream | Bridge | License | Status |
|---|---|---|---|---|
| Vane | ItzCrazyKns/Vane | HTTP (internal/vane) | MIT | ACTIVE |
| Websearch | SIN-Code-Websearch-Skill | MCP `websearch__*` | MIT | ACTIVE |
| Symfony-Lens | sin-code-symfony-lens | MCP `symfonylens__*` | MIT | ACTIVE |

**Bridged-External** means: SIN-Code never vendors the upstream code; it
spawns a subprocess or speaks the upstream protocol directly. If the
upstream is unreachable, the layer degrades gracefully (e.g. vane →
websearch fallback) instead of crashing the agent.


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
├── cmd/sin-code/            ← MAIN BINARY (35 subcommands)
│   ├── main.go
│   ├── chat_cmd.go          ← chat + -p headless
│   ├── session_cmd.go       ← sessions list/show/rm/fork
│   ├── mcp_cmd.go           ← MCP debug (list|status|call)
│   ├── goal_cmd.go          ← autonomous goal queue
│   ├── daemon_cmd.go        ← autonomous worker
│   ├── skill_cmd.go         ← ecosystem skill management
│   ├── swarm_cmd.go         ← v3.6.0: N-profile race, first verified wins
│   ├── superpowers_cmd.go   ← v3.7.0: obra/superpowers integration
│   ├── vane_cmd.go          ← v3.8.0: Vane HTTP-bridge subcommand
│   ├── stack_cmd.go         ← v3.8.0: unified install/doctor across 3 layers
│   └── internal/            ← 20 packages
│       ├── agentloop/       ← PLAN→ACT→VERIFY→DONE loop
│       ├── session/, verify/, permission/  ← C2/C3/C4
│       ├── mcpclient/       ← C5: external MCP consumption
│       ├── hooks/           ← C7: 24 lifecycle events
│       ├── commands/        ← C8: custom slash commands
│       ├── lessons/         ← v3.4.0: closed learning loop
│       ├── autonomy/        ← v3.5.0: goal queue + triggers
│       ├── skillmgr/        ← v3.5.0: install/verify skills
│       ├── loopbuilder/     ← v3.4.0: shared factory (DRY)
│       ├── apiweb/          ← v3.6.0: WebUI-v2 HTTP API (sessions/knowledge/chat-SSE)
│       ├── meta/            ← v3.6.0: sin_bootstrap_skill (self-extending)
│       ├── dox/             ← v3.8.0: agent0ai/dox protocol checker
│       ├── vane/            ← v3.8.0: HTTP bridge to ItzCrazyKns/Vane (Bridged-External)
│       ├── stack/           ← v3.8.0: unified install/doctor coordinator
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
> **SIN-Code v3.8.0:** The agent that learns, evolves, never forgets — and follows world-class methodology.
