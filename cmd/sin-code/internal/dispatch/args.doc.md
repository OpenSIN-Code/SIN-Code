# dispatch/args.go — argument parsing + placeholder substitution

The arguments a slash command receives, plus the substitution rules
that turn a command body with placeholders into a final prompt.

## Placeholders (ECC-compatible)

| Placeholder | Replaced by |
|---|---|
| `$ARGUMENTS` | the entire raw argument string |
| `$1`..`$9` | the i-th positional argument (empty if absent) |
| `$@` | all positional args, space-separated |
| `${flag}` | flag value, e.g. `--strict` makes `${strict}` → `true` |

## Tokenizer

Quote-aware: `"a b"` is a single positional arg. Flags are detected
by `--` prefix; `--key=value` and `--key value` are both accepted.

## Substitution order

`$ARGUMENTS` and `$@` first, then `$9` down to `$1` (descending so
`$10` would not be eaten as `$1`), then `${flag}`. Each pass is
plain `strings.ReplaceAll` — no template engine.

## Related files

- `command.go` — calls `ParseArgs` + `Substitute` to resolve a
  slash command
- `dispatcher.go` — the consumer
