# RESEARCH.md — Ecosystem Skills Full Activation Phase

## Context
- Repo: SIN-Code (`/Users/jeremy/dev/SIN-Code`)
- Current `main`: `0beaee5`
- CEO audit: A+ / Score 273 / 48 gates
- Open issue: #159 (WebUI-v2 dashboard, out of scope per AGENTS.md)

## Current Skill State
16 ecosystem skills tracked:

| Skill | Installed | Runnable | Detail |
|-------|-----------|----------|--------|
| analyse | false | false | |
| browser | false | true | PATH binary: sin-browser-mcp |
| codocs | false | true | PATH binary: sin-codocs |
| contextbridge | false | false | |
| frontend | false | true | PATH binary: sin-frontend-design |
| goalmode | false | true | PATH binary: sin-goal-mode |
| grillme | false | false | |
| honcho | false | false | |
| marketplace | false | true | PATH binary: sin-marketplace |
| mcpbuilder | false | true | PATH binary: sin-mcp-server-builder-mcp |
| scheduler | false | true | PATH binary: sin-scheduler-mcp |
| shop-cj-dropshipping | false | false | |
| shop-stripe | false | false | |
| shop-tiktok | false | false | |
| simone | false | false | |
| symfonylens | false | false | |
| websearch | true | true | installed binary |

## Observations
- 8 skills are runnable via PATH binaries but not marked installed.
- 7 skills are not runnable at all: analyse, contextbridge, grillme, honcho, shop-cj-dropshipping, shop-stripe, shop-tiktok, simone, symfonylens.
- The skill registry/manager was recently improved (nested mcp_server.py discovery, PATH fallback).

## Likely Root Causes
1. **PATH binaries are not trusted** unless the skill repo is cloned.
2. **Shop skills** may lack proper entrypoints or be deprecated.
3. **`honcho`** requires external server (Honcho).
4. **`simone`** is a Node.js MCP server.
5. **`symfonylens`** is a Python module (`symfony_lens.server`).
6. **`contextbridge`**, **`grillme`**, **`analyse`** are Python modules with specific entrypoints.

## Constraints
- Do not require external internet at install time.
- Skills are in `~/.local/share/sin-code/skills/` or on PATH.
- `CGO_ENABLED=0` for Go skills.
- CEO audit must stay A+.

## Goal
Get all 16 skills to `installed: true` and `runnable: true`.

## Next Steps
- Audit each skill's entrypoint.
- Fix registry/manager to recognize PATH binaries as installed.
- Fix or remove broken skills.
- Add `sin-code skill install all --yes` and `sin-code skill doctor`.
- Update tests and docs.
