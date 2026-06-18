# AGENTS.md — SIN-Code Master Blueprint

> Single source of truth for ALL agents (human or AI) working on this repository.
> Read this file completely before making any change. If reality and this file
> diverge, fix the divergence in the same PR (code or doc — whichever is wrong).
>
> **Last verified against main:** v3.18.0 (2026-06-16) —
> bundled skills reorganized into category directories and renamed to
> `skill-<category>-<name>`; `github-skills/` category added; 35 bundled skills
> embedded in the binary. Branch protection on `main` permanently relaxed to
> **Last verified against main:** v3.19.0 (2026-06-16) —
> `sin-code serve --compress-tools` wired (issue #173): ponytail-tag
> compressor `cmd/sin-code/internal/mcpcompress/` shrinks the 47-tool
> manifest on the wire (delete|stdlib|native|yagni|shrink tags,
> byte-stable per `(tool_spec, ruleset)`). Bundled skills reorganized
> into category directories and renamed to `skill-<category>-<name>`;
> `github-skills/` category added; 35 bundled skills embedded in the
> binary. Branch protection on `main` permanently relaxed to
> `required_approving_review_count: 0` for solo-maintainer workflow.
> `sin-code install` (issue #170) — new 40th subcommand; root `install.sh`
> rewritten to a 27-line curl|bash shim; matching `install.ps1` for Windows.
> `sin-code compress` (issue #172): deterministic + LLM compaction for
> lessons / instincts / summaries / memory / AGENTS.md; snapshot+rollback
> for lossless Apply; 39 → 40 subcommands. Branch protection on `main`
> permanently relaxed to `required_approving_review_count: 0` for
> solo-maintainer workflow.
> `sin-code debt` (issue #177) — sin-debt marker convention
> (`// sin-debt: <ceiling>, upgrade: <trigger>`) adopted from ponytail;
> 41st subcommand, `cmd/sin-code/internal/sindept/` package with byte-stable
> scanner + aggregator + report + policy gate.
> **Last verified against main:** v3.19.0 (2026-06-16) —
> `autoactivate` hooklife subpackage (issue #176) wired into `sin-code chat`
> with `--activate <rule>` + `--no-trigger` flags; project-local
> `.sin-code/autoactivate.toml`; deterministic byte-stable rule rendering;
> race-safe per-session state (mandate M7). Branch protection on `main`
> permanently relaxed to `required_approving_review_count: 0` for solo-
> maintainer workflow.
> `sin-code audit` + `sin-code ceo-audit` added (issue #180): complexity-audit
> engine in `cmd/sin-code/internal/audit/`, 48th CEO-audit gate.
> `sin-code install` (issue #170) — 40th subcommand; `sin-code review --complexity`
> (issue #179) — 41st subcommand.
> **Last verified against main:** v3.20.0 (2026-06-17) —
> agent profiles now expose the full `sin_*` + registered MCP prefix surface
> while keeping destructive builtins at `ask` (issue #249); system-prompt M6
> enforcement fragment prefers SIN tools over naive bash/read equivalents
> (issue #253); runtime `ToolCoverageEnforcer` rejects completion when
> `required_tools` are missing or `forbidden_tools` were used (issue #248);
> tool-usage telemetry in `internal/ledger/` drives `ledger tools --heatmap`
> / `--coverage` / `--unused` / `--json` (issue #250); orchestrator planner
> emits mandatory `ToolChain` per classified intent (issue #252); unified
> catalog enumerates 46+ MCP tools, 17+ chat tools, and 14+ external MCP
> prefixes via `sin-code catalog` / `hub search --unused` (issue #251).

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

> **Last verified against main:** v3.23.0 (2026-06-18) —
> v3.23.0 roadmap batch 2 shipped: sin_edit/MCP surgical-editor unification
> (issue #373), reactive permission policy from tool results (issue #374),
> offline semantic tool retrieval (issue #364), and containerized execution
> for autonomous goals (issue #389) wired into `sin-code daemon` via
> `--container` / `--container-image`. WebUI daemon management (issue #388)
> closed as out-of-scope. Docker is an external binary dependency; the Go
> code remains CGO-free (M2).

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

**Stop-gate (loop engineering, decoupled completion authority):** when a
Definition-of-Done contract is present (`internal/goalcontract`), the worker's
"done" is necessary but NOT sufficient. After the verify-gate passes, an
independent stop-gate (`internal/stopgate`) confirms the contract — deterministic
checks first (fail-closed: ANY failed check blocks, including predicates), then a
strong/equal LLM judge (`SIN_EVALUATOR_MODEL`, fallback worker model) for
non-mechanical criteria. A green judge can never override a red deterministic
check. Rejection re-injects the open criteria and forces continued work. All of
this is opt-in and fail-safe: nil stop-gate / empty contract / no contract flag
preserves exact legacy single-gate behavior.

**Tool coverage gate (issue #248):** the runtime `ToolCoverageEnforcer` in
`internal/agentloop/` is part of the verification gate. When `required_tools`
are configured and the model completes a run without invoking every required
tool, or when a `forbidden_tools` entry was invoked, the loop rejects the
completion and re-injects the violation as open criteria. The enforcer is
fail-closed and race-safe (M7).

**Daemon loop-engineering flags:** `--max-continuations` (checkpoint+resume past
`--max-turns` instead of aborting, bounded), `--max-depth` (sub-goal nesting via
the `spawn_subgoal` tool), `--no-contract` (disable Definition-of-Done, revert to
single verify-gate). Discovery: `goal discover [--dry-run]` and the `discover`
trigger type turn TODO/FIXME markers and `MASTER_TODO.md` items into deduplicated
goals. Env vars: `SIN_EVALUATOR_MODEL`, `SIN_EVALUATOR_BASE_URL`,
`SIN_EVALUATOR_API_KEY`.

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
| `fusion__tournament` | ask | autonomy (Fusion) | spawns sub-agents, costs money |
| `fusion__status` | allow | autonomy (Fusion) | read-only tournament status |
| `fusion__config` | allow | autonomy (Fusion) | read-only config query |

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
  ├─ sin-code sessions      ← list/show/rm/fork/tree resumable sessions (parent_id lineage)
  ├─ sin-code mcp           ← list|status|call (debug ecosystem skills)
  ├─ sin-code goal          ← enqueue autonomous goals (v3.5.0)
  ├─ sin-code daemon        ← autonomous worker: lease → verify → learn
  ├─ sin-code skill         ← install/status ecosystem skills (v3.5.0)
  ├─ sin-code serve         ← unified MCP server (44+ tools)
  ├─ sin-code tui           ← standalone TUI binary
  ├─ sin-code webui         ← WebUI serve mode
  ├─ sin-code gh            ← v3.9.0: GitHub bridge (3-tier policy)
  ├─ sin-code hub           ← v3.12.0: tool catalog hub
  ├─ sin-code ledger        ← v3.13.0: semantic session ledger
  ├─ sin-code summary       ← v3.13.0: session auto-summary
  ├─ sin-code compress      ← v3.18.0: deterministic + LLM compaction (issue #172)
  └─ 40 subcommands
  ├─ sin-code audit         ← v3.18.0: repo-wide complexity audit (issue #180)
  ├─ sin-code ceo-audit     ← v3.18.0: 48-gate CEO audit with complexity gate (issue #180)
  └─ 42 subcommands
  ├─ sin-code review        ← v3.19.0: ponytail complexity review (issue #179)
  ├─ sin-code image-graph   ← v3.20.0: SOTA ECharts chart generation (bar/line/pie/area)
  └─ 42 subcommands (v3.20.0)

         │
         ▼
  ┌──────────────────────────────────────────┐
  │      AUDIT LAYER (v3.18.0)               │
  │  • complexity — static + LLM judge (M3)    │
  │  • ceo-audit — 48 gates incl. complexity   │
  └──────────────────────────────────────────┘
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
│   ├── ceo-audit.yml          ← n8n delegation (mandate M1) + profile-verify job (issue #175)
│   ├── sin-code-release.yml   ← goreleaser + brew tap
│   └── ecosystem-sync.yml     ← prevents registry/permission/ECOSYSTEM drift
├── install.sh                    ← 27-line curl|bash shim → `sin-code install` (issue #170)
├── install.ps1                   ← PowerShell equivalent shim (issue #170)
├── profiles/                   ← v3.4.0: agent profile TOML files
│   ├── fireworks.toml
│   └── qwen-relay.toml
├── docs/
│   ├── HOOKS.md
│   ├── LEARNING.md
│   ├── WEBUI.md                ← WebUI-v2 backend contract
│   ├── mcp.json.example
│   └── agent-profiles/
│       └── sin-profile.md      ← v3.18.0: single-source-of-truth per-agent profile (issue #175)
│
├── cmd/
│   ├── sin-code/              ← MAIN BINARY (40 subcommands — v3.18.0)
│   ├── complexity-review.md    ← v3.19.0: ponytail complexity review docs
│   └── mcp.json.example
│
├── cmd/
│   ├── sin-code/              ← MAIN BINARY (42 subcommands — v3.20.0)
│   │   ├── main.go            ← cobra root; AddCommand for all subcommands
│   │   ├── tui.go, webui_cmd.go
│   │   ├── chat_cmd.go        ← v3.4.0: chat + -p headless
│   │   ├── chat_tools.go      ← builtin toolset
│   │   ├── chat_tools_extra.go ← v3.5.0: sin_git_*, sin_test, sin_http_get
│   │   ├── chat_mcp.go        ← combinedTool/combinedSpecs
│   │   ├── session_cmd.go     ← sessions list/show/rm/fork/tree (issue #194)
│   │   ├── mcp_cmd.go         ← mcp list|status|call (debug)
│   │   ├── goal_cmd.go        ← v3.5.0: goal add|list
│   │   ├── daemon_cmd.go      ← v3.5.0: autonomous worker
│   │   ├── skill_cmd.go       ← v3.5.0: skill install|status
│   │   ├── superpowers_cmd.go   ← v3.7.0: obra/superpowers integration
│   │   ├── vane_cmd.go          ← v3.8.0: Vane HTTP-bridge subcommand (NewVaneCmd)
│   │   ├── stack_cmd.go         ← v3.8.0: unified install/doctor coordinator (NewStackCmd)
│   │   ├── hub_cmd.go           ← v3.12.0: tool catalog hub subcommand
│   │   ├── ledger_cmd.go        ← v3.13.0: ledger query subcommand
│   │   ├── summary_cmd.go       ← v3.13.0: summary builder subcommand
│   │   ├── install_cmd.go       ← v3.18.0: `sin-code install` (issue #170, single-binary installer)
│   │   ├── permission_defaults.go ← C4: default rules + MCP prefix policy
│   │   └── internal/          ← 18 packages (v3.18.0)
  │   │   ├── summary_cmd.go       ← v3.13.0: summary builder subcommand
  │   │   ├── compress_cmd.go       ← v3.18.0: sin-code compress subcommand (plan/apply/rollback, issue #172)
  │   │   ├── permission_defaults.go ← C4: default rules + MCP prefix policy
│   │   └── internal/          ← 17 packages (v3.8.0)
│   │   ├── install_cmd.go       ← v3.18.0: sin-code install (issue #170)
│   │   ├── profile_cmd.go       ← v3.18.0: single-source-of-truth profile renderer (issue #175)
│   │   ├── permission_defaults.go ← C4: default rules + MCP prefix policy
│   │   └── internal/          ← 18 packages (v3.18.0)
│   │   ├── review_cmd.go        ← v3.19.0: `review --complexity` ponytail 5-tag review (issue #179)
│   │   ├── permission_defaults.go ← C4: default rules + MCP prefix policy
│   │   └── internal/          ← 18 packages (v3.19.0)
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
│   │       ├── skilldist/     ← v3.17.0: marker-fenced skill distribution (issue #169)
│   │       ├── loopbuilder/   ← v3.4.0: shared factory (DRY)
│   │       ├── vane/          ← v3.8.0: HTTP bridge to ItzCrazyKns/Vane (internal/vane)
│   │       ├── stack/         ← v3.8.0: unified install/doctor across 3 layers
│   │       ├── hub/           ← v3.12.0: static tool catalog
│   │       ├── ledger/        ← v3.13.0: semantic session ledger (SQLite)
│   │       ├── summary/       ← v3.13.0: deterministic session summary builder
│   │       ├── install/       ← v3.18.0: pure-stdlib release install + SHA256 verify + atomic place (issue #170)
│   │       ├── mcpcompress/   ← v3.19.0: ponytail-tag compressor for `serve --compress-tools`
│   │       ├── install/       ← v3.18.0: pure-stdlib release install + SHA256 verify (issue #170)
│   │       ├── profile/       ← v3.18.0: single-source-of-truth per-agent renderer (issue #175)
│   │       ├── eval/          ← v3.18.0: issue #75 eval + observability
│   │       ├── dataset/       ← v3.18.0: golden-dataset JSON parser
│   │       ├── trace/         ← v3.18.0: OpenTelemetry TracerProvider
│   │       ├── evalharness/   ← v3.18.0: 4-arm comparator (issue #171)
│   │       ├── install/       ← v3.18.0: pure-stdlib install + SHA256 (issue #170)
│   │       ├── sindept/       ← v3.18.0: // sin-debt: marker scanner/reporter (issue #177)
│   │       ├── complexity/    ← v3.19.0: static ponytail complexity analyzer (issue #179)
│   │       ├── llm/           ← provider layer
│   │       ├── style/         ← v3.17.0: verbosity / compression mode system-prompt renderer (issue #167)
│   │       ├── orchestrator/  ← DAG, critic, adversary, governor, ...
│   │       ├── memory/        ← (existing) store/search/embed
│   │       ├── lsp/, notifications/, todo/, plugins/, sandbox/, attachments/, webui/
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
│   │       ├── skilldist/     ← v3.18.0: marker-fenced skill distribution (issue #169)
│   │       ├── isolation/     ← v3.20.0: git-worktree primitives (issue #194 part 2)
│   │       ├── auto_mem/      ← v3.20.0: byte-stable MEMORY.md (Claude-Auto-Memory parity, M3-safe)
│   │       ├── rules/         ← v3.20.0: path-scoped rule loader (Claude-Code v2.1 parity, issue #195)
│   │       ├── sdk/           ← v3.20.0: in-process MCP Go-SDK wrapper (issue #202)
│   │       ├── agentteams/    ← v3.20.0: file-locked agent-team mailbox (issue #203)
│   │       ├── loopbuilder/   ← v3.4.0: shared factory (DRY)
│   │       ├── vane/          ← v3.8.0: HTTP bridge to ItzCrazyKns/Vane (internal/vane)
│   │       ├── stack/         ← v3.8.0: unified install/doctor across 3 layers
  │   │       ├── hub/           ← v3.12.0: static tool catalog
  │   │       ├── ledger/        ← v3.13.0: semantic session ledger (SQLite)
  │   │       ├── summary/       ← v3.13.0: deterministic session summary builder
  │   │       ├── compress/      ← v3.18.0: deterministic + LLM compaction (issue #172)
│   │       ├── llm/           ← provider layer
│   │       ├── style/         ← v3.17.0: verbosity / compression mode system-prompt renderer (issue #167)
│   │       ├── orchestrator/  ← DAG, critic, adversary, governor, ...
│   │       ├── orchestrator/  ← DAG, critic, adversary, governor,
│   │       │                   cartographer; caveman-style output
│   │       │                   contracts (issue #174)
│   │       ├── memory/        ← (existing) store/search/embed
│   │       ├── lsp/, notifications/, todo/, plugins/, sandbox/, attachments/, webui/
│   ├── sin-tui/               ← standalone TUI binary
│   └── SIN-Code-Container-Tool-Go, SIN-Code-SAST-Tool,
│       SIN-Code-SBOM-Generator, SIN-Code-SBOM-Generator-Go,
│       SIN-Code-SCA-Tool-Go, SIN-Code-Secrets-Scanner
│
├── src/sin_code_bundle/       ← Python companion: `sin` CLI + `sin-serve`
├── skills/                    ← 37 bundled skills in category directories
│   ├── browser-skills/
│   ├── code-skills/
│   ├── debug-skills/
│   ├── design-skills/
│   ├── ecosystem-skills/
│   ├── github-skills/
│   ├── infrastructure-skills/
│   ├── memory-skills/
│   ├── planning-skills/
│   ├── process-skills/
│   └── shop-skills/
├── tests/                     ← Go + Python tests
└── scripts/                   ← org-cleanup.sh, promote-to-sin-code.sh, validate_skill.py
```

### 6.1 Orchestrator output contracts (issue #174)

The four orchestrator sub-agents — `Critic`, `Adversary`, `Governor`,
`Cartographer` — emit caveman-style one-liners via `Finding` structs in
`cmd/sin-code/internal/orchestrator/output_contract.go`. The contract is
the prose layer; the structured result types (e.g. `AdversaryResult.Attacks`)
are unchanged.

**One-liner shape** (byte-stable, em-dash U+2014 separator):

```
<path>:<line> — <symbol> — <tag> — <hint> # c=<confidence>
```

**5 closed tags** (parallel to JuliusBrussee/ponytail):

| Tag       | Meaning                                          |
| --------- | ------------------------------------------------ |
| `delete`  | remove this code / path / symbol                 |
| `simplify`| inline, collapse, reduce                         |
| `rebuild` | rewrite from scratch                             |
| `risk`    | blast radius, regression source                  |
| `verify`  | needs human verification                         |

**Hedging forbidden** in `Hint` (closed set, case-folded):
`you might`, `perhaps`, `could consider`, `maybe`, `i think`, `i would`,
`sort of`, `kind of`, `tends to`, `should probably`, `i'd suggest`,
`we should`. `VerifyFindings` rejects every finding whose hint contains
any of these — the sub-agent loses the entire batch, not silently
passes.

**Wired into**: `CriticResult.Findings` (parsed from the last attempt's
prose; `CriticResult.ParseErrors` carries the per-line rejection trace),
`AdversaryResult.Findings` (derived from each Attack), `GovernorResult.Findings`
(one per Escalation, anchored at `task://<ID>`), `Cartographer.Findings(k)`
(top-k by PageRank, opt-in — k ≤ 0 yields empty). Byte-stability is a hard
prerequisite for downstream consumers (issue #168 ledger hashing,
orchestrator re-ingestion cost).

This file is distinct from `contract.go`, which owns the **Intent Contract**
(task-scope: allowed globs, frozen globs, forbidden patterns, blast radius).
Output-contract is the **prose-shape** contract; the two have no shared
types and no shared parser.

---

## 7. Configuration contract

User-level `~/.config/sin/sin-code.toml`, overridden by project-level
`./.sin-code/config.toml` (deep-merge, project wins). The TOML-like file uses
flat namespaced keys (e.g. `llm.base_url`, `permissions.tools_allow`) because
the parser is dependency-free (M2). Manage it with `sin-code config`:
`init`, `show`, `validate`, `get`, `set`, `list`, `path`. Secrets are masked
in `show` unless `--plain` is passed. See `cmd/sin-code/internal/config.go`
and `config.doc.md` for the full schema and validation rules.

Agent profiles (`profiles/*.toml`) are copied/symlinked into
`~/.config/sin-code/agents/<name>/agent.toml`. The bundled `fireworks` and
`qwen-relay` profiles set `tools_allow` to the full SIN tool surface and all
registered MCP prefixes (issue #249). Destructive SIN builtins (`sin_bash`,
`sin_git_commit`, `sin_test_generate`, `sin_browser_navigate`) are intentionally
omitted from the profile allow list so they remain at the default "ask" tier and
are still gated by the permission engine in headless mode (mandate M4).

**Tool coverage config keys (issue #248):**

| Config key | Type | Default | CLI equivalent |
|---|---|---|---|
| `agentloop.required_tools` | comma-separated list | `[]` | `--require-tools` |
| `agentloop.forbidden_tools` | comma-separated list | `[]` | `--forbid-tools` |

These are parsed by the stdlib TOML reader as flat strings and converted to
slices by `cmd/sin-code/internal/config.go`. They are forwarded through
`loopbuilder.Config` into `agentloop.Loop.CoverageRequiredTools` /
`CoverageForbiddenTools` and into spawned subagents.

**Autonomous container execution config keys (issue #389):**

| Config key | Type | Default | CLI equivalent |
|---|---|---|---|
| `autonomy.container.enabled` | bool | `false` | `--container` |
| `autonomy.container.image` | string | `""` | `--container-image` |

When enabled, the daemon runs the verify command inside the container via
`docker run --rm -v <workspace>:/workspace ... <image>`. The Docker binary is an
external runtime dependency; the Go code has no CGO or Docker-SDK dependency (M2).

Session DB: `~/.local/share/sin-code/sessions.db` (SQLite, modernc).
Schema: `sessions(id, created_at, updated_at, title, parent_id)` with
`parent_id TEXT REFERENCES sessions(id) ON DELETE SET NULL` (issue #194) —
recorded automatically by every `Store.Fork`/`ForkEx` invocation and walked
upward by `Store.Tree`. Idempotent `ALTER TABLE` migration for pre-#194 DBs.
Lessons DB: `~/.local/share/sin-code/lessons.db` (SQLite, modernc).
Goal Queue DB: `~/.local/share/sin-code/goals.db` (SQLite, modernc).
Ledger DB: `~/.local/share/sin-code/ledger.db` (SQLite, modernc), overridable
via `SIN_CODE_LEDGER`.
Compress snapshots: `~/.local/share/sin-code/compress-snapshots/<plan-id>.json`
(JSON, one file per Apply), overridable via `SIN_CODE_SNAPSHOT_DIR`. Each
snapshot is content-addressed (Plan.ID is `plan-<sha256-prefix>`) and is
the rollback artifact for `sin-code compress rollback <id>` (issue #172).

### Test-First Verify-Loop (RFC-test-automation.md)

Chat tools that close the loop from code written to tests verified:

| Tool | Policy | Purpose |
|---|---|---|
| `sin_test` | allow | Run tests with `-race`, `-cover`, structured JSON output. |
| `sin_test_generate` | ask | Generate table-driven Go tests (mutating — writes files). |
| `sin_quality_gate` | allow | Pipeline: `build` → `vet` → `test` → optional `staticcheck`/`gosec`/`govulncheck`. |
| `sin_mutation` | allow | Wrap `gremlins unleash` if present on PATH. |
| `sin_fuzz` | allow | Run `go test -fuzz` targets. |
| `sin_property` | allow | Run property-based tests (`rapid` / `testing/quick`). |

Auto-generation: `sin_write`/`sin_edit` automatically invoke `sin_test_generate`
when `SIN_AUTO_GENERATE_TESTS=1` is set or `test.auto_generate = true` is
configured. The default is off for performance/privacy.

**Config keys:**

| Config key | Type | Default | Used by |
|---|---|---|---|
| `test.coverage_threshold` | float (0–100) | `0` | `sin_quality_gate` default |
| `test.mutation_threshold` | float (0–100) | `0` | `sin_mutation` default |
| `test.auto_generate` | bool | `false` | Auto-trigger `sin_test_generate` on writes |
| `test.timeout_seconds` | int | `300` | Default timeout for test tools |
| `test.use_llm` | bool | `false` | `sin_test_generate` calls configured LLM to fill cases |

Hook payload: `tool.post` events for `sin_write`/`sin_edit` now include the
edited file path in `data.path` and expose it as `$SIN_HOOK_DATA_PATH` to
command hooks.

### SIN Fusion v1: Verify-Tournament (issue #290)

| Config key | Type | Default | Used by |
|---|---|---|---|
| `fusion.enabled` | bool | `false` | loopbuilder — enables tournament on verify.fail |
| `fusion.providers` | comma-separated list | `["minimax-m3", "kimi-k2p7-code-fast", "kimi-k2p7-code", "deepseek-v4-pro", "qwen-3p7-plus", "glm-5p2"]` | fireworks_pool — which models to fan out to |
| `fusion.max_cost_usd` | float | `5.0` | tournament — USD kill-switch per invocation |
| `fusion.min_quorum` | int | `1` | tournament — minimum passers before declaring winner |
| `fusion.per_provider_timeout_s` | int | `120` | tournament — per-model wall-clock timeout |
| `fusion.difficulty_gate` | bool | `true` | difficulty.go — filter stylistic failures to legacy retry |
| `fusion.oracle_mode` | bool | `false` | loopbuilder — oracle-mode tournament with judge safety (issue #344) |

Oracle mode is **opt-in and gated**: it only activates when `fusion.enabled = true`, `fusion.oracle_mode = true`, and the gate is in `oracle` mode. Unlike PoC mode, oracle mode does **not** use first-pass-wins; all candidates run to completion, a single judge evaluates all outputs in randomized order, and the highest-scoring candidate wins. Default cost cap is tighter ($2.00) and `fusion__oracle_tournament` is `ask` policy (M4).

### Context Compaction Modes (first PR — compaction-modes)

`cmd/sin-code/internal/agentloop/compaction_types.go` adds a mode-based
compaction layer on top of the legacy `CompactionStrategy` switch
(issue #278). The new API (`Compect2(ctx, CompactInput) CompactResult`)
honours mandate **M3**: verification evidence is always preserved
(`agentloop.compaction_preserve_evidence=true` by default).

| Config key | Type | Default | Used by |
|---|---|---|---|
| `agentloop.context_compaction` | enum | `"off"` | loopbuilder — selects the new compaction mode (off\|deterministic\|llm\|hybrid) |
| `agentloop.compaction_trigger` | enum | `"tokens"` | agentloop — when the compactor fires (turns\|tokens\|both) |
| `agentloop.context_window` | int | `0` (auto) | agentloop — effective token cap; 0 = `CompactionMaxTokens * 4` |
| `agentloop.compaction_preserve_evidence` | bool | `true` | agentloop — keep system prompt, first user goal, last N turns, role:tool messages, and any message matching `VERIFICATION PASSED \| VERIFICATION FAILED \| NOT DONE \| Open acceptance criteria` |
| `agentloop.compaction_recent_turns` | int | `4` | agentloop — number of recent human turns the retain rule keeps |
| `agentloop.compaction_max_tokens` | int | `8000` | agentloop — token budget for compacted messages |

### Session-start context injection (issue #379)

`cmd/sin-code/internal/agentloop/session_context.go` plus the store adapters
in `cmd/sin-code/internal/loopbuilder/session_context_stores.go` assemble a
unified markdown preamble from open todos, the previous session summary, and
the workspace/user `MEMORY.md`. The preamble is injected as a user message
before the Definition-of-Done preamble and the first user prompt, but only
when the session history is empty (i.e. a brand-new session) and the builder
returns a non-empty result. Errors from any store are non-fatal.

| Config key | Type | Default | Used by |
|---|---|---|---|
| `agentloop.session_context.enabled` | bool | `true` | loopbuilder/chat — enables the unified session-start preamble |

**Request-only compaction** (mandate M3): when a non-off Mode is
selected, `Loop.Run` keeps the persisted `msgs` slice full in the
session DB and produces a deterministically compacted view for the
model via `Compact2`. The legacy `Compact()` API (used by
`agentloop.compaction_strategy=…` callers) remains unchanged for
backward compatibility.

**Sidecar snapshots**: lossy modes (`llm`, `hybrid`) write a
content-addressed JSON snapshot to
`~/.local/share/sin-code/context-snapshots/<session-id-hash>/turn-NNNNN.json`
(atomic temp+rename). The loop's persisted history stays complete; the
sidecar is the audit/rollback artifact and the surface that downstream
`compressor_test.go`-style golden harnesses consume (issue #172).

**Byte-stability**: every `Compact2` output is byte-stable per
`(input, Config)` pair so the eval harness (issue #171), the four-arm
comparator, and the compress unit tests can pin deterministic
snapshots.


### Model Performance Registry (issue #395)

\`cmd/sin-code/internal/modelperf/\` — SQLite-backed per-model-per-category
performance database that drives benchmark-based model selection for Fusion.

| Path | Purpose |
|------|---------|
| \`internal/modelperf/store.go\` | SQLite store: upsert, recommend, ranking, categories |
| \`internal/modelperf/benchmark.go\` | Parallel benchmark runner across providers |
| \`fusion_cmd.go\` | \`sin-code fusion benchmark/rank/recommend\` subcommands |

**Schema** (\`modelperf.db\`, \`~/.local/share/sin-code/modelperf.db\`):

\`\`\`sql
CREATE TABLE model_perf (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model TEXT NOT NULL,
    category TEXT NOT NULL,
    dataset TEXT NOT NULL,
    pass_rate REAL NOT NULL,
    avg_latency_ms INTEGER DEFAULT 0,
    avg_cost_usd REAL DEFAULT 0,
    avg_tokens INTEGER DEFAULT 0,
    sample_count INTEGER DEFAULT 1,
    recorded_at TEXT NOT NULL,
    UNIQUE(model, category, dataset)
);
\`\`\`

**Score formula:** \`0.8 * pass_rate + 0.2 * (1 / (1 + avg_cost))\`

**Task categories** (auto-detected from prompt keywords):
\`code-generation\`, \`debugging\`, \`planning\`, \`refactoring\`,
\`review\`, \`documentation\`, \`security\`.

**Integration:** \`loopbuilder\` opens \`modelperf.db\`, detects the task
category from \`Config.TaskDescription\`, queries recommendations, and
sorts providers: recommended first, then the rest. Cold-start (empty DB)
falls back to the full provider pool.

### Verbosity / compression mode (issue #167)

| Config key | Allowed values | Default |
|---|---|---|
| `llm.style` | `default` \| `verbose` \| `normal` \| `terse` \| `ultra` | `default` |

The setting is read at startup and forwarded into
`internal/style.RenderRules` via `wiring.Deps.Style` and
`learning.Learner.BeforeTurn`. The renderer is dependency-free
(contributes bytes only) and is byte-stable per `(mode, skillBody)`
pair — a prerequisite for the system-prompt hash metric (issue #2).
`default` and `verbose` are pass-throughs (no ruleset injected);
`normal`, `terse`, `ultra` emit the caveman-derived ruleset that
drops pleasantries, hedging, and tool-call narration while preserving
every byte of code, URLs, paths, error strings, and `func`/`var`/
`const` names. Every ruleset carries the **auto-clarity** clause
that drops to normal prose around destructive, security-relevant, or
order-sensitive actions (mandate M3, the verification gate is sacred).

Headless JSON contract (stable API — never break without major bump):

```json
{ "session_id": "…", "summary": "…", "verified": true, "turns": 12 }
```

---

## 8. Roadmap (versions 3.0.0 – 3.23.0)

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
| v3.10.0 | ✅ SHIPPED | `--version` flag fix (#38), `.gitignore` for TUI runtime DB (#61), AGENTS.md rollout to 6 ecosystem repos (#40) |
| v3.11.0 | ✅ SHIPPED | `sin update` e2e self-update (#33), security + sbom MCP tools (#36), 36 → 36 subcommands |
| v3.12.0 | ✅ SHIPPED | Tool catalog hub (`internal/hub/`, `hub_cmd.go`), `sin-code hub list/search/info`, 36 → 37 subcommands, closes #35 |
| v3.13.0 | ✅ SHIPPED | Semantic Session Ledger (`internal/ledger/`, `internal/summary/`), `sin-code ledger list/show`, `sin-code summary`, deterministic auto-summaries with verification evidence, 37 → 39 subcommands, closes #43 |
| v3.14.0 | ✅ SHIPPED | Unified config subsystem (#34): `sin-code config init/show/validate`, expanded TOML schema, user + project deep merge, atomic writes, secret masking, 39 subcommands |
| v3.15.0 | ✅ SHIPPED | Go-native SCA Phase 1 (#41), race-flake hardening (#59) |
| v3.16.0 | ✅ SHIPPED | Forge integration (#37): `sin forge` command, `sin status` detection, 16th MCP tool in `mcp_config` |
| v3.18.0 | ✅ SHIPPED | Memory compaction (`internal/compress/`, `compress_cmd.go`, issue #172): deterministic dedupe + byte-budget + sort, optional LLM summarization (`--strategy llm|hybrid`) with byte-preservation validation, snapshot+rollback for lossless Apply, `compress__plan/apply/rollback` permission defaults (ask-only on `apply`). Closes #172. |

| v3.18.0 | ✅ SHIPPED | `sin-code install` + curl|bash shim + PowerShell (issue #170): new 40th subcommand + internal/install/ package, 27-line install.sh mirror + 35-line install.ps1, SHA256-verified single-binary downloads from goreleaser assets |
| (next)  | TBD    | eval/trace infra hardening + first-party golden-dataset CI gate (issue #75 phase 2) |
| v3.19.0 | ✅ SHIPPED | `sin-code serve --compress-tools` (issue #173): ponytail-tag compressor in `internal/mcpcompress/` shrinks the 47-tool manifest on the wire. Tag set `delete|stdlib|native|yagni|shrink`, subset via `--compress-tags`, savings reported via `--print-stats`. Tool names, schemas, and behavior are unchanged (AGENTS.md §10). Closing #173. |

| v3.18.0 | ✅ SHIPPED | `sin-code debt` (issue #177) — `// sin-debt: <ceiling>, upgrade: <trigger>` marker convention (ponytail adoption), byte-stable `internal/sindept/` scanner + report, policy gate via `sin-code debt check`, 41st subcommand; alongside `sin-code install` (issue #170), `eval` / `trace` (issue #75), `evalset` / `prp` / `instinct` / `assets` / `hooks` / `rtk` / `codegraph` / `spec` v0 work |
| (next)  | TBD    | eval/trace infra hardening + first-party golden-dataset CI gate (issue #75 phase 2) |
| v3.19.0 | ✅ SHIPPED | `autoactivate` hooklife subpackage (#176): `cmd/sin-code/internal/hooklife/autoactivate` — per-session rule injection on `SessionStart` + `UserPrompt`. `--activate <rule>` + `--no-trigger` flags on `sin-code chat`; project-local `.sin-code/autoactivate.toml`; deterministic byte-stable `RuleSet.Render()`; race-safe (mandate M7). Hooks register against any `*hooklife.Registry` via `Activator.Register(reg)`. |
| v3.17.0 | ✅ SHIPPED | TUI runtime DB .gitignore, MCP warning deduplication, marketplace update test hardening, skills 33 → 34 |
| v3.18.0 | ✅ SHIPPED | `sin-code install` single-binary installer (issue #170), curl/bash + PowerShell shims, SHA256-verified release downloads |
| v3.19.0 | ✅ SHIPPED | `sin-code review --complexity` (issue #179): `internal/complexity/` static analyzer with ponytail 5-tag format (`delete`, `stdlib`, `native`, `yagni`, `shrink`), `// sin-debt:` marker support (issue #177), text/json/markdown output, race-clean tests |
| v3.20.0 | ✅ SHIPPED | Tool coverage, M6 enforcement, catalog, telemetry (issues #249, #253, #248, #250, #252, #251): agent profiles expose full `sin_*` + MCP prefix surface; system prompt injects SIN-tool preference fragment; runtime `ToolCoverageEnforcer` rejects missing/forbidden tool usage; `ledger tools` heatmap/coverage/unused; orchestrator planner emits mandatory `ToolChain` per intent; `sin-code catalog` unifies 46+ MCP tools, 17+ chat tools, and 14+ external MCP prefixes. |
| v3.20.0 | ✅ SHIPPED | `sin-code image-graph` — SOTA ECharts chart generation (bar/line/pie/area); `skill-github-readme` bundled. 42 subcommands, 37 bundled skills. |
| v3.21.0 | ✅ SHIPPED | Test-First Verify-Loop (RFC-test-automation.md): `sin_test` + `sin_test_generate` + `sin_quality_gate` + `sin_mutation` + `sin_fuzz` + `sin_property`; `tool.post` hook payload path; `test.*` config keys; `evals/test-generation.json` golden dataset. |
| v3.22.0 | ✅ SHIPPED | SIN Fusion v1 enhancements: **Plan-Merge mode** (issue #393 — N planners → judge merges → 1 coder → verify, preserves all insights); **Oracle as default** (issue #394 — quality over cost, was: PoC first-pass-wins); **Model Performance Registry** (issue #395 — `modelperf.db`, `fusion benchmark/rank/recommend` CLI, auto-wired into `loopbuilder`). |
| v3.22.0 | ✅ SHIPPED | pre-existing fixes stacked into the same release: `llm/provider.go`+`recorder.go`+`stream.go` ThinkingTokens + 8-arg `RecordUsage`; `agentloop/compaction_helpers.go` for `compaction_types.go`; vet fix in `orchestrator/event_dispatch_test.go`. |
| v3.23.0 | ✅ SHIPPED | v3.23.0 roadmap batch 1: autonomous research report (`sin-code research`, issue #384), SWE-bench test suite (issue #363), scientific research skill (issue #387), `sin_apply_diff`/`sin_generate_diff` chat tools (issue #365), dynamic MCP server discovery (`sin-code mcp discover/add`, issue #368). Also repaired main build drift from parallel-agent WIP. |
| v3.23.0 | ✅ SHIPPED | v3.23.0 roadmap batch 2: sin_edit/MCP surgical-editor unification (issue #373), reactive permission policy from tool results (issue #374), offline semantic tool retrieval (issue #364), containerized execution for autonomous goals (issue #389). WebUI daemon management (issue #388) closed as out-of-scope for this repo. |

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
- The catalog kinds (`agent`, `command`, `skill`, `hub`, `mcp`, `chat`,
  `external`) and the `internal/orchestrator.ToolChain` type are public API
  surfaces for downstream tooling and tests. Adding kinds/tools is non-breaking;
  renaming or removing a kind is a major bump.
- The string "SIN-Code-Bundle" may only appear in CHANGELOG history and
  migration notes — never in code, config, or new docs (mandate M5).

### Bundled skill naming rules

All bundled skills live under `skills/<category>-skills/` and are named
`skill-<category>-<descriptive-name>`. Examples:

| Category | Directory | Skill name |
|---|---|---|
| Code | `code-skills/` | `skill-code-audit`, `skill-code-build`, `skill-code-create` |
| GitHub | `github-skills/` | `skill-github-actions`, `skill-github-account`, `skill-github-app` |
| Memory | `memory-skills/` | `skill-memory-honcho`, `skill-memory-infisical` |
| Process | `process-skills/` | `skill-process-goal`, `skill-process-grill`, `skill-code-lazy` |
| Shop | `shop-skills/` | `skill-shop-cj-dropshipping`, `skill-shop-stripe` |

Each skill **must** contain:
- `SKILL.md` with frontmatter (`name`, `description`, `license`, `compatibility`, `metadata`)
- `context/`, `frameworks/`, `tasks/`, `templates/` directories
- `LICENSE` file

Skills ported from external repos (e.g. `Infra-SIN-OpenCode-Stack`,
`DietrichGebert/ponytail` → `skill-code-lazy`) must include
`lifecycle: external` and `sources:` in their metadata.

### Skill distribution to external agents (issue #169)

`sin-code skill install <name> --agent <target>` distributes a bundled
Skill artifact to one of eleven registered agent families. The single
source of truth is `cmd/sin-code/internal/skilldist/Targets`:

| Target | Format | Install path template (relative to `$SIN_CODE_HOME`) |
|---|---|---|
| `claude-code` | `dir` | `.claude/skills/<skill>` |
| `opencode` | `dir` | `.config/opencode/skills/<skill>` |
| `gemini` | `dir` | `.gemini/skills/<skill>` |
| `codex` | `rule` | `.codex/rules/<skill>.md` |
| `cursor` | `rule` | `.cursor/rules/<skill>.mdc` |
| `windsurf` | `rule` | `.windsurf/rules/<skill>.md` |
| `cline` | `rule` | `.clinerules/<skill>.md` |
| `copilot` | `marker` | `.github/copilot-instructions.md` |
| `aider` | `rule` | `.aider/conventions/<skill>.md` |
| `continue` | `rule` | `.continue/rules/<skill>.md` |
| `zed` | `rule` | `.zed/rules/<skill>.md` |

**Marker-fence contract.** Every write for `rule` and `marker` Formats
goes through `ParseMarkers` so a subsequent install with the same
`(target, skill)` pair replaces the previously written block in place:

```
<!-- SIN-CODE-SKILL-START: <skill> -->
… rendered body …
<!-- SIN-CODE-SKILL-END:   <skill> -->
```

The leading/trailing ASCII prefix `SIN-CODE-SKILL` is the rg-friendly
anchor. The trailing whitespace before `<skill>` on the END marker is
visual alignment only — `ParseMarkers` strips it on lookup, so any
strict matcher outside this package works too.

**Public API surface.** `(Target.Name, Target.DisplayName)` is exposed
via the `--agent <name>` flag. Adding a target is non-breaking; renaming
or removing one is a major bump. The `sin-code skill list --json`
output schema is also a public API — preferred-format changes go through
the same major-bump policy.
### Naming-convention note (issue #178)

The canonical pattern is `skill-<category>-<name>` (i.e. the
middle token matches the directory category, e.g.
`skill-process-lazy`). **`skill-code-lazy`** is the v3.18.0
historical exception preserved for activation-keyword stability
(`lazy_skill` binds to the literal name as it appears in
`SKILL.md:frontmatter.name`). New bundled skills must follow the
canonical pattern; renaming `skill-code-lazy` would require a
major bump because the `lazy_skill` keyword is part of the
external activate-mode contract (issue #176).

### CLI subcommands (verified `cmd/sin-code/main.go`, v3.5.0)

```
Core:      discover, execute, map, grasp, scout, harvest, orchestrate,
           ibd, poc, sckg, adw, oracle, efm
Agents:    chat, sessions, mcp, goal, daemon, skill, superpowers,
           vane, stack, gh, install
           vane, stack, gh, install, profile
           vane, stack, gh, debt
           (sessions: list|show|rm|fork|tree, issue #194)
Frontend:  serve, tui, webui
Lifecycle: memory, knowledge, todo, notifications, orchestrator_run,
           orchestrator_agents, orchestrator_plan, update
Utility:   read, write, edit, lsp, plugin, index, security, sbom,
           config, self-update, hub, ledger, summary
``` (v3.18.0: 40 subcommands; `install` is the v3.18.0 single-binary installer from issue #170)
           config, self-update, hub, ledger, summary, compress
``` (v3.18.0: 40 subcommands, up from 39 in v3.13.0)
``` (v3.18.0: 40 subcommands, up from 39 in v3.16.0;
`install` is the v3.18.0 single-binary installer from issue #170;
`profile` is the v3.18.0 single-source-of-truth per-agent renderer
from issue #175.)

### Per-agent profile distribution (issue #175)

`sin-code profile` renders the single in-repo source
(`docs/agent-profiles/sin-profile.md`, ≤80 lines, KISS) into every
per-host-agent mirror file. The render is **byte-stable** per
`(target, source)` pair; the verify gate (`sin-code profile verify`)
refuses to pass whenever any mirror drifts off the source SHA. Adding
a target is non-breaking; renaming or removing one is a major bump.
The single source of truth for the table is
`cmd/sin-code/internal/profile/target.go` — keep the AGENTS.md row in
sync if the table moves.

| Target        | Format  | Install path (relative to repo root)                       |
| ------------- | ------- | ----------------------------------------------------------- |
| claude-code   | dir     | `.claude/skills/sin-code/SKILL.md`                          |
| opencode      | dir     | `.config/opencode/skills/sin-code/SKILL.md`                 |
| gemini        | dir     | `.gemini/skills/sin-code/SKILL.md`                          |
| codex         | rule    | `.codex/rules/sin-code.md`                                  |
| cursor        | rule    | `.cursor/rules/sin-code.mdc`                                |
| windsurf      | rule    | `.windsurf/rules/sin-code.md`                               |
| cline         | rule    | `.clinerules/sin-code.md`                                   |
| copilot       | marker  | `.github/copilot-instructions.md`                           |
| aider         | rule    | `.aider/conventions/sin-code.md`                            |
| continue      | rule    | `.continue/rules/sin-code.md`                               |
| zed           | rule    | `.zed/rules/sin-code.md`                                    |

The four marker-fence outputs (`rule` + `marker`) wrap the body in
`<!-- SIN-CODE-SKILL-START: sin-code -->` … `<!-- SIN-CODE-SKILL-END:   sin-code -->`
inside the file. The fence is **byte-identical** to the one
`internal/skilldist` uses (issue #169): the two systems share the
same anchor (`SIN-CODE-SKILL`) so a downstream parser finds both
per-skill bundles and per-profile mirrors with one regex. Profile
renders are idempotent (rerun with unchanged source = byte-identical
output), exactly as skilldist demands.
``` (v3.18.0: 42 subcommands — `debt` is issue #177, sin-debt markers; `install` is issue #170; `eval`, `evalset`, `prp`, `instinct`, `assets`, `hooks`, `rtk`, `codegraph`, `spec` follow)
Audit:     audit, ceo-audit
``` (v3.18.0: 42 subcommands, up from 39 in v3.13.0)
           config, self-update, hub, ledger, summary, review, image-graph
``` (v3.20.0: 42 subcommands — `image-graph` added: SOTA ECharts chart generation)

### Hook events (verified `internal/hooks/hooks.go`, v3.5.0)

24 events: `session.{start,resume,end}`, `turn.{start,end}`,
`tool.{pre,post,denied,error}`, `permission.ask`, `verify.{pre,pass,fail}`,
`agent.{spawn,complete}`, `critic.reject`, `adversary.finding`,
`governor.block`, `memory.{write,compact}`, `commit.{pre,post}`,
`push.pre`, `task.{complete,abort}`, `compaction.pre`,
`goal.{enqueued,started,verified,exhausted}` (v3.5.0),
`trigger.fired` (v3.5.0), `skill.{installed,failed}` (v3.5.0),
`fusion.dispatch` (v3.22.0, issue #290).

### Ponytail tag set — MCP-tool-description compressor (issue #173, v3.19.0)

`cmd/sin-code/internal/mcpcompress/` (introduced in v3.19.0) is the
canonical ponytail-tag compressor for `sin-code serve --compress-tools`.
The five canonical tags control which rules run:

| Tag | Rule | Drops |
|---|---|---|
| `delete` | `DeleteHedges` | pleasantries / hedge adverbs ("safely", "carefully", "thoroughly", "robustly", "elegantly", "gracefully", "seamlessly", "effortlessly", "smoothly", "meticulously") |
| `stdlib` | `StdlibPatterns` | redundant `(via stdlib)` parentheticals; "Go stdlib" / "Python stdlib" / "Rust stdlib" / "Java stdlib" / "JavaScript stdlib" / "TypeScript stdlib" adjectives |
| `native` | `DropTrimEncouragement` | M6 tail clauses `Always prefer over native X` / `Prefer sin_X over native Y`; leading `Use sin_X when possible, …` encouragements |
| `yagni` | `YagniPatterns` | speculative `(experimental)` / `(TBD)` / `(TBA)` / `(reserved)` / `(may be deprecated …)` parentheticals; bare `TBD` / `TBA` words |
| `shrink` | `ShrinkExamples` | redundant `(e.g. …)` / `(such as …)` parenthetical examples |

**Public API contract.** The five tag strings are part of the public
configuration surface; renaming any is a major bump. The five `Rule`
constructor types (`RuleDeleteHedges`, `RuleStdlibPatterns`,
`RuleDropTrimEncouragement`, `RuleYagniPatterns`, `RuleShrinkExamples`)
are part of the public Go surface for downstream embeds; renaming is a
major bump. Adding a sixth rule is non-breaking. The `sin-code serve
--print-stats` text-table format is part of the documented telemetry
surface — additions are non-breaking, removals/reorderings are major.

**Byte-stability.** Every Rule and the Pipeline are byte-stable per
`(input, Pipeline)` pair. The `compressor_test.go` golden suite is the
single source of truth — any rule regex / order change must update the
golden expectations in the same commit. This is a prerequisite for the
system-prompt hash metric (issue #2).

**Names are immutable.** The 44+ MCP tool `Name` field is public API
(§10). The compressor mutates `Description` only. `CompressSpec`
asserts this in `TestCompressSpec_NameMutable`.
### sin-debt marker convention (issue #177)

Every **intentional** shortcut in source code is marked in-line with:

```
// sin-debt: <ceiling>, upgrade: <trigger>
// sin-debt: <ceiling>            # upgrade clause is OPTIONAL but RECOMMENDED
```

- Recognised comment families: `//`, `#`, `--`, `/* … */`, `<!-- … -->`.
- The `upgrade:` clause names the trigger to revisit; markers without it
  are **rot-risk** and surface in the `debt stats` rot-risk table.
- Authoring template: prefer the canonical reasons catalogued in
  `cmd/sin-code/internal/sindept/policy.go` (`DefaultReasons`) and the
  upgrade triggers in `UpgradeTriggers`. Free-form text is allowed;
  pick from the catalogue when possible.
- The scanner (`internal/sindept`) and CLI (`sin-code debt`) read this
  format; the complexity auditor (issue #179) and audit-engine
  (issue #180) recognise it as an "approved shortcut" tag.
- Byte-stable: the same source tree emits byte-identical reports
  (`sin-code debt stats`) so the four-arm comparator (issue #171)
  can pin its golden snapshot.
- Policy file: `.sin-code/debt-policy.toml` (see
  `cmd/sin-code/debt_cmd.doc.md`).
### Hook events (verified `internal/hooklife/event.go`, v3.19.0)

Seven phases for the second, programmatic hooking system:

| Phase | Verdict caps | Used by |
|---|---|---|
| `PreToolUse` | Block allowed | `block-no-verify`, `config-protection`, `quality-gate` |
| `PostToolUse` | Warn (aggregated) | `post-edit-format`, `post-edit-typecheck`, `cost-tracker` |
| `Stop` | Warn (aggregated) | `suggest-compact` |
| `SessionStart` | Warn (aggregated) | `autoactivate-session-start` (issue #176) |
| `SessionEnd` | Warn (aggregated) | (reserved) |
| `PreCompact` | Warn (aggregated) | `suggest-compact` (privacy-first) |
| `UserPrompt` | Warn (aggregated) | `autoactivate-user-prompt` (issue #176) |

Auto-activation hooks (v3.19.0, issue #176) wire through
`Activator.Register(reg)` and emit their rule body via the `Decision.Message`
field, returning `Warn` so the runner's aggregation surfaces it. Off by
default — privacy-first activation via `--activate` or
`.sin-code/autoactivate.toml`. See
`cmd/sin-code/internal/hooklife/autoactivate/activator.doc.md`.

---

## 11. For external AI agents (Claude Code, Codex, etc.) working here

1. Read sections 3 (mandates) and 8 (roadmap) before any edit.
2. Use the repo's own tools on itself when available (`sin-code serve`
   dogfooding).
3. One C-gap or one issue per PR. No drive-by refactors.
4. If you cannot satisfy a mandate, STOP and report — do not work around it.
5. WebUI-v2 is OUT OF SCOPE for this repo's agent loop. Edits to
   `/Users/jeremy/dev/sin-code-web-ui-v2` belong to that repo's local agent.

### 11.1 Known runtime DB locations

The Go binary writes SQLite DBs to user-config directories by default
(issue #265 / #62). Workspace isolation is keyed by a 12-char sha256
of the workspace's absolute path so two projects never share a row.

| Purpose          | Path (relative to `os.UserConfigDir()/sin-code/`)        | Module                                     | Issue  |
| ---------------- | -------------------------------------------------------- | ------------------------------------------ | ------ |
| Lessons log      | `workspaces/<sha256-prefix12(abs(ws))>/lessons.db`      | `internal/tui/dbhome.go` + `agent_runner.go` | #265   |
| Session store    | `workspaces/<sha256-prefix12(abs(ws))>/sessions.db`     | `internal/tui/dbhome.go` + `agent_runner.go` | #265   |
| Code index       | `<root>/.sin-code/index.bin` (per-project, intentional) | `internal/index_store.go`                  | n/a    |

`internal/session/store.go:DefaultPath()` and
`internal/lessons/store.go:DefaultPath()` keep their
`~/.local/share/sin-code/` shape for direct CLI use (`sin-code sessions`
without a workspace). The TUI agent runner resolves the home through
`Config.DBHome` (overridable in tests; default = `os.UserConfigDir()`).

These files are NOT inside the workspace tree and so cannot be
accidentally `git add`-ed. The legacy `.gitignore` rule (lines 64–65)
remains as a defence-in-depth for users who keep a pre-migration
working tree. Migration to add *automatic* move-from-old-path is
out of scope for issue #265.

---

## 12. Evaluation & Observability System (issue #75)

SIN-Code ships a first-party eval pipeline + OpenTelemetry tracing
that lets agents produce **golden datasets**, run them as CI gates,
and inspect trajectories visually (Langfuse / Jaeger / Arize Phoenix).

### Components

| Path | Purpose |
|------|---------|
| `cmd/sin-code/internal/trace/provider.go` | OTel TracerProvider bootstrap (stdout / OTLP-HTTP / noop) |
| `cmd/sin-code/internal/trace/hook_listener.go` | Decorator wrapper around `*hooks.Engine` that emits one span per Fire() |
| `cmd/sin-code/internal/dataset/dataset.go` | Pure-stdlib Golden Dataset JSON parser + validator |
| `cmd/sin-code/internal/dataset/runner.go` | Executes TestCases against the existing `agentloop.Loop` |
| `cmd/sin-code/internal/eval/judge.go` | LLM-as-a-Judge via `internal/llm.Client` |
| `cmd/sin-code/internal/eval/metrics.go` | Summary aggregation + JSON envelope for CI |
| `cmd/sin-code/internal/evalharness/arms.go` | v3.18.0 four-arm constructors (baseline / terse / lazy / `<skill>`) for issue #171 |
| `cmd/sin-code/internal/evalharness/comparator.go` | v3.18.0 `Compare` runner producing per-arm aggregates |
| `cmd/sin-code/internal/evalharness/prices.go` | v3.18.0 self-pricing price book (USD/1k tokens per model) |
| `cmd/sin-code/internal/evalharness/snapshot.go` | v3.18.0 deterministic snapshot round-trip (caveman evals/README.md §3) |
| `cmd/sin-code/eval_cmd.go` | `sin-code eval run` + `eval compare` + `eval snapshot` + `eval diff` (issue #171) |
| `cmd/sin-code/internal/evalharness/scorer.go` | Scorers: exact, contains, success, LLM-judge, composite, **CompileAndRun** |
| `cmd/sin-code/internal/evalharness/runner_extras.go` | Compile-and-run helpers (code extraction, sandboxed compile/run) |
| `cmd/sin-code/eval_cmd.go` | `sin-code eval run` + `sin-code eval list` |
| `cmd/sin-code/trace_cmd.go` | `sin-code trace doctor` — exporter-only sanity check |
| `evals/critical.json` | Example Golden Dataset (3 cases, no LLM needed) |
| `evals/three-arm-example.json` | v3.18.0 four-arm bench, 3 cases (issue #171) |

### CLI surface

```bash
# Run a Golden Dataset (CI format)
sin-code eval run --dataset evals/critical.json \
    --min-pass-rate 0.95 --json

# With tracing exported to OTLP (Langfuse / Jaeger / Phoenix)
sin-code eval run --dataset evals/critical.json \
    --trace --trace-exporter otlp \
    --trace-endpoint langfuse.local:4318

# LLM-as-a-Judge pass
sin-code eval run --dataset evals/critical.json \
    --judge-model gpt-4o --judge-endpoint https://api.openai.com/v1

# Four-arm comparator (issue #171) — baseline / terse / lazy_skill / <user-skill>
sin-code eval run --dataset evals/three-arm-example.json \
    --arm baseline,terse,lazy_skill,skill-code-create
sin-code eval compare --dataset evals/three-arm-example.json
sin-code eval snapshot --dataset evals/three-arm-example.json --out /tmp/snap.json
sin-code eval diff --snapshot /tmp/snap-base.json --snapshot-b /tmp/snap-head.json
# Compile-and-run scorer (ponytail correctness.js analog)
sin-code eval run --dataset evals/coding.json \
    --scorer compile-and-run --language python \
    --self-check "assert fizzbuzz(15) == 'FizzBuzz'"

# YAGNI mode: trivial one-liners accepted after compile-only
sin-code eval run --dataset evals/trivial.json \
    --scorer compile-and-run --language python --skip-test

# Sanity-check the OTel exporter setup without a full eval
sin-code trace doctor --exporter stdout --emit-sample-span
```

### 12.1 Four-arm comparator (issue #171)

The comparator runs the same dataset against the four arms
identified in the issue body:

| Arm | Reserved ID | SystemPrompt |
|---|---|---|
| baseline | `__baseline__` | empty (legacy single-arm behaviour) |
| terse | `__terse__` | `"Answer concisely."` |
| lazy skill | `__lazy_skill__` | terse-prefixed body of `skill-code-lazy` (issue #178) |
| user skill | `<user-supplied>` | terse-prefixed body of the bundled skill named by `--skill` |

The **honest delta** between any candidate arm and the `terse`
arm is what reviewers grade on. Comparing a skill directly to the
baseline conflates the skill's content with the generic "be
terse" instruction; the four-arm harness isolates them.

The output matrix mirrors ponytail's
`benchmarks/README.md:34-58`:

| column | meaning |
|---|---|
| `pass_rate` | `Passed / TotalCases` |
| `med_LOC` | median lines of output across cases |
| `med_latency_ms` | median wall-clock per (case, arm) |
| `med_usd` | median USD cost (using `prices.go` price book) |
| `med_tokens` | median prompt + completion tokens |
| `med_score` | median `Result.Score` per arm |

Snapshots are deterministic JSON: the comparator sorts every arm,
median-recomputes every cell, and writes the same bytes for the
same input on every CI run (caveman evals/README.md §3 promise:
"snapshot committed to git so CI runs are deterministic and free").

### Hard mandates honored

- **M2 (single static binary, CGO_ENABLED=0):** pure-Go OTel SDK at
  `go.opentelemetry.io/otel/sdk@v1.24.0` plus `stdouttrace` +
  `otlptracehttp`. No CGO.
- **M4 (permission engine):** eval/trace rules registered in
  `cmd/sin-code/internal/permission_defaults.go` (`eval__list` allow,
  `eval__run` ask, `trace__doctor` allow).
- **M5 (module path):** the import path is
  `github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/...` everywhere.
  No `SIN-Code-Bundle` references.
- **M7 (race-free):** every new test passes
  `go test -race -count=1 ./cmd/sin-code/internal/{trace,dataset,eval,evalharness}/...`.

### Reference documentation

- `cmd/sin-code/internal/trace/provider.doc.md` — exporter / sampler
  configuration surface.
- `cmd/sin-code/internal/trace/hook_listener.doc.md` — list of
  divergences between the issue's reference code and the actual hook
  API (the issue describes a `hooks.Manager.On(...)` API that does
  not exist; we use the decorator-on-`Engine.Fire` pattern that
  matches the real `hooks.go:108-143` interface).
- `cmd/sin-code/internal/dataset/dataset.doc.md` — JSON schema contract.
- `cmd/sin-code/internal/dataset/runner.doc.md` — agent-loop API
  divergence notes (real `Loop.Run(ctx, *session.Session, string)`
  vs reference `Loop.Run(ctx, sessID, prompt, RunOptions)`).
- `cmd/sin-code/internal/eval/eval.doc.md` — judge prompt + JSON
  contract; `JudgeResult` schema.
- `cmd/sin-code/internal/evalharness/comparator.doc.md` — four-arm
  `Compare` runner, arm constructors, snapshot round-trip (issue #171).
- `cmd/sin-code/internal/evalharness/snapshot.doc.md` — deterministic
  matrix + diff (issue #171).
- `cmd/sin-code/internal/evalharness/scorer.doc.md` — scorer interface
  and built-in scorers (including `CompileAndRun`).
- `cmd/sin-code/internal/evalharness/runner_extras.doc.md` — code-block
  extraction, per-language compile, and sandboxed self-check execution.
- `docs/eval.md` — user-facing guide for `sin-code eval`.
- `evals/README.md` — Golden Dataset catalog and schema quick reference.
- `.github/workflows/eval-n8n.yml` — n8n-delegated CI for datasets (M1).

