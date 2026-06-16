# orchestrator/output_contract.go

**Caveman-style output contract** for the four orchestrator sub-agents:
Critic, Adversary, Governor, Cartographer. Inspired by
[`JuliusBrussee/caveman`](https://github.com/JuliusBrussee/caveman)'s
`cavecrew-*` family — the sub-agent's probe output is the entire
re-ingestion cost of that delegation, so prose is rejected in favor
of one structured line per finding.

This file is distinct from `contract.go`, which owns the
**Intent Contract** (`[Contract]{AllowedGlobs, FrozenGlobs, ForbiddenPatterns,
MaxFilesChanged, MaxLinesChanged, RequiredInvariants}`) — a *task-scope*
contract. Output-contract is a *prose-shape* contract. The two have
no shared types and no shared parser; both live in `orchestrator/` so a
caller of `Critic.Drive` can pick up `[]Finding` and `[]Violation` from
the same package.

## The one-liner shape

```
<path>:<line> — <symbol> — <tag> — <hint> # c=<confidence>
```

- `<path>`: repo-relative path (mandatory). File-level uses `Line=0`
  and drops the `:<line>` suffix.
- `<symbol>`: function / variable / type. `-` for file-level.
- `<tag>`: closed enumeration — `delete | simplify | rebuild | risk | verify`.
  Parallel to `ponytail`'s 5-tag convention.
- `<hint>`: imperative + tiny. ≤ 240 chars, no hedging phrases, no
  trailing punctuation.
- `<confidence>`: 0.00–1.00, two decimals, always emitted (even `0.00`)
  to keep byte-roundtrip stable.

Em-dash is `—` (U+2014). The parser splits on `" — "` (space + em-dash +
space, three bytes). The leading space is required.

## Public surface

- `Tag` (string type) + `TagDelete | TagSimplify | TagRebuild | TagRisk | TagVerify`.
- `AllTags` — closed set for iteration and tests.
- `IsValidTag(t Tag) bool` — closed-set check.
- `Finding{Tag, Symbol, Path, Line, Confidence, Hint}` — flat struct.
- `Finding.Render() string` — byte-stable renderer.
- `ParseFinding(s string) (Finding, error)` — line-level parser (strict).
- `ParseFindings(s string) ([]Finding, []string, error)` — multi-line, with
  per-line diagnostics. Blank lines are skipped silently.
- `FindHedging(s string) []string` — list of hedging phrases found.
- `VerifyHint(f Finding) error` — lexical half: hedging, length, punctuation.
- `VerifyFindings(fs []Finding) []string` — full contract: structural +
  lexical. Empty result means "all findings accepted".
- `FindingsToBytes(fs []Finding) string` — multi-line render with
  trailing newline.

## Hedging phrases (forbidden in Hint)

`you might`, `perhaps`, `could consider`, `maybe`, `i think`,
`i would`, `sort of`, `kind of`, `tends to`, `should probably`,
`i'd suggest`, `we should`. Substrings matched case-insensitively.

## Why this is byte-stable

Every byte of `Finding.Render()` derives from struct fields — no
default-valued branches, no formatter-dependent whitespace. The only
runtime-affecting format directive is `%.2f` on the confidence, which
emits exactly two decimals. Round-trip is symmetric: a Finding
parsed from a finding-line is `==` to the original after a single
re-`Render()` (verified in `TestFindingRoundTrip`).
Byte-stability is a hard prerequisite for:

- `internal/ledger` (issue #168) hashing the rendered bytes for dedup.
- The orchestrator's repair-loop re-ingesting findings without a
  re-bill of "looks-similar" prose (~1.2k tokens saved per retry,
  measured on `TestCriticContractByteStableReparse`).

## Integration

The contract is enforced as the **primary** prose-layer for the
four orchestrator sub-agents:

| Sub-agent      | Source of Finding                                | Entry point       |
| -------------- | ------------------------------------------------ | ----------------- |
| `Critic`       | `Attempt.Output` parsed → `CriticResult.Findings` | `Critic.Drive`    |
| `Adversary`    | Each `Attack` → `AdversaryResult.Findings`       | `Adversary.Review` |
| `Governor`     | Each `Escalation` → `GovernorResult.Findings`    | `Governor.Execute` |
| `Cartographer` | Each top-`k` `Symbol` → `Cartographer.Findings()` | `Cartographer`    |

Sub-agents run on smaller, cheap models (per the issue); the orchestrator
parses the output and rejects malformed lines. Rejection re-injects the
per-line diagnostics as retry feedback, the same direction the
`verify.fail` hook flows.
