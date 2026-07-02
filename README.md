# SIN-Code

> The verification-first, self-improving AI agent for code — learns from mistakes, never repeats a failure, and acts autonomously within hard safety invariants.

## Why SIN-Code?

- **Mandatory verification gate** — Claude Code, Codex CLI, and Cursor all allow the model to declare "done" without proof. SIN-Code refuses.
- **Closed learning loop** — failed verifications persist across sessions. No competitor does this.
- **Bounded autonomy** — daemon runs goals end-to-end but cannot self-escalate permissions or skip the gate.
- **52+ MCP tools** — consumable by ANY agent via `sin-code serve`.
- **Full Pipeline** — 10-stage `/pipeline` command chains grill → plan → GSD → execute → review → done-gate → commit → record → CI/CD. 55+ systems, ~60% ecosystem coverage.
- **GSD Go-native** — `sin-code gsd` replaces 8 npm-based GSD skills with a single Go binary. Phases, waves, plans, execution state in `.gsd/`.
- **SIN-DoDone** — 11-pillar deterministic Definition-of-Done gate. Exit 0 = WIRKLICH FERTIG. No KI self-assessment possible.

[![test-gate](https://img.shields.io/badge/test--gate-passing-brightgreen)](#)
[![ecosystem-sync](https://img.shields.io/badge/ecosystem--sync-passing-brightgreen)](#)
[![version](https://img.shields.io/badge/version-v3.29.2-blue)](https://github.com/OpenSIN-Code/SIN-Code/releases)

## Status

- **Version**: [v3.29.2](https://github.com/OpenSIN-Code/SIN-Code/releases)
- **Maturity**: Production
- **Language**: Go (single static binary) + Python companion package
- **Tests**: 200+ tests across Go and Python; `go test ./... -race -count=1` is the gate
- **CI**: ✅ Passing (n8n-delegated, see `.github/workflows/`)

## Installation

```bash
# Go binary (recommended)
go install github.com/OpenSIN-Code/SIN-Code/cmd/sin-code@latest

# Or build from source
git clone https://github.com/OpenSIN-Code/SIN-Code.git
cd SIN-Code
go build -o ~/.local/bin/sin-code ./cmd/sin-code

# Python companion package (optional, for `sin` legacy CLI and `sin serve`)
pip install -e .
```

## Usage

```bash
sin-code --help
sin-code chat                              # interactive REPL
sin-code chat -p "refactor auth to use Argon2" --json   # headless one-shot
sin-code goal add "run the test suite and fix any failures" --priority 5
sin-code daemon --verify-cmd "go test ./... -count=1"
sin-code sessions list
sin-code chat --resume <session-id>
sin-code mcp list
sin-code skill status
sin-code stack doctor --json
sin-code gsd init --name "MyProject" --description "Build X"  # GSD project lifecycle
sin-code gsd phase add "Backend" --priority P0               # add phases
sin-code gsd execute 1                                       # show execution waves
```

See the **Quick Start** section below for more detailed examples, including
swarm mode, skill bootstrapping, and methodology skills.

## MCP Integration

- **MCP Server**: Go — `sin-code serve` (main binary, 54+ tools); Python legacy — `src/sin_code_bundle/mcp_server.py`
- **Tools**: 90+ subcommands, 41 bundled skills, 12 ecosystem skill servers, and external MCP servers (websearch, browser, symfony-lens, etc.)
- **Register**: Add `sin-code serve` to your MCP client config (see `docs/mcp.json.example`), or register the legacy Python server via `sin mcp register sin-serve src/sin_code_bundle/mcp_server.py`

## Development

- **CoDocs**: 210+ `.doc.md` companions — every meaningful code file has one
- **AGENTS.md**: ✅ Present — read it before any change; it is the single source of truth for all agents
- **Tests**: `go build ./... && go test ./... -race -count=1` for Go; `pytest tests/` for Python
- **Lint**: `golangci-lint run` (Go) and `ruff check .` (Python)
- **Compliance**: This repository follows the OpenSIN-Code CoDocs/AGENTS.md standard

## Features

**Verification-First (M1/M3):** Every change passes a PoC/Oracle gate before completion — no unverified code ships.

**Closed Learning Loop (v3.4.0):** Failed verifications and tool errors accumulate in a persistent knowledge base. The agent queries this before each run and never repeats a recorded mistake.

**Bounded Autonomy (v3.5.0):** Goal queue + cron/file triggers + skill-lifecycle manager + autonomous daemon. Three hard safety invariants: no gate → no daemon; headless → ask=deny; budget exhausted → hook summons the human.

**SinCode Loop System (always-on Definition-of-Done):** Every goal automatically carries the "self-evident" follow-through — write tests, debug, remove scaffolding, finish the job, and keep `README`/`CHANGELOG`/`AGENTS.md`/`MASTER_TODO`/`.doc.md` CoDocs in sync — so you never have to tell an agent to do that work again. The baseline is merged into every contract, stated to the worker up front, and enforced by the independent stop-gate. ON by default in `daemon` and `auto run`; escape with `--no-baseline` or `SIN_BASELINE=off`.

**Self-Extending (v3.4.0+):** `sin_bootstrap_skill` writes Python MCP servers from natural-language specs, tests them, and deploys them on the fly (defense-in-depth: requires `SIN_ALLOW_BOOTSTRAP=1`).

**Time-Travel Debugging:** Fork any session at any turn to explore parallel solution paths (`sin-code session fork <id> <turn>`).

**Multi-Agent Orchestration:** 90+ subcommands, 52+ MCP tools, 12 ecosystem skill servers (websearch, browser automation, goal mode, rollback, …), permission gates (allow/ask/deny), deterministic lifecycle hooks (24 events).

**Swarm Mode (v3.6.0):** N agent profiles race on the same prompt with diverse strategies (different models, temperatures, tool sets); first verified solution wins. Three hard safety invariants: no gate → no daemon; headless → ask=deny; budget exhausted → hook summons the human.

**Self-Extending Agent (v3.6.0):** `sin_bootstrap_skill` writes Python MCP servers from natural-language specs and registers them in `.sin-code/mcp.json`. Defense-in-depth: requires `SIN_ALLOW_BOOTSTRAP=1` for headless use.

**Methodology Skills (v3.7.0):** `sin-code superpowers` integrates obra/superpowers (MIT) — the TDD/debugging/planning workflow library. Skills are pinned to a reviewed upstream SHA, overlaid with SIN-Code tool mappings (sin_bash, sin_preflight, orchestrate, etc.), and served as MCP tools. `sin-code superpowers update` shows the upstream skill diff before applying — review-before-trust, because skill content flows into your agent context.

**Go-Native SCA (v3.15.0):** `sin security` now uses a native Go SCA client for Go projects, parsing `go.mod` and invoking `grype` JSON output directly.

## Bundled Skills (v3.29.2)

SIN-Code ships **43+ bundled skills** embedded in the binary, installable via `sin-code skills list|install`. Skills are organized into category directories and follow a unified naming convention `skill-<category>-<name>`.

```bash
sin-code skills list          # list all bundled skills
sin-code skills install skill-code-audit  # install a skill to ~/.config/opencode/skills
```

**Categories:**

| Category | Example skills |
|---|---|
| `browser-skills` | `skill-browser-tools` |
| `code-skills` | `skill-code-audit`, `skill-code-build`, `skill-code-codocs`, `skill-code-create`, `skill-code-docs`, `skill-code-mcp-builder`, `skill-code-refactor`, `skill-code-spec` |
| `debug-skills` | `skill-debug-deep` |
| `design-skills` | `skill-design-frontend`, `skill-design-image` |
| `ecosystem-skills` | `skill-ecosystem-context`, `skill-ecosystem-marketplace` |
| `github-skills` | `skill-github-account`, `skill-github-actions`, `skill-github-app`, `skill-github-governance`, `skill-github-readme` |
| `infrastructure-skills` | `skill-infrastructure-cloudflare`, `skill-infrastructure-oci-vm`, `skill-infrastructure-supabase` |
| `memory-skills` | `skill-memory-honcho`, `skill-memory-honcho-rollback`, `skill-memory-infisical` |
| `multimodal-skills` | `skill-multimodal-web-tools`, `skill-multimodal-youtube`, `skill-multimodal-analyse` |
| `process-skills` | `skill-process-goal`, `skill-process-grill`, `skill-process-gsd`, `skill-process-pipeline`, `skill-process-dodone`, `skill-process-delegate`, `skill-process-self-review`, `skill-process-scheduler`, `skill-code-lazy` |
| `shop-skills` | `skill-shop-cj-dropshipping`, `skill-shop-stripe`, `skill-shop-tiktok` |

**Unified Planning:** `plan v2` (in Infra-SIN-OpenCode-Stack) consolidates 5 former planning skills into 1 with modes: `--lite` (quick), `--cli` (deterministic), `--from-spec` (spec→tasks). `skill-code-plan` and `skill-planning-enterprise` have been absorbed.

**GSD Go-native:** `sin-code gsd` replaces 8 npm-based GSD skills (gsd-discuss-phase, gsd-execute-phase, gsd-help, gsd-new-project, gsd-phase, gsd-plan-phase, gsd-surface, gsd-update) with a single Go binary subcommand.

**Full Pipeline:** `/pipeline <task>` triggers `skill-process-pipeline` — 10 stages, 55+ systems, ~60% ecosystem coverage. Chains: grill → plan → GSD → execute → review → done-gate → commit → record → CI/CD.

Each skill contains `SKILL.md`, `context/`, `frameworks/`, `tasks/`, `templates/`, and a `LICENSE` file. External skills copied from `Infra-SIN-OpenCode-Stack` are tagged with `lifecycle: external` and `sources` in their metadata.

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
sin-code skill doctor  # diagnose why skills are not runnable
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


## Methodology Stack (v3.25.0)

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


## GitHub Bridge (v3.9.0)

The agent can now talk to GitHub itself — issue-first contributing, fully automated.
SIN-Code never vendors the `gh` CLI; it bridges the official binary via a 3-tier policy
enforced in code (`internal/ghbridge/`).

```
   Agent loop
       │  gh_query  (allow)  ← read-only: issue/pr/release/workflow-run/repo
       │  gh_health (allow)  ← PATH + auth probe
       │  gh_execute (ask)   ← mutating: issue create/comment/close, pr open/merge, workflow run
       ▼
  internal/ghbridge/  ──→  exec.Command("gh", ...)  ──→  real GitHub
   ├─ Classify()        3-tier policy:  allow | ask | forbidden
   ├─ runner.go         fail-closed subprocess wrapper
   └─ surface.go        capability discovery (doctor, surface)

  FORBIDDEN (hard-blocked, never reach runner):
    gh api, gh auth, gh secret, gh config, gh alias, gh extension,
    gh codespace, gh fork, gh sync, gh archive/unarchive/transfer,
    gh ssh-key, gh gpg-key
```

Install + doctor:

```bash
# One-time setup (idempotent)
sin-code gh setup

# Verify the bridge (PATH + auth)
sin-code gh doctor
# {"gh_path":"/opt/homebrew/bin/gh","gh_version":"2.62.0","auth_ok":true}

# Discover the active surface (verbs grouped by tier)
sin-code gh surface
# {"allow":["issue view","issue list","pr view",...],"ask":["issue create",...],"forbidden":["api",...]}
```

Workflow example — issue-first contributing, end-to-end:

```bash
# 1. Agent checks if a similar issue exists (gh_query, allow)
sin-code gh query issue list --search "gh bridge policy" --json

# 2. Agent drafts + opens a new issue (gh_execute, ask → human confirms)
sin-code gh execute issue create   --title "gh bridge: add rate-limit awareness"   --body "When a run hits the 5000/h REST cap, surface the reset time."   --label "enhancement,gh-bridge"

# 3. Note the issue number, then implement + reference it in the commit
git commit -m "feat(gh-bridge): honor X-RateLimit-Reset (#128)"
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
├── cmd/sin-code/            ← MAIN BINARY (90+ subcommands — v3.25.0)
│   ├── main.go
│   ├── chat_cmd.go          ← chat + -p headless
│   ├── session_cmd.go       ← sessions list/show/rm/fork/tree
│   ├── mcp_cmd.go           ← MCP debug (list|status|call|discover|add)
│   ├── goal_cmd.go          ← autonomous goal queue
│   ├── daemon_cmd.go        ← autonomous worker
│   ├── skill_cmd.go         ← ecosystem skill management
│   ├── swarm_cmd.go         ← v3.6.0: N-profile race, first verified wins
│   ├── superpowers_cmd.go   ← v3.7.0: obra/superpowers integration
│   ├── vane_cmd.go          ← v3.8.0: Vane HTTP-bridge subcommand
│   ├── stack_cmd.go         ← v3.8.0: unified install/doctor across 3 layers
│   ├── gh_cmd.go            ← v3.9.0: GitHub bridge (3-tier policy)
│   ├── hub_cmd.go           ← v3.12.0: tool catalog hub
│   ├── ledger_cmd.go        ← v3.13.0: semantic session ledger
│   ├── summary_cmd.go       ← v3.13.0: session auto-summary
│   ├── compress_cmd.go      ← v3.18.0: deterministic + LLM compaction
│   ├── audit_cmd.go         ← v3.18.0: repo-wide complexity audit
│   ├── ceo_audit_cmd.go     ← v3.18.0: 48-gate CEO audit
│   ├── review_cmd.go        ← v3.19.0: ponytail complexity review
│   ├── fusion_cmd.go        ← v3.22.0: SIN Fusion benchmark/rank/recommend
│   ├── research_cmd.go      ← v3.23.0: autonomous research report
│   ├── image_graph_cmd.go   ← v3.20.0: SOTA ECharts chart generation
│   ├── analyse_cmd.go       ← v3.23.0: static analysis runner
│   ├── analyse_image_cmd.go ← v3.24.0: vision-based image analysis
│   ├── doctor_cmd.go        ← v3.25.0: unified health check
│   ├── diff_cmd.go          ← v3.25.0: git diff with complexity overlay
│   ├── benchmark_cmd.go     ← v3.25.0: eval golden dataset runner
│   ├── tokens_cmd.go        ← v3.25.0: cost projection + budget alerts
│   ├── security_scan_commands.go ← v3.24.0: merged security scan subcommands
│   ├── permission_defaults.go ← default rules + MCP prefix policy
│   └── internal/            ← 95+ packages
│       ├── agentloop/       ← PLAN→ACT→VERIFY→DONE loop
│       ├── session/, verify/, permission/  ← C2/C3/C4
│       ├── mcpclient/       ← C5: external MCP consumption
│       ├── hooks/           ← C7: 24 lifecycle events
│       ├── hooklife/        ← v3.19.0: programmatic hook system + autoactivate
│       ├── commands/        ← C8: custom slash commands
│       ├── lessons/         ← v3.4.0: closed learning loop
│       ├── autonomy/        ← v3.5.0: goal queue + triggers
│       ├── skillmgr/        ← v3.5.0: install/verify skills
│       ├── skilldist/       ← v3.17.0: marker-fenced skill distribution
│       ├── loopbuilder/     ← v3.4.0: shared factory (DRY)
│       ├── stopgate/        ← decoupled completion authority
│       ├── goalcontract/    ← Definition-of-Done contracts
│       ├── compress/        ← v3.18.0: deterministic + LLM compaction
│       ├── mcpcompress/     ← v3.19.0: ponytail-tag tool-description compressor
│       ├── sindept/         ← v3.18.0: sin-debt marker scanner
│       ├── complexity/      ← v3.19.0: static ponytail complexity analyzer
│       ├── audit/           ← v3.18.0: CEO audit engine
│       ├── eval/, dataset/, trace/ ← v3.18.0: eval + observability
│       ├── evalharness/     ← v3.18.0: 4-arm comparator
│       ├── modelperf/       ← v3.22.0: per-model performance registry
│       ├── fusion/          ← v3.22.0: SIN Fusion tournament
│       ├── ghbridge/        ← v3.9.0: GitHub bridge (3-tier policy)
│       ├── ledger/, summary/ ← v3.13.0: semantic session ledger
│       ├── telemetry/       ← v3.24.0: live progress / NDJSON events
│       ├── vision/          ← v3.24.0: vision model for analyse-image
│       ├── security/        ← v3.24.0: Security Scanning V2
│       ├── sandbox/         ← OS-level sandbox (landlock/seatbelt/bubblewrap)
│       ├── catalog/         ← v3.20.0: unified tool catalog
│       ├── config/          ← unified config subsystem
│       ├── testgate/        ← v3.21.0: test-first verify-loop tools
│       ├── testgen/         ← v3.21.0: test generation
│       ├── swebench/        ← v3.23.0: SWE-bench test suite
│       ├── dox/             ← v3.8.0: agent0ai/dox protocol checker
│       ├── vane/            ← v3.8.0: HTTP bridge to ItzCrazyKns/Vane (Bridged-External)
│       ├── stack/           ← v3.8.0: unified install/doctor coordinator
│       ├── isolation/       ← v3.20.0: git-worktree primitives
│       ├── auto_mem/        ← v3.20.0: byte-stable MEMORY.md
│       ├── rules/           ← v3.20.0: path-scoped rule loader
│       ├── agentteams/      ← v3.20.0: file-locked agent-team mailbox
│       ├── imagegraph/      ← v3.20.0: SOTA ECharts chart generation
│       ├── profile/         ← v3.18.0: single-source-of-truth profile renderer
│       ├── install/         ← v3.18.0: pure-stdlib release install
│       └── llm/, orchestrator/, memory/, lsp/, todo/, ...
├── ECOSYSTEM.md             ← complete org inventory
├── AGENTS.md                ← master blueprint
├── profiles/                ← agent profile TOML files
├── docs/                    ← HOOKS.md, LEARNING.md, WEBUI.md, agent-profiles/
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

1. Every new feature lands as a GitHub issue FIRST — since v3.9.0 the agent can do this itself via `gh_execute issue create` (tier: ask → human confirms). Reference the issue number in the commit message (`feat: ... (#N)`).
2. Conventional commits (`feat:`, `fix:`, `docs:`, `test:`)
3. All tests pass: `go build ./... && go test ./... -race -count=1`
4. Core packages ≥70% coverage (target ≥85%)
5. Update ECOSYSTEM.md if adding/removing a repo (CI `ecosystem-sync.yml`
   enforces registry↔permission_defaults↔ECOSYSTEM agreement)

## License

MIT — see [LICENSE](LICENSE).

---

> **Einstein:** "Insanity is doing the same thing and expecting different results."
> **SIN-Code v3.25.0:** The agent that learns, evolves, never forgets — and follows world-class methodology.
