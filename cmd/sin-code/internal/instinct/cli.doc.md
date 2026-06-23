# instinct/cli.go — `sin instinct ...`

Thin cobra command tree. All heavy lifting lives in `Manager`. The
command tree is wired into `cmd/sin-code/main.go` exactly once.

## Subcommands

| Command | Purpose |
|---|---|
| `status` | show effective instincts for current project |
| `projects` | list known projects with counts |
| `evolve [--apply]` | cluster eligible instincts into proposals |
| `promote [--apply]` | move cross-project instincts to global |
| `prune [--ttl-days N]` | delete stale pending, decay rest |
| `export [-o file]` | write all instincts as one YAML stream |
| `import [file]` | import from an exported stream |
| `show [id]` | print one instinct (effective scope) |
| `forget [id] [--global]` | delete one instinct |
| `history [--limit N]` | show audit.jsonl tail |

## Why so thin

The CLI is the least-tested surface. All state lives in `Manager`
and `Store`, both of which have unit tests. The CLI is a 1:1
forwarder — if a behavior changes here, the change is in the
underlying function, not in the command.
