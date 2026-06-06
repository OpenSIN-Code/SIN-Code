# commands.doc.md

Command catalog for the `sin tui` binary.

## What this file does

Defines the `Command` struct and the `Commands` slice — the full menu of `sin`
subcommands exposed in the TUI. Each entry maps 1:1 to a `sin <subcommand>`
CLI invocation so the TUI is a discoverable wrapper, not a parallel interface.

## Dependencies

- `app.go` — renders the list and calls `startCommand`
- `runner.go` — executes the built shell string
- `theme/styles.go` — Danger flag triggers red styling in the delegate

## Important values & limits

- `MaxGroups` — display order of groups in the menu
- `Filter(query)` — substring match on Key + Title + Description
- `ByGroup()` — groups commands preserving catalog order

## Design decisions

- Catalog is a package-level `var` so tests can patch it if needed.
- `Danger` flag is separate from Group so any command can be marked
  destructive without moving it to a different section.
- `Args` is a template string shown as the prompt placeholder; if empty
  the command runs immediately without prompting.

## Usage

```go
for _, c := range Commands {
    fmt.Println(c.Key, c.Title)
}
```

## Caveats

- Adding a new `sin` subcommand requires a matching entry here or it
  won't appear in the TUI.
