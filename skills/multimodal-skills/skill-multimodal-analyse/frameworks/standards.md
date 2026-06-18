# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Read-Only Invariant (M4)

`analyse__*` tools are registered with `allow` permission in
`cmd/sin-code/internal/permission_defaults.go:64-66`. They MUST NOT mutate
the input file. Any tool that writes through to the file system MUST be
rejected by the permission engine with `deny`.

## One Tool Per File

Never call two different `analyse__*` tools on the same file. Pick the
right modality from context/triggers.md. If the file truly is hybrid
(PDF with embedded images), call `analyse__pdf_parse` first and let it
return embedded image refs; treat those as a follow-up against
`analyse__image_extract` only if the user asks.

## Output Is Canonical

The structured output of an `analyse__*` call is the source of truth. If
the prose answer disagrees with the tool output, the prose is wrong; fix
the answer, not the tool.

## Tool Coverage Gate (issue #248)

When this skill is active, the `analyse__*` tools are added to
`agentloop.Loop.CoverageRequiredTools`. The runtime `ToolCoverageEnforcer`
will reject the completion if the model never invoked at least one of
the six tools. This is the same fail-closed gate that enforces `sin_*`
tool usage for other skills — see `internal/agentloop/coverage.go`.

## Markdown Citation Format

When citing tool output in the final answer, use:

```
[analyse__pdf_parse /path/to/file.pdf, page 3 of 12]
```

so the operator can trace any claim back to its evidence.
