# PLAN.md — Ecosystem Skills Full Activation Phase

## Phase Goal
Make all 16 ecosystem skills both installed and runnable, with a `sin-code skill doctor` command and robust `sin-code skill install all`.

## Tasks

### Subagent 1: PATH-Binary Skills Activation
Make the 8 PATH-binary skills (browser, codocs, frontend, goalmode, marketplace, mcpbuilder, scheduler) recognized as installed and runnable.

- Update `cmd/sin-code/internal/skillmgr/manager.go` to treat a PATH binary as an installed skill.
- Update `cmd/sin-code/internal/mcpclient/registry.go` to accept PATH-based skills without requiring a cloned repo.
- Add tests.
- Verify `sin-code skill status` shows these as installed.

### Subagent 2: Python Module Skills Fix
Fix the Python-module skills (analyse, contextbridge, grillme) that are not runnable.

- Inspect each skill repo's structure (if available on PATH or in `~/.local/share/sin-code/skills/`).
- Find the correct entrypoint (e.g., `mcp_server.py`, `-m` module).
- Update registry/manager entrypoint resolution.
- Add tests.
- Verify `sin-code skill status` shows these as runnable.

### Subagent 3: External/Server Skills (Honcho, Simone, SymfonyLens)
Fix or gracefully handle skills that depend on external servers.

- **Honcho**: requires Honcho server. Add a health check and skip-with-warning if server unreachable.
- **Simone**: Node.js MCP server. Ensure `node` command and path are correct.
- **SymfonyLens**: Python module `symfony_lens.server`. Ensure module path resolution.
- Update registry/manager.
- Add tests.

### Subagent 4: Shop Skills Cleanup
Handle the 3 shop skills (cj-dropshipping, stripe, tiktok) that are currently not runnable.

- Determine if these skills are deprecated or need special setup.
- Either fix them or add a `deprecated` flag to the skill registry and exclude them from default `install all`.
- Update `sin-code skill status` to show deprecation.
- Add tests.

### Subagent 5: `sin-code skill install all` and `skill doctor`
- Add `sin-code skill install all` (without requiring confirmation per skill).
- Add `sin-code skill doctor` that checks each skill and reports why it's not runnable.
- Update `scripts/install-sin-skills.sh` if needed.
- Update docs and tests.

### Subagent 6: Verification & CEO Audit
- Run full verification suite.
- Ensure `go build ./...`, `go test -race ./...`, `golangci-lint` all pass.
- Ensure `sin-code ceo-audit ./` remains A+.
- Update `AGENTS.md` and `CHANGELOG.md` if behavior changed.

## Verification Criteria
- [ ] `sin-code skill status` shows ≥14 skills as runnable (all except deprecated/externally-dependent ones).
- [ ] `go build ./...` passes
- [ ] `go test -race -count=1 ./...` passes
- [ ] `golangci-lint run ./...` clean
- [ ] `sin-code ceo-audit ./` A+ / 48 gates

## Risks
- External skills (Honcho, Simone) may not work without external services.
- Shop skills may be deprecated or unmaintained.
- Skill repos may not be cloned locally.
- Multiple subagents may touch the same registry/manager files.

## Mitigations
- Orchestrator reconciles overlapping changes.
- Graceful degradation for external dependencies.
- Mark deprecated skills explicitly rather than hiding them.
- No commits from subagents.
