# assets/asset.go — data model

`Asset` is a Markdown file with YAML frontmatter and a body. The
frontmatter is the *schema*; the body is the *prompt*. Three kinds:

- `agent` — subagent with its own model + tools
- `command` — slash command (one-shot prompt)
- `skill` — reusable knowledge asset (loaded into context)

## Field superset

The struct is a **superset** of all three schemas. Per-kind fields
(`Model` for agents, `Argument` for commands) are simply empty when
the asset is not of that kind.

## Why a `Body` field

The body is *not* metadata. It is the actual prompt the agent will
receive. Splitting it from frontmatter lets us:

- Pass `Body` to a subagent as its system prompt
- Re-render the asset (`Render()`) without losing the body
- Diff bodies across versions

## Related files

- `loader.go` — `LoadDir` produces `[]*Asset`
- `validate.go` — `Validate` enforces per-kind schema
- `registry.go` — index by `(kind, name)`
