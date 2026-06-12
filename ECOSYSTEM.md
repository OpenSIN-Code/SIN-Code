# OpenSIN-Code Ecosystem Inventory

> Single source of truth for every repo developed FOR SIN-Code and its
> integration status. CI and agents read the table below — keep it in sync
> with `requirements-ecosystem.txt` and `internal/mcpclient/registry.go`.

## Core

| Repo | Role | Integration | Status |
|---|---|---|---|
| SIN-Code | Main agent CLI/TUI/WebUI | — | ACTIVE |
| SIN-Code-WebUI-v2 | Official web frontend | `sin-code webui` (see docs/WEBUI.md) | ACTIVE |
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
| SIN-Code-Symfony-Lens | `symfonylens__*` | MCP server (registry.go) | ACTIVE |

## MCP Skill Servers (registry.go defaults)

| Repo | Server name / tool prefix | Default policy | Status |
|---|---|---|---|
| SIN-Code-Websearch-Skill | `websearch__*` | allow | ACTIVE |
| SIN-Code-Context-Bridge-Skill | `contextbridge__*` | allow | ACTIVE |
| Simone-MCP | `simone__*` | allow | ACTIVE |
| SIN-Code-Scheduler-Skill | `scheduler__*` | ask | ACTIVE |
| SIN-Code-Goal-Mode-Skill | `goalmode__*` | ask | ACTIVE |
| SIN-Code-Grill-Me-Skill | `grillme__*` | ask | ACTIVE |
| SIN-Code-Marketplace-Skill | `marketplace__*` | ask | ACTIVE |
| SIN-Code-Doc-Coauthoring-Skill | `codocs__*` | ask | ACTIVE |
| SIN-Code-Honcho-Rollback-Skill | `honcho__*` | ask (destructive) | ACTIVE |
| SIN-Code-Frontend-Design-Skill | `frontend__*` | ask | ACTIVE |
| SIN-Code-MCP-Server-Builder-Skill | `mcpbuilder__*` | ask | ACTIVE |
| SIN-Browser-Tools | `browser__*` (106 tools) | ask | ACTIVE |

## LLM Backends

| Repo | Integration | Status |
|---|---|---|
| coder-SIN-Qwen | Agent profile `qwen-relay` (profiles/qwen-relay.toml) | ACTIVE |
| SIN-Code-FireworksAI-OpenCode-Config | Agent profile `fireworks` (profiles/fireworks.toml) | MIGRATED |

## Deprecated / Archived

| Repo | Superseded by | Action |
|---|---|---|
| SIN-Code-Slash-Skill | `internal/commands` (C8, in-tree since v3.2.0) | ARCHIVE |
| SIN-Code-Security-Bundle | in-tree SAST/SBOM/SCA/Secrets tools | ARCHIVE |
| SIN-Code-Bundle-Web | SIN-Code-WebUI-v2 | ARCHIVED |

## Sync rules

1. Every new repo in the org MUST be added here in the same PR that creates it.
2. Every MCP skill server MUST have an entry in
   `cmd/sin-code/internal/mcpclient/registry.go` and a policy line in
   `cmd/sin-code/internal/permission_defaults.go`.
3. `requirements-ecosystem.txt` lists pinned versions for everything marked ACTIVE.
4. Every backend migration (e.g. SIN-Code-FireworksAI-OpenCode-Config →
   `profiles/fireworks.toml`) MUST leave a deprecation note in the source
   repo's README before archiving.
