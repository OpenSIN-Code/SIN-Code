# `internal/profile` — single-source-of-truth per-agent project profile (issue #175)

## What this package is

`internal/profile` renders a single in-repo markdown
(`docs/agent-profiles/sin-profile.md`) into the per-agent mirror
files that SIN-Code installs into every supported host agent family:

| Agent family | Output path (relative to repo root) | Format       |
| ------------ | ------------------------------------ | ------------ |
| Claude Code  | `.claude/skills/sin-code/SKILL.md`    | `dir`        |
| opencode     | `.config/opencode/skills/sin-code/SKILL.md` | `dir`  |
| Gemini CLI   | `.gemini/skills/sin-code/SKILL.md`    | `dir`        |
| Codex CLI    | `.codex/rules/sin-code.md`            | `rule`       |
| Cursor       | `.cursor/rules/sin-code.mdc`          | `rule`       |
| Windsurf     | `.windsurf/rules/sin-code.md`         | `rule`       |
| Cline        | `.clinerules/sin-code.md`             | `rule`       |
| GitHub Copilot | `.github/copilot-instructions.md`   | `marker`     |

The table is the **single source of truth** in this package. Adding a
target is non-breaking; renaming or removing one is a major bump per
AGENTS.md §10. The set is intentionally a mirror of the skilldist
table so the two packages can be merged later without changing
public output.

## Why byte-stable

`Render(tgt, body)` is pure: the bytes depend only on (target
struct, source body). The CLI's `sin-code profile verify` reads
every on-disk mirror, recomputes the expected render, and refuses
to merge if any mirror is missing or drift. This is the same shape
as the verify-gate in `internal/verify` (mandate M3) — the
renderer is a deterministic preprocessor.

## Marker-fence covenant (issue #169)

Every `rule` / `marker` output is bracketed by a
`<!-- SIN-CODE-SKILL-START/END -->` fence with the same exact
ASCII bytes as `internal/skilldist`. A downstream parser that
scans for one kind also finds the other; a `profile render` call
is idempotent (rerun with unchanged source = byte-identical
output). See `parser.go` for the contract.

## Public surface

```go
type Target struct { Name, DisplayName, InstallPath, Format string }
var Targets map[string]Target
func TargetNames() []string
func MustTarget(name string) Target

func RenderAll(body string) (map[string]string, []string, error)
func Render(tgt Target, body string) (string, error)
func Resolve(tgt Target, base string) (string, error)
func StripFrontmatter(raw string) string

func HashSource(tgt Target, body string) (string, error)
func Verify(base, body string) ([]Result, error)

func LoadSource(base string) (string, error)
func WriteAll(base, body string) ([]string, error)
func WriteSelected(base, body, name string) ([]string, error)
func ListTable() []ListEntry
```

## CLI surface

```
sin-code profile show                  # print current source
sin-code profile list                  # print target table
sin-code profile render <target|all>   # write one or all mirrors
sin-code profile render --dry-run      # preview without writing
sin-code profile verify                # CI gate
sin-code profile verify --json         # machine-readable drift
```

## Tests

`profile_test.go` pins:

- Exact SHA-256 of `Render(tgt, fixture)` for one target per format.
- Marker-fence idempotency (rerun = byte-identical output).
- `ParseMarkers` open / close / half-opened-fence roundtrip.
- `Verify` missing-file → DriftError.
- `Verify` drift → DriftError.
- `Verify` pass → nil error.

A new golden test fails the moment the renderer output drifts.
