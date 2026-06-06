# app.doc.md

Top-level Bubbletea model for the `sin tui` binary.

## What this file does

Implements `Model` — the finite-state machine driving the TUI. Two-pane
layout: searchable command menu on the left, live preview / streaming
output on the right. The TUI is a thin shell-out wrapper; every command
runs `sin <subcommand> [args]` via `runner.go` and streams the output
back into a Bubbles `viewport`.

## States

| State          | What's on screen                                  | How you leave           |
|----------------|---------------------------------------------------|-------------------------|
| `StateMenu`    | command list + preview pane                       | `/` (search), `enter` (run), `q`/`ctrl+c` (quit) |
| `StateSearch`  | focused search input filtering the list           | `esc` (back), `enter` (run highlighted) |
| `StatePrompt`  | argument prompt for the selected command          | `esc` (cancel), `enter` (run with value) |
| `StateRunning` | spinner + streaming subprocess output             | runs to completion → `StateOutput` |
| `StateOutput`  | finished output + return-code badge               | `esc` (menu), `y` (copy), `q` (quit) |

## Polish features

| Key      | When                  | Action                                            |
|----------|-----------------------|---------------------------------------------------|
| `?`      | menu / output         | toggle the centered Help modal                    |
| `t`      | menu / output         | cycle theme (dark ↔ light); persisted to config   |
| `↑` `↓`  | search                | recall prior search terms (last 20, in-memory)    |
| `y`      | output                | `pbcopy` the visible output, flash "Copied!" 1s   |

## Dependencies (imports)

- `charmbracelet/bubbletea` — Elm-style event loop
- `charmbracelet/bubbles/{list,textinput,spinner,viewport}` — components
- `charmbracelet/lipgloss` — styling
- `internal/tui/theme` — palettes + styles
- `internal/tui/{commands,runner,history,copy,config}` — peer modules

## Files this module touches

- `commands.go` — the `Command` catalog rendered in the menu
- `runner.go` — subprocess streaming, fires `runFinishedMsg`
- `history.go` — search history ring buffer (`m.history`)
- `copy.go` — clipboard write via `pbcopy`
- `config.go` — theme persistence (`LoadConfig` / `SaveConfig`)
- `theme/` — palette + style tokens

## Design decisions

- **`tea.KeyMsg` short-circuits to `handleKey`** — historically that path
  returned `m, nil` for unhandled keys, swallowing typed characters. The
  polish pass routes anything not explicitly handled in a given state to
  the focused subcomponent (textinput in search/prompt, list in menu),
  fixing arrow-key navigation and typed filtering as a side effect.
- **Help modal replaces the frame entirely** while open — simpler than
  overlaying with a backdrop and clearer to the user that this is a
  hard-stop dialog.
- **Toast uses a token guard** (`toastToken`) — a rapid second toast
  doesn't get cleared early by the first one's tick.
- **Theme switch rebuilds the list delegate via `buildDelegate`** —
  bubbles' default delegate caches style copies, so a second
  `SetDelegate` is required after the styles object changes.
- **Persist failure is swallowed in `cycleTheme`** — a read-only
  `~/.config` shouldn't break the in-session cycle.

## Magic constants

```go
const toastDuration = 1 * time.Second  // see also: MaxHistoryItems (history.go)
```

## Example: keybinding loop

```
user presses '/' in StateMenu
→ handleKey: state = StateSearch, search.Focus()
user types "scout"
→ handleKey StateSearch default branch → search.Update → applyFilter
user presses 'enter'
→ handleKey: history.Push("scout"); startCommand → tea.Batch(spinner, runCommand)
runner finishes
→ Update receives runFinishedMsg → state = StateOutput; refreshOutput()
user presses 'y'
→ handleKey StateOutput: copyOutput → CopyToClipboard + flashToast("Copied!")
```

## Caveats

- Theme cycle re-creates the styles object; any external code holding a
  pointer to the old `*theme.Styles` will keep the stale colors. The
  built-in subcomponents (list, textinput, viewport, spinner) are
  refreshed by `applyTheme`; if you add new style-bearing widgets,
  remember to refresh them there too.
- `padRight` measures with `lipgloss.Width` (rune-aware) but assumes
  single-cell glyphs. For double-width CJK in help text, alignment may
  drift.
- The Help modal does **not** scroll; it's sized for ≤ 15 entries. If
  the binding list grows beyond a comfortable single page, wrap it in a
  viewport.
