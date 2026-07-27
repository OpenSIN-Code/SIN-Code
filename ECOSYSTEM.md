# OpenSIN-Code Ecosystem Inventory

> Single source of truth for every repo developed FOR SIN-Code and its
> integration status. CI and agents read the table below — keep it in sync
> with `requirements-ecosystem.txt` and `cmd/sin-code/internal/mcpclient/config.go`.

## Core

| Repo | Role | Integration | Status |
|---|---|---|---|
| SIN-Code | Main agent CLI/TUI/WebUI | — | ACTIVE |
| SIN-Code-WebUI-v2 (`sin-code-web-ui-v2`) | Official web frontend | `sin-code webui` (see docs/WEBUI.md) | ACTIVE |
| SIN-Code-Orchestration | Multi-agent orchestration | Go import / subprocess | ACTIVE |
| SIN-Code-Review-Interface | Review workflow UI | subprocess | ACTIVE |

## Verification & Analysis Tools (issue #28 six-pack + Symfony)

| Repo | Tool prefix | Integration | Status |
|---|---|---|---|
| SIN-Code-PoC-Tool | `poc_*` | CLI subprocess (verify gate runner) | ACTIVE |
| SIN-Code-Oracle-Tool | `oracle_*` | CLI subprocess (verify gate runner) | ACTIVE |
| SIN-Code-SCKG-Tool | `sckg_*` | CLI subprocess | ACTIVE |
| SIN-Code-IBD-Tool | `ibd_*` | CLI subprocess | ACTIVE |
| SIN-Code-ADW-Tool | `adw_*` | CLI subprocess | ACTIVE |
| SIN-Code-EFM-Tool | `efm_*` | CLI subprocess | ACTIVE |
| SIN-Code-Forge-Tool | `forge_*` | CLI subprocess + MCP server | ACTIVE |
| SIN-Code-Symfony-Lens | `symfonylens__*` | MCP server (config.go) | ACTIVE |
| SIN-Code (security) | `sin_security_scan` (v3.11.0) | MCP tool (serve.go) | ACTIVE |
| SIN-Code (sbom) | `sin_sbom_generate` (v3.11.0) | MCP tool (serve.go) | ACTIVE |
| SIN-Code (run_loop) | `sin_run_loop` (unreleased) | MCP tool (serve.go) — synchronous full-agent-loop delegation | ACTIVE |
| SIN-Code (goal queue) | `sin_goal_add` / `sin_goal_list` / `sin_goal_status` / `sin_goal_complete` (unreleased) | MCP tool (serve.go) — asynchronous autonomy goal queue | ACTIVE |

> **opencode / Claude Code / Codex integration:** any MCP client can now
> delegate a full verified task synchronously via `sin_run_loop` (one call,
> one verified result — PLAN→ACT→VERIFY→DONE in-process), or enqueue
> autonomous goals asynchronously via `sin_goal_add` and poll progress via
> `sin_goal_status` / `sin_goal_list`. This brings the total to 49+ MCP tools
> exposed by `sin-code serve`.

## MCP Skill Servers (config.go defaults)

| Repo | Server name / tool prefix | Default policy | Status |
|---|---|---|---|
| web_search_bundle | `websearch__*` | allow | ACTIVE |
| vane (bridged, never vendored) | `vane__*` | allow | ACTIVE |
| SIN-Code-Context-Bridge-Skill | `contextbridge__*` | allow | ACTIVE |
| Simone-MCP | `simone__*` | allow | ACTIVE |
| SIN-Code-Scheduler-Skill | `scheduler__*` | ask | ACTIVE |
| SIN-Code-Goal-Mode-Skill | `goalmode__*` | ask | ACTIVE |
| SIN-Code-Grill-Me-Skill | `grillme__*` | ask | ACTIVE |
| SIN-Code (bundled Marketplace) | `marketplace__*` | ask | ACTIVE |
| SIN-Code-Doc-Coauthoring-Skill | `codocs__*` | ask | ACTIVE |
| SIN-Code-Honcho-Rollback-Skill | `honcho__*` | ask (destructive) | ACTIVE |
| SIN-Code-Frontend-Design-Skill | `frontend__*` | ask | ACTIVE |
| SIN-Code (bundled MCP Server Builder) | `mcpbuilder__*` | ask | ACTIVE |
| SIN-Browser-Tools | `browser__*` (106 tools: navigation, click/type/fill, screenshots, PDF, tab/session, cookie/storage, diagnostics, Shadow DOM, OOPIF, SPA wait, network mocking, macOS Spaces, screen recording) | allow (35 read-only) + ask (71 mutating) — per-tool M4 split policy | ACTIVE |
| GitHub CLI (gh) | `gh_query`, `gh_health`, `gh_execute` | allow / allow / ask (M4) | ACTIVE |
| [OpenSIN-Code/sin-analyse-suite](https://github.com/OpenSIN-Code/sin-analyse-suite) | `analyse__*` (image, video, PDF, logs, data, audio) | allow (read-only) | ACTIVE |
| SIN-Code-Share-Skill | `share__*` | ask | DEPRECATED *(policy-only; no filesystem backing)* |
| SIN-Code-Skills-Skill | `skills__*` | ask | DEPRECATED *(policy-only; no filesystem backing)* |

| [OpenSIN-Code/autodev-cli](https://github.com/OpenSIN-Code/autodev-cli) | `autodev__*` (e.g. `autodev_status`, `autodev_lessons`, `autodev_init`, `autodev_run_experiment`, `autodev_swarm`, `autodev_session_log`) | allow (read-only) + ask (mutating) — split M4 policy | ACTIVE |
| SIN-Code (native_browser) | `native_browser__*` (e.g. `native_browser__navigate`, `native_browser__snapshot`, `native_browser__screenshot`) | allow (read-only) + ask (mutating) — split M4 policy (issue #382) | ACTIVE |
| SIN-Code (research) | `research__dry_run`, `research__list`, `research__show`, `research__run` | allow / allow / allow / ask — split M4 policy (issue #384) | ACTIVE |
| vibe-notion (Bridged-External) | `notion__notion_read_*` (10 read tools), `notion__notion_write_*` (6 write tools), `notion__notion_raw_cli` | allow (reads) + ask (writes) — split M4 policy | ACTIVE |
| youtube-for-ai-agents (Bridged-External) | `youtube__*` (6 read tools, 3 download/edit tools) | allow (reads) + ask (download/clip/highlight) — split M4 policy | ACTIVE |
| Template-fuer-Repo-Skill | Template for infrastructure skill repos | ACTIVE |
| kubernetes-sota-practices | K8s best practices (Helm, k3s, HPA, Istio) for Code-Swarm & OpenSIN | ACTIVE |

## LLM Backends

| Repo | Integration | Status |
|---|---|---|
| SIN-Code-FireworksAI-OpenCode-Config | Agent profile `fireworks` (profiles/fireworks.toml) | ACTIVE |
| ~~coder-SIN-Qwen~~ | ~~Agent profile `qwen-relay` (profiles/qwen-relay.toml)~~ | ARCHIVED *(per AGENTS.md §5; return to life only when repo restored)* |

## Deprecated / Archived

| Repo | Superseded by | Action |
|---|---|---|
| SIN-Code-Slash-Skill | `internal/commands` (C8, in-tree since v3.2.0) | ARCHIVE |
| SIN-Code-Marketplace-Skill | `sin marketplace` + bundled MCP module | ARCHIVE after #512 |
| SIN-Code-MCP-Server-Builder-Skill | `sin mcp-server` + bundled MCP module | ARCHIVE after #512 |
| SIN-Code-Security-Bundle | in-tree Go vendors: SIN-Code-SAST-Tool, SIN-Code-SBOM-Generator-Go, SIN-Code-SCA-Tool-Go, SIN-Code-Secrets-Scanner | ACTIVE (vendored) |
| SIN-Code-Security-Bundle-Python | `python -m sin_code_bundle.tools.security` | DEPRECATED (use vendored Go tools) |

## Methodology Stack (v3.8.0)

| Layer | Upstream | License | Tool | Status |
|---|---|---|---|---|
| Context (AGENTS.md) | agent0ai/dox | MIT | `dox__*` | ACTIVE |
| Methodology | obra/superpowers | MIT | `superpowers__*` | ACTIVE |
| Research (cited sources) | ItzCrazyKns/Vane | MIT | `vane__*` | ACTIVE |
| Tools | (this repo) | MIT | `sin_*` | ACTIVE |

| Helper | What it does | License | Command | Status |
|---|---|---|---|---|
| sin-code stack | Unified install/doctor across 3 layers | MIT | `sin-code stack install` | ACTIVE |

## Sync rules

1. Every new repo in the org MUST be added here in the same PR that creates it.
2. Every MCP skill server MUST have an entry in
   `cmd/sin-code/internal/mcpclient/config.go` and a policy line in
   `cmd/sin-code/internal/permission_defaults.go`.
3. `requirements-ecosystem.txt` lists pinned versions for everything marked ACTIVE.
4. Every backend migration (e.g. SIN-Code-FireworksAI-OpenCode-Config →
   `profiles/fireworks.toml`) MUST leave a deprecation note in the source
   repo's README before archiving.
