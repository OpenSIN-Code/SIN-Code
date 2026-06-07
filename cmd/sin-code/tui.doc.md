# `tui.doc.md` — Interactive Bubbletea TUI Menu

Provides an interactive terminal UI for browsing and launching all sin-code subcommands.

## What it does

- **Lists all 16 subcommands** in a searchable, keyboard-navigable menu built with [Bubbletea](https://github.com/charmbracelet/bubbletea).
- **Supports theme switching** with 5 built-in color schemes (default, Dracula, Nord, Solarized, Monokai).
- **Handles argument input** for commands that require arguments (press `r`, type args, press Enter).
- **Falls back to plain text** when no TTY is detected (e.g., in CI or pipes), so scripts never crash.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `tuiCmd` into the root cobra command
- `cmd/sin-code/tui_test.go` — unit tests for model creation, theme cycling, and fallback logic
- `cmd/sin-code/main.go` — `getSubcommand()` looks up commands from `rootCmd.Commands()`

## Bubbletea architecture

- **Model:** `tuiModel` holds the `list.Model`, `delegate`, `themeIndex`, and an optional `textinput.Model` for arg entry.
- **Update loop:** Handles `tea.KeyMsg` for navigation, filtering (`/`), theme (`t`), arg input (`r`), help (`Enter`), and quit (`q`/`ctrl+c`).
- **View:** Renders the list with a dynamic hint bar at the bottom that changes based on whether the selected command supports running without args.

## Key bindings

| Key | Action |
|---|---|
| `↑` / `↓` or `k` / `j` | Navigate list |
| `/` | Filter / search |
| `Enter` | Show `--help` for selected command |
| `r` | Run selected command (prompts for args if needed) |
| `t` | Cycle to next theme |
| `q` or `Ctrl+C` | Quit |

## Commands that run without args

- `serve` — starts MCP server with defaults
- `orchestrate` — lists tasks by default
- `tui` — technically valid, but recursive

For all other commands, pressing `r` enters an inline text-input mode where you can type arguments (e.g., `--path . --format json`).

## Theme system

Themes are stored as accent-color hex codes in `themeColors` and human-readable names in `themeNames`. The active theme is applied to the list title, normal/selected items, and hint text via `lipgloss`.

| Index | Name | Color |
|---|---|---|
| 0 | default | `#7D56F4` (purple) |
| 1 | Dracula | `#FF79C6` (pink) |
| 2 | Nord | `#88C0D0` (blue) |
| 3 | Solarized | `#B58900` (yellow) |
| 4 | Monokai | `#A6E22E` (green) |

## Usage examples

```bash
# Launch the interactive TUI
sin-code tui

# In a CI pipeline, the same command prints a plain text catalog
sin-code tui | cat
```

## Known caveats / footguns

- **TTY required for interactive mode:** If `tea.NewProgram` returns an error (no TTY), the fallback plain text catalog is printed. This is intentional but means you lose the interactive benefits in non-interactive environments.
- **Argument input is basic:** The `r` prompt splits arguments on whitespace (`strings.Fields`). It does NOT handle quoted arguments with spaces. For complex commands, use the CLI directly instead of the TUI.
- **Recursive TUI:** Launching `tui` from inside the TUI (`r` → type `tui`) is technically allowed but probably not useful. No guard is implemented.
- **Theme is not persisted:** Switching themes in the TUI does not write to `~/.config/sin/tui.toml`. The theme resets to default on every launch.
- **Window size:** The list height is `window height - 2` to make room for the hint bar. Very small terminals (< 5 lines) may clip the list.
