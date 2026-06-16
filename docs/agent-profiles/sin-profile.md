<!-- sin-code:profile v1 -->
# SIN-Code Project Profile

> Single source of truth for the per-agent rules SIN-Code installs into
> every supported host agent (Claude Code, Codex, opencode, Gemini, Cursor,
> Windsurf, Cline, GitHub Copilot). Edit **only** this file — the
> `sin-code profile render` step (issue #175) syncs every per-agent
> mirror off these bytes.

## Identity

- Product: `sin-code` (single static Go binary, `CGO_ENABLED=0`).
- Companion: `sin` Python CLI (`sin serve`, `sin chat`) — same rules.
- Repo: `github.com/OpenSIN-Code/SIN-Code`. 44+ MCP tools. 34 bundled skills.

## Hard mandates (NEVER violate)

- **M1** CI runs **only** via the n8n delegator. No GitHub Actions
  runners for build/test/lint.
- **M2** Single static binary. No runtime deps beyond the binary.
- **M3** **Verification gate is sacred.** Never report "done" unless the
  PoC/Oracle check passed.
- **M4** Permission engine (allow/ask/deny) gates every destructive tool.
  In headless mode `ask` → `deny` unless `--yolo`.
- **M5** Module path is `github.com/OpenSIN-Code/SIN-Code`. Never
  `SIN-Code-Bundle` (history only).
- **M6** Prefer SIN semantic tools over naive built-ins (`sin_edit` over
  string replace, SCKG over blind reads).
- **M7** Race-free concurrency. New goroutine code must pass
  `go test -race -count=1` before merge.

## Working style

- Read the file before editing it. Match the existing style.
- Smallest change that satisfies the issue. No drive-by refactors.
- Test what you break; new loop code targets ≥ 80% coverage.
- Conventional Commits (`feat:`, `fix:`, `docs:`, `feat!:` for breaking).

## Subagent contracts

- `sin_poc(code, spec)` — proof of correctness; mandatory for M3.
- `sin_oracle(claim, evidence)` — independent re-verification.
- `sin_adw(strict=true)` — flag god modules, circular deps, high coupling.

## Per-agent notes

- **Claude Code / Codex / opencode** — read `AGENTS.md` first, then this.
- **Gemini** — single-context; ambient rules = hard mandates only.
- **Cursor / Windsurf / Cline** — single `.mdc`/`.md` rule, full body
  inlined verbatim. Always-on.
- **GitHub Copilot** — shared `.github/copilot-instructions.md`; this
  body is appended inside a marker-fenced block so rerenders stay
  idempotent.

## Auto-clarity

If the next action is destructive, security-relevant, or order-sensitive,
drop terse mode and write normal prose around the action. The Verify
Gate (M3) is sacred — never compress the report of a verification step.
