# dispatch/command.go — slash command resolution

`ResolveCommand` is the bridge from `/name args` to an executable
prompt. The flow:

1. Strip leading `/` from the name.
2. Look up the asset in the registry by `(KindCommand, name)`.
3. Parse the raw args into `Args` (positional + flags).
4. Run `Args.Substitute` against the asset's body.
5. Return a `ResolvedCommand` with the prompt + tool whitelist.

`ParseSlash` does the *outer* split — `/name rest of args` → name +
rawArgs — and reports whether the line was a slash command at all.

## Why `AllowedTools` is exposed

A command can declare `allowed-tools:` in its frontmatter to
restrict the toolset the agent may use while executing it (e.g.
`/review` allows only `Read`, `Grep`, `Bash`). The agent loop
should apply that restriction before submitting the resolved prompt.

## Related files

- `args.go` — `ParseArgs`, `Substitute`
- `agent.go` — the parallel flow for agent assets
- `dispatcher.go` — the consumer
