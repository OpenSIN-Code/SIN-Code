# Backlog

> Active work queue for SIN-Code. Items are roughly prioritized; references to GitHub issues use `#NN`. Items saved in the local `coverage/save-local` branch are marked accordingly.

## In Progress / Near-term

- **Fix golden help test** — `cmd/sin-code/testdata/golden/help.golden` is out of sync with the current subcommand list (new subcommands from the `coverage/save-local` work: `instinct`, `hooks`, `assets`, `evalset`, `prp`).
- **Fix coverage branch test failures** — `coverage/save-local` has missing test hooks (`mcpHookVars`, `autoHookVars`) causing `go test ./cmd/sin-code` to fail. Resolve and merge back to `main` when green.
- **Skill lifecycle markers** (#139) — add `lifecycle: native|external|deprecated` and `sources:` metadata to every bundled skill.
- **Fusion: goal-mode into native `sin-code goal`** (#140) — port `SIN-Code-Goal-Mode-Skill` functionality into the native `sin-code goal` subcommand.
- **Fusion: grill-me into native `sin-code grill`** (#141) — port `SIN-Code-Grill-Me-Skill` to a native subcommand.
- **Integrate shop-skills** (#142) — plan fusion of `SIN-Shop-Center` skills (CJ Dropshipping, Stripe, TikTok Shop) into the binary or keep as external docs.

## Technical Debt

- **LSP framing bug** — `internal/lsp/client.go` cannot handle interleaved JSON-RPC notifications from gopls v0.20+; rewrite `Call` with a scanner split function (see `docs/lsp-known-issues.md`).
- **Auto-migration of pre-#265 per-workspace `.sin-code/` directories** — the migration in #265 / #62 changed the runtime home to `os.UserConfigDir()/sin-code/workspaces/<ws-hash>/` but did not move existing per-workspace `.sin-code/{lessons,sessions}.db` files. A `sin-code sessions migrate` subcommand (or first-run prompt) is out of scope for the original issue.
- **AGENTS.md / README drift** — keep these files in sync with the actual binary after every subcommand/tool change.
- **Branch protection note** — `main` now requires `CEO Audit (QUICK, grade≥B)` status check and `required_approving_review_count: 0` for the solo-maintainer workflow.

## Completed (latest)

- ✅ Migrate TUI runtime DBs (`lessons.db`, `sessions.db`) to `os.UserConfigDir()` with workspace-hash isolation (#265 / #62).

## Docs & Process

- Update `README.md` and `AGENTS.md` whenever a new subcommand or bundled skill lands.
- Update `CHANGELOG.md` with every user-facing change in the same PR.
- Keep `ECOSYSTEM.md` in sync with `registry.go` and `permission_defaults.go` (CI `ecosystem-sync.yml` enforces this).

## Completed (latest)

- ✅ Reorganize all bundled skills into category directories and rename to `skill-<category>-<name>`.
- ✅ Add `github-skills/` category with 4 GitHub-focused skills.
- ✅ Merge PR #138 into `main` and relax branch protection for solo workflow.
- ✅ Save local WIP coverage/docs work to `coverage/save-local` branch.
- ✅ Add global AGENTS.md rule banning destructive git commands (`git checkout`, `git reset`, etc.) without explicit permission.
