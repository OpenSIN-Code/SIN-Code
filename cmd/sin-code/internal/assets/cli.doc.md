# assets/cli.go — `sin assets ...`

| Subcommand | Purpose |
|---|---|
| `assets list [kind]` | list loaded assets (kind = agent\|command\|skill, default all) |
| `assets validate` | run schema validation; exit non-zero on errors |
| `assets show [kind] [name]` | print one asset's prompt body |
| `assets import` | harvest skills from a vendored source repo with attribution |

## Why the `--base` flag is persistent

Most subcommands read from the same asset base. A persistent flag
removes the need to repeat it on every invocation. `import` ignores
it — it has its own `--source` and `--dest`.

## Related files

- `loader.go` — `LoadStandardLayout(base)` powers `list`/`validate`/`show`
- `importer.go` — `ImportSkills(opts)` powers `import`
- `selector.go` — used by the orchestrator, not the CLI directly
