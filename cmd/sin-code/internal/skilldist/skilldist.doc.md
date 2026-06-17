# `cmd/sin-code/internal/skilldist` — marker-fenced skill distribution

Issue: **#169** (multi-agent Skill distribution via `--agent` flag).
Schema version: **v1.0.0** (first stable cut).

This CoDoc is the rg-friendly reference for the `skilldist` package. It is
intentionally short: the package doc comment in `skilldist.go` carries the
authoritative spec, and this file is the catalog of "what lives where".

```
internal/skilldist/
├── skilldist.go        — package API (Target struct, Targets map, Install,
│                         Uninstall, IsInstalled, ParseMarkers, RenderBlock,
│                         StripFrontmatter, Resolve, BeginMarker, EndMarker)
└── skilldist_test.go   — race-safe unit tests + table-driven registry tests
```

## One-page API surface

| Symbol | Kind | Stability | Notes |
|---|---|---|---|
| `Target` | struct | public | (Name, DisplayName, InstallPath, Format) |
| `Targets` | map | public | Single source of truth — must stay <=8 entries this PR. |
| `TargetNames()` | sort helper | public | Alphabetical; for stable log lines. |
| `InstallOptions` | struct | public | `{SrcRoot, Home, Body}` — see Install. |
| `Install(skill, target, opts) error` | writer | public | Idempotent marker-fence write. |
| `Uninstall(target, skill, home) error` | writer | public | Reversible marker-fence delete. |
| `IsInstalled(target, skill, home) (bool, error)` | probe | public | Format-aware existence check. |
| `Resolve(target, skill, home) (path, error)` | path-resolver | public | Expand `<skill>` and join to home. |
| `ParseMarkers(content, skill) ParseResult` | scanner | public | Line-based fence locator. |
| `RenderBlock(skill, body) string` | renderer | public | Begin/body/End fenced block. |
| `StripFrontmatter(raw) string` | helper | public | YAML frontmatter trim. |
| `BeginMarker(name) string` | constant-formatter | public | ASCII: `<!-- SIN-CODE-SKILL-START: <name> -->` |
| `EndMarker(name) string` | constant-formatter | public | ASCII: `<!-- SIN-CODE-SKILL-END:   <name> -->` |
| `FormatDir / FormatRule / FormatMarker` | constants | public | Writer dispatch. |
| `MarkerPrefix = "SIN-CODE-SKILL"` | constant | public | Visible in `head -n1` output. |

## Marker-fence contract

Every block written by `Install` for `FormatRule` or `FormatMarker` MUST
begin with `<!-- SIN-CODE-SKILL-START: <skill> -->` and end with
`<!-- SIN-CODE-SKILL-END:   <skill> -->` (note the trailing spaces before
`<skill>` on the end marker — visual alignment in `head -n 2`).

`ParseMarkers` strips that whitespace during lookup, so a hand-rolled
editor outside this package that produces `<!-- SIN-CODE-SKILL-END: x -->`
without the padding still parses correctly.

## Format semantics

| Format | Path shape | Writer |
|---|---|---|
| `dir` | `<home>/<InstallPath-with-<skill>>/` (directory) | copies SKILL.md + optional context/frameworks/tasks/templates |
| `rule` | `<home>/<InstallPath-with-<skill>>` (single file, .md or .mdc) | writes RenderBlock; replaces prior block for same skill |
| `marker` | `<home>/<InstallPath>` (single shared file, no `<skill>` placeholder) | appends RenderBlock; preserves other skills' blocks |

## Registered targets (11, alphabetical)

Format `dir`: `claude-code`, `gemini`, `opencode`.
Format `rule`: `aider`, `cline`, `codex`, `continue`, `cursor`, `windsurf`, `zed`.
Format `marker`: `copilot`.

The `sin-code skill install <name> --agent all` command runs through every
target in `TargetNames()` order — determinism is required so the verify-gate
can diff log output across machines.

## Test layout

`skilldist_test.go` is one file with three families of tests:

1. **Registry** — `TestTargets_AllPresent`, `TestTargets_FieldsValidates`.
   Lock the set and schema; alarm if a future contributor adds a target
   without updating AGENTS.md / CHANGELOG / CLI completion.
2. **Marker parser** — round-trip, distinct skills, half-opened fence,
   CRLF tolerance, multi-block preservation. These tests are the safety
   net for the `FormatMarker` case where a regression would tear another
   skill's block.
3. **Install / Uninstall round trip** — per-target idempotent re-install,
   marker-file uninstall that preserves other blocks, FormatMarker empty-
   file cleanup.

## Why this exists

`sin-code skill` had two responsibilities before this PR:

- v3.5.0 (`skillmgr`): clones upstream skill **repos** (websearch, scheduler,
  …) and verifies their MCP entrypoint.
- v3.6.0 (`skills`): lists + installs bundled SKILL.md files via skillsmith
  into `~/.claude/skills/` or `~/.agents/skills/`.

Neither path shipped a `--agent` flag. Issue #169 fills the gap: take a bundled
Skill and route it to one of 11 agent families with a marker-fenced block so
the install is idempotent across re-runs. The marker-fence guarantees no
duplicate blocks grow on the disk even after dozens of updates.

## Known limitations

- The marker fence is line-based; a `<!-- SIN-CODE-SKILL-START: x -->`
  embedded mid-line inside a code block would mislead `ParseMarkers`. The
  host agents we ship to don't emit such lines, but a third-party editor
  could. `TestParseMarkers_HalfOpenedFence` documents the safety net.
- `Install` does not validate that `home` exists; it `MkdirAll`s parents as
  needed. This is intentional for the canonical use case (the home dir is
  always present) but means `sin-code skill uninstall --agent x y` creates
  an empty parent directory if the path was never written.
- `SIN_CODE_HOME` env override is the only override; `~/.claude/skills` is
  hardcoded relative to it. Test scaffolding passes `t.TempDir()` to
  `InstallOptions.Home` to bypass the env var.

## Change-log

- **v1.0.0 / 2026-06-16** — initial implementation (issue #169). 8 targets,
  3 formats, 1 marker convention. Race-safe in every code path.
- **v1.1.0 / 2026-06-17** — expanded to 11 targets: `aider`, `continue`, `zed`
  added (rule format). 3 formats unchanged.
