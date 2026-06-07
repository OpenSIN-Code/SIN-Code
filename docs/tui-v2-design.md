# `sin-code tui` v2 — Design Document

> **Purpose:** Redesign `cmd/sin-code/tui.go` (currently a 264-line flat list picker) into a multi-pane, Bubbletea v2-native **command center** for the entire sin-code CLI tool suite. Modeled on the opencode TUI architecture (`anomalyco/opencode`), but with sin-code's own personality: a **cyber/hacker** aesthetic, **19 subcommand tabs**, an **EFM container dashboard**, and a **persistent footer** with token/cost placeholders ready for a future LLM integration.
>
> **Audience:** A second agent will use this document to rebuild `cmd/sin-code/tui.go` (and new sibling files) from scratch.
>
> **Status:** Design v0.1 — research complete, ready for implementation.

---

## 1. Research Summary

### 1.1 opencode TUI architecture (`anomalyco/opencode`)

The real opencode TUI (171k★, 13.9k commits, dev branch) is a **TypeScript SolidJS-style Solid-TUI** running in a `Bun.Worker` (see `packages/opencode/src/cli/cmd/tui.ts`). Key takeaways from the source:

| What | How |
|---|---|
| Entry point | `tui.ts` → spins up a `new Worker(file)` and an `Effect.runPromise(run(...))` to the actual TUI runtime |
| TUI root | `packages/opencode/src/cli/tui/` (subdir, not `cmd/tui/`) |
| Run arguments | `--model`, `--continue/-c`, `--session/-s`, `--fork`, `--prompt`, `--agent` |
| Agent switcher | `Tab` key cycles between `build` (default) and `plan` (read-only). `general` is a subagent invoked with `@general` |
| Transport | Internal `fetch` and `EventSource` are shimmed to the worker; external mode exposes `--port`/`--hostname`/`--mdns` |
| Messaging | `Rpc.client<typeof rpc>` between the parent process and the worker |
| Crash safety | `win32InstallCtrlCtrlCGuard()` for Windows; `SIGUSR2` triggers a `client.call("reload")` |
| Heap snapshot | `writeHeapSnapshot("tui.heapsnapshot")` available on demand |

> **Lesson for sin-code:** Opencode keeps the **CLI process thin** and runs the **interactive TUI in a sidecar worker** (or a separate goroutine in our case). Long-running streams don't block the CLI. We'll mirror this by using `tea.NewProgram` in a `goroutine` and capturing the eventual return value.

### 1.2 The screenshot the user showed

The screenshot is a multi-region TUI with these regions (from the user's description):

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  Tabs:  [..ing (node) #1] [..ool (node) #2] ...                              │
├────────────┬─────────────────────────────────────────────────────────────────┤
│            │                                                                 │
│  Sidebar   │  Chat / Tool call / Code block area                             │
│  (history) │                                                                 │
│            │                                                                 │
├────────────┴─────────────────────────────────────────────────────────────────┤
│  [ESC] Build • MiniMax M3 (SIN) Vercel AI Gateway Pool (SIN)   119.5K  $111  ctrl+p commands
└──────────────────────────────────────────────────────────────────────────────┘
```

We will reproduce this **5-region layout** (`tab bar / sidebar / center / footer / optional right panel`) but for **tools, not chat**. See §3 for the full ASCII mockup.

### 1.3 Bubbletea v2 architecture

The **single biggest change in v2** (per the official `UPGRADE_GUIDE_V2.md`) is the shift from **imperative commands** (`tea.WithAltScreen()`, `tea.EnterAltScreen`) to **declarative View fields** (`v.AltScreen = true`).

| v1 | v2 |
|---|---|
| `tea "github.com/charmbracelet/bubbletea"` | `tea "charm.land/bubbletea/v2"` |
| `View() string` | `View() tea.View` |
| `tea.KeyMsg` (struct) | `tea.KeyPressMsg` (interface) |
| `msg.String() == " "` | `msg.String() == "space"` |
| `tea.WithAltScreen()` | `v.AltScreen = true` in `View()` |
| `tea.EnterAltScreen` | `v.AltScreen = true` |
| `p.Start()` / `p.StartReturningModel()` | `p.Run()` |
| `tea.WindowSize()` (returns `Cmd`) | `tea.RequestWindowSize` (returns `Msg`) |
| `tea.Sequentially(...)` | `tea.Sequence(...)` |

The **new `tea.View` struct** lets you also set:

- `v.Cursor = &tea.Cursor{X, Y, Shape, Color, Blink}` — for inline editing
- `v.MouseMode = tea.MouseModeCellMotion`
- `v.DisableBracketedPasteMode = true`
- `v.ReportFocus = true`
- `v.WindowTitle = "sin-code tui"`
- `v.ProgressBar` — for native terminal progress

**New `Program` options**:
- `tea.WithColorProfile(p)` — force a profile, **gold for tests**
- `tea.WithWindowSize(w, h)` — initial size, **also gold for tests**

> **Implication for our `go.mod`:** the project currently uses `bubbletea v1.3.10` and `bubbles v1.0.0`. We have **two design paths**:
>
> 1. **Path A — Stay on v1** (recommended for v1.0 of this redesign): keep the existing imports, use the v1 API. We get a working TUI today.
> 2. **Path B — Migrate to v2** (follow-up PR): bump to `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`. The code in this document shows the **v1** API (so it compiles today); the v2 migration is a mechanical 1:1 swap documented in the official upgrade guide.
>
> Every Go snippet in this document will use **v1 syntax** and include inline `// v2:` migration comments where the syntax differs.

### 1.4 Bubbles components inventory

The full `charmbracelet/bubbles v1.0.0` package (we already import it):

| Component | Used for in sin-code tui v2 |
|---|---|
| `list` | Tools picker, Sessions list, History browser, Config tree |
| `viewport` | Center pane (renders scrolled tool output / EFM logs / config preview) |
| `textinput` | Inline argument input, filter input (`/`), command-palette search |
| `textarea` | Multi-line arg input, EFM compose file, raw CLI input |
| `spinner` | Tool-running indicator (per-tab) — we wrap it with our own cyber animation in §6 |
| `progress` | EFM container build progress, multi-step tool progress |
| `table` | EFM container table, key pool table (future), history table |
| `paginator` | Help screens, large result sets |
| `cursor` | Blink tick for textinputs/textarea |
| `filepicker` | Path argument picker for `discover`/`grasp`/`sckg` etc. |
| `timer` / `stopwatch` | Session time, command elapsed time |
| `help` | Auto-generated help from `key.Binding`s (the opencode `ctrl+p` pattern) |
| `key` | Remappable keybindings (future — for now just hardcoded map) |

### 1.5 Multi-pane TUI precedents (best practices we adopt)

| Project | Pattern we steal | Source |
|---|---|---|
| `charmbracelet/bubbletea` `examples/tabs` | tab-with-bottom-border style | `examples/tabs/main.go` (already uses v2) |
| `charmbracelet/bubbletea` `examples/split-editors` | focus-ring + side-by-side panels, `tea.Batch` for multiple cmds | `examples/split-editors/main.go` |
| `charmbracelet/bubbletea` `examples/composable-views` | `state sessionState` enum, `currentFocusedModel()`, two-pane layout | `examples/composable-views/main.go` |
| `charmbracelet/bubbletea` `examples/views` | "View A vs View B" toggle via `model.Chosen` bool, `tea.Tick` for animation | `examples/views/main.go` |
| `charmbracelet/bubbletea` `examples/doom-fire` | per-cell `lipgloss.NewStyle().Foreground(…).Background(…).Render("▀")` half-block — **direct inspiration for our cyber loading** | `examples/doom-fire/main.go` |
| `charmbracelet/bubbletea` `examples/spinners` | 9 stock spinners: `Line, Dot, MiniDot, Jump, Pulse, Points, Globe, Moon, Monkey` | `examples/spinners/main.go` |
| `charmbracelet/bubbletea` `examples/send-msg` | `p.Send(msg)` from outside the `Program` to stream events | `examples/send-msg/main.go` |
| `charmbracelet/bubbletea` `examples/chat` | viewport + textarea, `tea.WindowSizeMsg` resize, `lipgloss.Width()` wrap | `examples/chat/main.go` |
| `dlvhdr/gh-dash` | **per-section list with custom delegate**, YAML-defined layout — relevant to our "5 tabs" pattern | `github.com/dlvhdr/gh-dash` |
| `jesseduffield/lazygit` | **3-pane layout** (status, files, main) + side-panel on every list item — relevant to our sidebar/center pattern | `github.com/jesseduffield/lazygit` |
| `yorukot/superfile` | file-manager-style sidebar + 3-pane — proves the layout scales | `github.com/yorukot/superfile` |
| `charmbracelet/glow` | `viewport` + `glamour` markdown rendering for long content | (we use `viewport` directly) |
| `anomalyco/opencode` TUI | tab bar, agent switcher, footer with token/cost, sub-panels | `packages/opencode/src/cli/cmd/tui.ts` |

### 1.6 Unique TUI loading animations (cyber theme research)

The DOOM-fire example (`examples/doom-fire/main.go`) proves **per-cell, per-frame, two-tone half-block rendering** is feasible in a v2 Bubbletea app at 50ms tick rate. The pattern is:

```go
// per cell, per frame:
lipgloss.NewStyle().
    Foreground(lipgloss.ANSIColor(hiColor)).
    Background(lipgloss.ANSIColor(loColor)).
    Render("▀")
```

For our **sin-code cyber animation** we use the same technique but render a **rotating, ASCII-encoded encryption glyph** plus a **binary particle stream** at the footer. Full concept in §6.

---

## 2. Goals & Non-Goals

### Goals

1. **Discoverability** — every one of the 19 subcommands is one tab/keypress away.
2. **Multi-session** — run several tools in parallel, each in its own tab.
3. **No chat, no LLM** — this is a **command center**, not an AI agent. The footer has `0.0K (0%)   $0.00` placeholders ready for a future AI integration.
4. **Cyber aesthetic** — our own visual identity. Lightning ⚡ accents, neon-on-black, monospace everywhere, a unique animated loader.
5. **Bubbletea v1 first, v2-ready** — code compiles against `v1.3.10` today; v2 is a 1-day migration.
6. **TTY fallback preserved** — if no TTY, print a plain text catalog like the current implementation.
7. **Testable** — every submodel has `View()` returning a `string` (v1) / `tea.View` (v2) that we can `golden` in tests.
8. **Fits the project conventions** — every new file gets a `.doc.md` companion, every public function gets a docstring, no inline comments that just restate the code.

### Non-Goals (v1 of this redesign)

- Live LLM streaming (footer shows `0.0K (0%)`, but the LLM call is a `// TODO`).
- Mouse support beyond Bubbletea defaults (no `tea.MouseModeAllMotion`).
- Theme persistence (cycle through 5 themes in-session, no write to disk).
- Plugin system (no `~/.config/sin/tui/plugins/`).
- Tab drag-and-drop reorder.

---

## 3. Target Layout

### 3.1 ASCII mockup

```
                                                                              ⚡ sin-code
┌──────────────────────────────────────────────────────────────────────────────────────────────┐
│  TABS  [Tools] [Sessions] [EFM] [Config] [History]                              [+]   [search] │  ← tab bar (3 lines: top, tab row, bottom border)
├────────────────┬─────────────────────────────────────────────────────────┬────────────────────┤
│                │                                                         │                    │
│  SIDEBAR       │  CENTER PANE                                            │  RIGHT PANEL       │
│  ──────        │  ──────────                                             │  ──────            │
│  Sessions      │                                                         │  Stats / Context    │
│  • discover    │  Selected tool: discover                                │                    │
│  • map         │  Args: --path . --pattern **/*.go                       │  Active tool       │
│  • scout       │                                                         │  discover          │
│  • grasp       │  ┌─ Output (live) ─────────────────────────────────┐    │                    │
│                │  │ discover: scanning /Users/jeremy/dev/SIN-Code   │    │  Duration          │
│  Recent        │  │   .  142 files indexed                          │    │  00:00:03          │
│  #1 discover   │  │   .  312 files indexed                          │    │                    │
│  #2 map        │  │   .  ⚡ done in 0.42s                           │    │  Tokens / Cost     │
│  #3 scout      │  └─────────────────────────────────────────────────┘    │  0.0K (0%) $0.00   │
│                │                                                         │                    │
│  Commands      │  ┌─ Action ────────────────────────────────────────┐    │  Mode              │
│  /  filter     │  │ [Enter] run  [e] edit args  [o] open --help     │    │  Build ▸           │
│  :  cmd        │  │ [c] copy   [r] re-run     [x] clear output     │    │                    │
│  ?  help       │  └─────────────────────────────────────────────────┘    │  Agent             │
│  q  quit       │                                                         │  Build / Audit     │
│                │                                                         │                    │
├────────────────┴─────────────────────────────────────────────────────────┴────────────────────┤
│  ⚡ ROTATING-HALO  scanning 142 files...  |  build • agent:build  |  0.0K(0%)  $0.00  |  ^p cmds  │  ← footer
└──────────────────────────────────────────────────────────────────────────────────────────────┘
   ↑ top region (3 lines)        ↑ main region (terminal_height - 6)         ↑ bottom region (3 lines)
```

### 3.2 Region math

For terminal `W × H`:

| Region | Width | Height |
|---|---|---|
| Tab bar | W | 3 |
| Sidebar | 24 (or `min(28, W/5)`) | H - 6 |
| Center | W - sidebar - right | H - 6 |
| Right panel | 32 (or `0` if W < 100) | H - 6 |
| Footer | W | 3 |

Resized on every `tea.WindowSizeMsg`.

### 3.3 The 5 tabs (and their purpose)

| # | Tab | Sub-model | Components |
|---|---|---|---|
| 1 | **Tools** | `toolsModel` | `list.Model` of all 19 subcommands + right-pane preview + arg input |
| 2 | **Sessions** | `sessionsModel` | multi-tab sessions, each runs one tool, `viewport` for output, `spinner` while running |
| 3 | **EFM** | `efmModel` | `table.Model` of OrbStack containers, `progress.Model` for build, log `viewport` |
| 4 | **Config** | `configModel` | form with `textinput`s + `textarea` for TOML editing, plus diff preview |
| 5 | **History** | `historyModel` | `table.Model` of past invocations, `Enter` re-runs with same args |

The tab names map to a `TabKind` enum:

```go
type TabKind int
const (
    TabTools TabKind = iota
    TabSessions
    TabEFM
    TabConfig
    TabHistory
)
var tabLabels = []string{"Tools", "Sessions", "EFM", "Config", "History"}
```

### 3.4 The 19 subcommands available (from current `tui.go`)

```go
var sinCodeSubcommands = []ToolEntry{
    {Name: "discover",   Description: "Discover files with relevance scoring",     Category: "Search"},
    {Name: "execute",    Description: "Safe shell execution with redaction",       Category: "Exec"},
    {Name: "map",        Description: "Architecture map + dependency graph",       Category: "Code"},
    {Name: "grasp",      Description: "Deep single-file analysis",                 Category: "Code"},
    {Name: "scout",      Description: "Regex/semantic/symbol code search",         Category: "Search"},
    {Name: "harvest",    Description: "URL fetch + cache + structure extract",     Category: "Net"},
    {Name: "orchestrate",Description: "Task management with dependencies",         Category: "Tasks"},
    {Name: "ibd",        Description: "Intent-based diffing",                      Category: "Code"},
    {Name: "poc",        Description: "Proof-of-correctness verification",         Category: "Verify"},
    {Name: "sckg",       Description: "Semantic codebase knowledge graph",         Category: "Code"},
    {Name: "adw",        Description: "Architectural debt watchdogs",              Category: "Verify"},
    {Name: "oracle",     Description: "Verification oracle",                      Category: "Verify"},
    {Name: "efm",        Description: "Ephemeral full-stack mocking",              Category: "Mock"},
    {Name: "serve",      Description: "Start MCP server (stdio)",                  Category: "MCP"},
    // The 5 CLI-only (no MCP exposure) — visible in Tools, not invokable from MCP:
    {Name: "config",     Description: "Configuration management",                  Category: "Admin", CLIOnly: true},
    {Name: "tui",        Description: "This command — recursively",                Category: "Admin", CLIOnly: true},
    {Name: "sbom",       Description: "Generate SBOM (SPDX/CycloneDX)",            Category: "Verify", CLIOnly: true},
    {Name: "security",   Description: "Security scan (Go/Python/Node)",            Category: "Verify", CLIOnly: true},
    {Name: "self-update",Description: "Update to latest release",                  Category: "Admin", CLIOnly: true},
}
```

---

## 4. Component Inventory & Bubbles Mapping

For each UI surface, here's which `charmbracelet/bubbles` package to use, plus the **Lip Gloss style** it needs.

| Region | Bubbles component | Lip Gloss role | Notes |
|---|---|---|---|
| **Tab bar** | None (we draw the tabs ourselves) | `lipgloss.JoinHorizontal`, custom `tabBorderWithBottom` (copied from `examples/tabs`) | First tab is `Tab Tools`; `+` button at the end adds a new session tab. |
| **Sidebar — Sessions** | `list.Model` with custom `Delegate` | `delegates.StyledDelegate` from `bubbles/list` | Each session shows: name, status (●/○/⚠), elapsed time, last-line preview. |
| **Sidebar — Recent** | `list.Model` | normal title + dim desc | Last 10 invocations. |
| **Sidebar — Commands** | `help.Model` from `bubbles/help` | dim gray | The `?` key reveals a full help screen; bottom of sidebar always shows the top 4. |
| **Center — Tools tab** | `list.Model` (left half) + `viewport.Model` (right half) | tools = `list.DefaultDelegate`; right = rendered markdown description of selected tool | 50/50 split. Right side shows help text + example invocations. |
| **Center — Sessions tab** | `viewport.Model` per session + `textarea.Model` (or `textinput.Model` for one-line arg) at bottom | session = bordered, focused session = bold border | Multiple sessions stacked via the **multi-session sessions model** (see §5.4). |
| **Center — EFM tab** | `table.Model` (containers) + `progress.Model` (build) + `viewport.Model` (logs) | `table.New(...)` with custom `Styles.Header`, `Styles.Selected` | Mirrors how `lazygit` shows the status panel. |
| **Center — Config tab** | `[]textinput.Model` for scalar values + `textarea.Model` for TOML body | `cursorLineStyle`, `focusedBorderStyle` (copied from `examples/split-editors`) | Save → writes to `~/.config/sin/config.toml`; Cancel → discards. |
| **Center — History tab** | `table.Model` with sortable columns | columns: Time, Tool, Args, Status, Duration | Pressing `Enter` on a row jumps to the Sessions tab and re-runs it. |
| **Footer** | None — we render it directly from a `footerView` function | `lipgloss.JoinHorizontal` of 4 segments | See §5.5 for the 4-segment layout. |
| **Cyber loader** | Custom — see §6 | per-cell half-block, like `examples/doom-fire` | Not a `spinner.Model`; a bespoke model that ticks at ~50ms. |

---

## 5. Detailed Sub-Model Design

### 5.1 The root model

```go
// tui.go — top-level state

type rootModel struct {
    width, height int
    activeTab     TabKind
    tabs          []*tabState   // one per kind; future: multiple sessions
    sidebar       sidebarModel
    rightPanel    rightPanelModel
    footer        footerModel
    loader        *cyberLoader  // nil when idle

    // Key handling
    keymap keymap
    help   help.Model

    // Theme
    theme  Theme

    // Status of any running tool (one-at-a-time in v1)
    running *runningTool

    quitting bool
}

type tabState struct {
    kind   TabKind
    title  string
    sub    tea.Model   // toolsModel, sessionsModel, efmModel, configModel, historyModel
    dirty  bool        // true if sub has unsaved state
}
```

The root `Update` does two things:
1. If `m.running != nil`, intercept every message and route to the running tool (a goroutine in the background streams its stdout to a `chan tea.Msg`).
2. Otherwise, route to the **active tab's sub-model** plus a few **global** keys (Tab, Shift+Tab, Ctrl+P, Ctrl+C, `?`).

### 5.2 The keymap

```go
type keymap struct {
    // Tab navigation
    NextTab     key.Binding  // tab, ctrl+n
    PrevTab     key.Binding  // shift+tab, ctrl+p    (NB: conflicts with command palette — use ctrl+right/left)
    NewTab      key.Binding  // ctrl+t

    // Global
    Quit        key.Binding  // q, ctrl+c
    Help        key.Binding  // ?
    CommandPalette key.Binding // ctrl+p — open the cmd palette
    Interrupt   key.Binding  // esc — kill running tool

    // Sidebar
    ToggleSidebar  key.Binding // ctrl+b — collapse/expand sidebar
    FocusSidebar   key.Binding // alt+1
    FocusCenter    key.Binding // alt+2
    FocusRight     key.Binding // alt+3

    // Tab-specific
    RunTool     key.Binding // enter (when in tools tab)
    EditArgs    key.Binding // e
    ClearOutput key.Binding // x
    ReRun       key.Binding // r
    NewSession  key.Binding // ctrl+n (when in sessions tab)
    CloseSession key.Binding // ctrl+w

    // Agent switcher (Build / Audit / Stats)
    NextAgent   key.Binding // ctrl+]
    PrevAgent   key.Binding // ctrl+[

    // EFM
    EFMUp       key.Binding // ctrl+shift+up
    EFMDown     key.Binding // ctrl+shift+down
}
```

Every binding also gets a `key.WithHelp("…", "short description")` so the `help.Model` can auto-generate a keymap help screen.

### 5.3 Tools tab

```go
type toolsModel struct {
    list      list.Model
    selected  ToolEntry
    argInput  textinput.Model
    editing   bool
    viewport  viewport.Model  // right side: tool description + examples
    listW, vpW int
}
```

- The list is a `list.Model` of all 19 subcommands.
- Pressing `Enter` switches to a "Run" mode: shows an inline `textinput.Model` (or `textarea.Model` for tools with multi-line args) at the bottom, with the `textinput.Focus()` set.
- Pressing `Enter` again actually runs the tool — spawns a `goroutine`, captures stdout/stderr line-by-line into a channel, each line becomes a `toolOutputMsg{line string}`.
- The right-pane `viewport.Model` shows the `Long` description of the currently-selected tool, rendered with `glamour` if available, else plain `strings.Builder`.

### 5.4 Sessions tab (multi-session)

This is the most complex sub-model. It's a **stack of sessions**, each running one tool, with one *active* (focused) session.

```go
type sessionsModel struct {
    sessions []*sessionModel
    active   int
}

type sessionModel struct {
    id        string      // ulid
    tool      ToolEntry
    args      []string
    started   time.Time
    status    SessionStatus  // Running, Done, Failed, Killed
    exitCode  int
    output    []string     // captured stdout+stderr lines (capped at 10_000)
    viewport  viewport.Model
    spinner   spinner.Model
    cmd       *exec.Cmd
    cancel    context.CancelFunc
    errCh     chan error
    lineCh    chan string
}

type SessionStatus int
const (
    SessionRunning SessionStatus = iota
    SessionDone
    SessionFailed
    SessionKilled
)
```

`tea.Batch` is used to subscribe to multiple channels (e.g. spinner tick + line from tool).

Pressing `Ctrl+N` in the Sessions tab opens a "new session" prompt (a `list.Model` of available tools, like the Tools tab but transient).

### 5.5 Footer

A 3-line strip at the bottom of the screen, split into 4 horizontal segments:

```
┌─left──────────────────────┬─center──────────────────┬─center-right────┬─right──────────────┐
│ ⚡ loader  status text     │ build • agent:build    │ 0.0K(0%)  $0.00 │ ^p cmds  ? help  q  │
└───────────────────────────┴─────────────────────────┴──────────────────┴────────────────────┘
```

Each segment is a `lipgloss.Style` with a different `Foreground` color from the active `Theme`. The footer has **no bubbletea sub-model** — it's rendered by a `footerView(m *footerModel, w, h int) string` function called from `rootModel.View()`.

```go
type footerModel struct {
    loaderFrame int
    statusText  string
    agent       Agent
    tokens      int
    cost        float64
    theme       Theme
}

type Agent int
const (
    AgentBuild Agent = iota
    AgentAudit
    AgentStats
)
var agentNames = []string{"build", "audit", "stats"}
var agentIcons = []string{"🔨", "🛡", "📊"}
```

`Ctrl+]` and `Ctrl+[` cycle `agent`. The agent name flows into the footer and into the active sub-model (in the future, "audit" could disable Run and only show `ceo-audit` / `adw` / `oracle` results).

### 5.6 Sidebar

The sidebar has 3 collapsible sections:

```
SESSIONS
  ●  discover   00:00:03
  ●  map        00:00:00
  ⚠  scout      failed (exit 1)

RECENT
  1. discover --path . --pattern **/*.go     3s ago
  2. map --path ./cmd                        12s ago

COMMANDS
  /  filter
  :  cmd palette
  ?  help
  q  quit
```

Each section is a `list.Model` with a custom `Delegate` that shows a status glyph. `Ctrl+B` toggles a "collapsed" mode where only the section titles are shown.

```go
type sidebarModel struct {
    collapsed   bool
    sessions    list.Model
    recent      list.Model
    help        help.Model
    width       int
    height      int
    focusedIdx  int  // 0=sessions, 1=recent, 2=commands
}
```

### 5.7 Right panel (optional — only on wide terminals)

Hidden when `width < 100`. When shown, contains:

```
STATS
  Active tool     discover
  Session #       3
  Duration        00:00:03
  Lines captured  412

CONTEXT
  Project         /Users/jeremy/dev/SIN-Code-Bundle
  Files           142
  Languages       Go, Python, MD

ENV
  GOOS            darwin
  Go version      1.25
  sin-code        v0.1.0
```

Static info refreshed on `tea.WindowSizeMsg` and on tool start.

---

## 6. The Cyber Loader — Unique SIN-Code Loading Animation

We want a **distinctly sin-code** loader. Design goals:

- **Recognizable**: not a stock spinner; clearly ours.
- **Cyber/hacker**: encryption glyphs, binary particles, neon-on-black.
- **Themed**: color comes from the active `Theme`.
- **Performant**: 30-50ms tick rate, no per-frame allocations.

### 6.1 Concept: "Rotating Halo + Binary Rain"

Two components, rendered side-by-side in the footer:

**Left — Rotating Halo (3 cells wide)**

A rotating half-block "halo" using `▀ ▄ ▌ ▐ ▝ ▘ ▗ ▖` (the 8 half-block Unicode glyphs). Each frame rotates 45°.

```
frame 0: ▗▖▝▘     frame 1: ▘▗▖▝     frame 2: ▝▘▗▖     ...
```

**Right — Binary Particle Rain (16 cells wide)**

A 16-column window that scrolls binary digits down 1 row per frame, with a "head" cell that flashes bright (the active particle).

```
frame 0:  0 1 0 0 1 0 1 1 0 1 0 0 1 1 0 1
frame 1:  1 0 0 1 1 0 1 0 0 1 1 0 1 0 0 1
frame 2:  0 1 1 0 1 0 1 0 0 0 1 0 1 1 0 0
...
```

The "head" position is a fixed cell that flashes between two bright colors (e.g. theme primary ↔ theme accent).

### 6.2 ASCII preview

```
⚡ ⟮▗▖▝⟯  01001101  00101100  ⚡ ROTATING-HALO  scanning 142 files
```

Or, when idle:
```
⚡ ⟮▝▘▗⟯  11110000  00001111  ⚡ sin-code  ready  (5s since last cmd)
```

### 6.3 Implementation

```go
// loader.go

// Purpose: CyberLoader — sin-code's bespoke loading animation, a
// rotating half-block halo next to a binary particle stream.
// Renders into a fixed-size box (default 24×1), refreshed every 50ms via
// loaderTickMsg. No allocations per frame.
// Docs: loader.doc.md
type CyberLoader struct {
    frame     int
    width     int   // binary rain columns (default 16)
    haloColor lipgloss.Color
    headColor lipgloss.Color
    bgColor   lipgloss.Color
}

func NewCyberLoader() *CyberLoader {
    return &CyberLoader{width: 16}
}

func (l *CyberLoader) Tick() tea.Cmd {
    return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
        return loaderTickMsg{}
    })
}

// halo frames — 8 half-block glyphs, rotating
var haloFrames = []rune{'▗', '▖', '▝', '▘', '▗', '▖', '▝', '▘'}

func (l *CyberLoader) View() string {
    if l == nil { return "" }
    halo := string(haloFrames[l.frame%8])
    // Build a fixed-width binary string; the "head" cell flashes
    var b strings.Builder
    head := l.frame % l.width
    rng := rand.New(rand.NewSource(int64(l.frame)))
    for i := 0; i < l.width; i++ {
        bit := byte('0')
        if rng.Intn(2) == 1 { bit = '1' }
        cell := string(bit)
        style := lipgloss.NewStyle().Foreground(l.haloColor)
        if i == head {
            style = style.Foreground(l.headColor).Bold(true)
        }
        b.WriteString(style.Render(cell))
    }
    haloStyled := lipgloss.NewStyle().
        Foreground(l.headColor).Bold(true).
        Render("⟮" + halo + halo + halo + "⟯")
    return haloStyled + "  " + b.String()
}
```

### 6.4 Integration with the footer

The footer `View()` embeds the loader:

```go
func (f footerModel) View() string {
    left   := f.loader.View() + "  " + f.statusText
    center := f.agentIcon + " " + f.agentName + " • agent:" + agentNames[f.agent]
    mid    := fmt.Sprintf("%sK(%d%%)  $%.2f",
                           humanK(f.tokens), pct(f.tokens, 200_000), f.cost)
    right  := "^p cmds  ? help  q quit"
    return lipgloss.JoinHorizontal(lipgloss.Top,
        footerStyleLeft.Render(left),
        footerStyleCenter.Render(center),
        footerStyleMid.Render(mid),
        footerStyleRight.Render(right),
    )
}
```

### 6.5 Themes

5 built-in themes, cycled with `t`:

| Index | Name | Halo | Head | Status text |
|---|---|---|---|---|
| 0 | Sin-Default | `#7D56F4` (purple) | `#FF79C6` (pink) | `#FFFFFF` |
| 1 | Matrix | `#003300` (dark green) | `#00FF41` (bright green) | `#00FF41` |
| 2 | Cyberpunk | `#00FFFF` (cyan) | `#FF00FF` (magenta) | `#FFFFFF` |
| 3 | Amber-Terminal | `#FFB000` (amber) | `#FFFFFF` | `#FFB000` |
| 4 | Red-Alert | `#330000` | `#FF0033` | `#FF0033` |

Each theme is a `struct{ Halo, Head, Text, Bg, Border, Accent lipgloss.Color }` with pre-computed `lipgloss.Style` instances for the most common uses.

---

## 7. Key Bindings (Full Table)

### 7.1 Global (always active)

| Key | Action | Notes |
|---|---|---|
| `q` | Quit | confirms if sessions running |
| `ctrl+c` | Quit (hard) | kills all child processes |
| `?` | Open help screen | uses `bubbles/help` |
| `ctrl+p` | Command palette | `textinput` over a `list.Model` of all commands |
| `ctrl+b` | Toggle sidebar | collapses to icons-only |
| `ctrl+]` / `ctrl+[` | Next/prev agent | cycles Build / Audit / Stats |
| `ctrl+n` | New session (in Sessions tab) | opens a tool picker |
| `ctrl+w` | Close current session (in Sessions tab) | kills if running |
| `esc` | Interrupt current tool | sends SIGINT to the running exec.Cmd |
| `tab` | Focus next pane (sidebar → center → right) | |
| `shift+tab` | Focus prev pane | |
| `t` | Cycle theme | 5 themes |

### 7.2 Per-tab

| Tab | Key | Action |
|---|---|---|
| Tools | `↑/↓` or `j/k` | Navigate tool list |
| Tools | `Enter` | Run selected tool (prompts for args if needed) |
| Tools | `e` | Edit args inline |
| Tools | `/` | Filter tools by name/description |
| Sessions | `Enter` | Switch to selected session |
| Sessions | `n` | New session |
| Sessions | `x` | Clear output buffer |
| Sessions | `r` | Re-run last tool with same args |
| Sessions | `pgup/pgdn` | Scroll output viewport |
| EFM | `Enter` | Show container logs |
| EFM | `b` | Build new EFM stack |
| EFM | `d` | Destroy selected container |
| Config | `Tab` | Next field |
| Config | `Shift+Tab` | Prev field |
| Config | `ctrl+s` | Save |
| Config | `Esc` | Cancel / revert |
| History | `Enter` | Re-run with same args (jumps to Sessions) |
| History | `d` | Delete row from history |

### 7.3 Command palette (Ctrl+P)

Same as the opencode `ctrl+p` pattern (commands reachable by typing):

```
> _
  ⚡  New session
  ⚡  Switch to Tools
  ⚡  Switch to Sessions
  ⚡  Switch to EFM
  ⚡  Switch to Config
  ⚡  Switch to History
  ⚡  Cycle theme
  ⚡  Reload config
  ⚡  Run last command
  ⚡  Show stats
  ⚡  Quit sin-code tui
```

Implementation: a `list.Model` with a `textinput.Model` filter at the top. We use the `list.Filter` feature, which we already use in the current `tui.go` (`l.SetFilteringEnabled(true)`).

---

## 8. File Structure Proposal

The current `tui.go` (264 lines) becomes a small entry point that imports a new `internal/tui/` package. Why `internal/`? Because the sub-models are not part of the public CLI surface — they're an implementation detail of the `tui` subcommand.

### 8.1 New layout

```
cmd/sin-code/
├── main.go                          # registers tuiCmd (unchanged)
├── tui.go                           # new — thin entry point (≈50 lines)
├── tui.doc.md                       # new — overall design doc (this file)
└── internal/
    └── tui/
        ├── doc.md                   # package-level doc
        ├── root.go                  # rootModel + Init/Update/View
        ├── root_test.go             # golden tests of root.View()
        ├── keymap.go                # all key.Binding structs
        ├── theme.go                 # 5 themes + cycle
        ├── theme_test.go
        ├── loader.go                # CyberLoader (per §6)
        ├── loader.doc.md
        ├── loader_test.go           # golden tests of loader.View()
        ├── footer.go                # footerModel + footerView
        ├── footer_test.go
        ├── sidebar.go               # sidebarModel
        ├── sidebar_test.go
        ├── rightpanel.go            # rightPanelModel
        ├── rightpanel_test.go
        ├── tools.go                 # toolsModel
        ├── tools_test.go
        ├── sessions.go              # sessionsModel + sessionModel
        ├── sessions_test.go
        ├── efm.go                   # efmModel (wraps existing sin-code efm subcmd)
        ├── efm_test.go
        ├── config.go                # configModel
        ├── config_test.go
        ├── history.go               # historyModel
        ├── history_test.go
        ├── palette.go               # command palette modal
        ├── palette_test.go
        ├── subcommands.go           # the 19 ToolEntry list
        ├── runner.go                # exec.Cmd + line channel + cancel
        ├── runner_test.go
        ├── store.go                 # history.jsonl read/write
        ├── store_test.go
        └── toolsdata/
            ├── discover.json        # static descriptions
            ├── map.json
            └── ...
```

### 8.2 File roles

| File | Role | Approx LOC |
|---|---|---|
| `tui.go` (new) | Entry point: `tea.NewProgram(rootModel{}, ...).Run()`; falls back to plain text on no-TTY | 80 |
| `root.go` | `rootModel`, the top-level `Update`/`View`, global keymap | 250 |
| `keymap.go` | All `key.Binding` declarations + grouped help | 120 |
| `theme.go` | `Theme` struct, 5 themes, `cycleTheme()` | 80 |
| `loader.go` | `CyberLoader` | 100 |
| `footer.go` | `footerModel` (status + tokens + cost + agent) | 120 |
| `sidebar.go` | `sidebarModel` with 3 collapsible lists | 180 |
| `rightpanel.go` | `rightPanelModel` (stats / context / env) | 100 |
| `tools.go` | `toolsModel` (list + viewport split) | 220 |
| `sessions.go` | `sessionsModel` (multi-session) | 350 |
| `efm.go` | `efmModel` (table + progress + logs) | 280 |
| `config.go` | `configModel` (form + diff) | 220 |
| `history.go` | `historyModel` (table) | 150 |
| `palette.go` | `paletteModel` (modal) | 130 |
| `subcommands.go` | The 19-entry static list | 80 |
| `runner.go` | `ToolRunner` (exec.Cmd + line channel) | 200 |
| `store.go` | `~/.local/state/sinator/tui-history.jsonl` | 100 |

**Total ≈ 2,600 LOC** — a big jump from 264, but each file is small and single-purpose.

### 8.3 Migration strategy

1. **Phase 1 (this PR)**: Add `internal/tui/` and the new `tui.go` that uses it. **Keep** the old 264-line `tui.go` behavior behind a `--legacy` flag for 1 release.
2. **Phase 2 (next PR)**: Remove `--legacy`. Move the old file to `cmd/sin-code/tui_legacy.go.bak` in git history only.
3. **Phase 3 (follow-up)**: Migrate to Bubbletea v2 in a separate PR.

---

## 9. Implementation Roadmap (Steps in Order)

### Step 0 — Bootstrap (½ day)
- [ ] Add `internal/tui/` directory
- [ ] Copy current `tui.go` behavior into a `legacyMode` variable so the new TUI can fall back to it on panic
- [ ] Set up `go test ./cmd/sin-code/internal/tui/...` workflow

### Step 1 — Root + Theme + Loader (1 day)
- [ ] `root.go` with the 5-region layout, **no sub-models yet** — just static placeholder content
- [ ] `theme.go` with 5 themes and a `cycleTheme()` function
- [ ] `loader.go` with the `CyberLoader`
- [ ] `footer.go` rendering the loader + status
- [ ] Test: launch TUI in test mode (`tea.WithColorProfile(...)` + `tea.WithWindowSize(120, 40)`), capture `View()` output, assert it contains "sin-code"

### Step 2 — Tools tab (1 day)
- [ ] `subcommands.go` with the 19 entries
- [ ] `tools.go` with the `list.Model` + `viewport.Model` split
- [ ] `runner.go` with `exec.Cmd` + line channel
- [ ] `store.go` for history persistence
- [ ] Test: golden test of `toolsModel.View()` with sample data

### Step 3 — Sidebar + Footer + Right panel (½ day)
- [ ] `sidebar.go` with 3 collapsible lists
- [ ] `rightpanel.go` with static stats
- [ ] `footer.go` (already from Step 1) gets the agent switcher
- [ ] Test: golden tests of each sub-model

### Step 4 — Sessions tab (2 days)
- [ ] `sessions.go` with `sessionsModel` + `sessionModel`
- [ ] Spawn a tool: goroutine → line channel → `toolLineMsg{line}` → append to output
- [ ] Kill a tool: `cancel()` → `SessionKilled` status
- [ ] Multiple sessions: `tea.Batch` over multiple `spinner.TickMsg` and `toolLineMsg`
- [ ] Test: integration test that runs `discover --help` in a fake session and asserts the output reaches the viewport

### Step 5 — EFM tab (1 day)
- [ ] `efm.go` calls the existing `sin-code efm` subcommand via `exec.Command`
- [ ] `table.Model` of containers
- [ ] `progress.Model` for build progress
- [ ] Test: golden test of EFM table with 3 fake containers

### Step 6 — Config tab (1 day)
- [ ] `config.go` reads `~/.config/sin/config.toml`
- [ ] `[]textinput.Model` for scalar fields
- [ ] `textarea.Model` for the raw TOML body
- [ ] Save → `os.WriteFile` (with backup)
- [ ] Test: read a fixture TOML, assert fields are populated; modify, save, read again

### Step 7 — History tab (½ day)
- [ ] `history.go` reads `~/.local/state/sinator/tui-history.jsonl`
- [ ] `table.Model` with sortable columns
- [ ] Pressing `Enter` jumps to Sessions tab and re-runs
- [ ] Test: write a fixture history file, assert the table renders correctly

### Step 8 — Command palette + Help (½ day)
- [ ] `palette.go` is a `list.Model` modal that opens on `ctrl+p`
- [ ] `help.go` (or inline in `root.go`) renders the full keymap
- [ ] Test: type into the palette, assert the list filters

### Step 9 — Polish (1 day)
- [ ] TTY fallback: if `tea.NewProgram` returns an error, fall back to plain text (preserved from current `tui.go`)
- [ ] Resize handling: every sub-model reacts to `tea.WindowSizeMsg`
- [ ] Color profile detection: degrade gracefully on `NoColor`
- [ ] CoDocs: every file gets a `.doc.md` companion
- [ ] Lint: `golangci-lint run ./cmd/sin-code/...`

### Step 10 — Bubbletea v2 migration (1 day, follow-up PR)
- [ ] Bump `go.mod`: `charmbracelet/bubbletea v2.x`, `charmbracelet/bubbles v2.x`, `charmbracelet/lipgloss v2.x`
- [ ] Update imports
- [ ] Change `View() string` → `View() tea.View` everywhere
- [ ] Add `v.AltScreen = true` in `rootModel.View()`
- [ ] Replace `case " ":` with `case "space":` (we have none, so no-op)
- [ ] Replace `tea.KeyMsg` (struct) with `tea.KeyPressMsg`
- [ ] Run `golangci-lint` and `go test`

**Total estimate: ~8-10 days** for one engineer.

---

## 10. Code Snippets — The Critical Parts

> All snippets are **Bubbletea v1** (matches our current `go.mod`). v2 migration is a mechanical 1:1 swap; the official upgrade guide is the source of truth.

### 10.1 Root model — the orchestrator

```go
// internal/tui/root.go
package tui

import (
    "fmt"

    "github.com/charmbracelet/bubbles/help"
    "github.com/charmbracelet/bubbles/key"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// rootModel is the top-level tea.Model. It owns the layout, the active tab,
// the sidebar, the right panel, and the footer. Sub-models are owned by
// individual tabs and receive Update calls via dispatch.
type rootModel struct {
    width, height int
    activeTab     TabKind
    tabs          []tabState
    sidebar       sidebarModel
    right         rightPanelModel
    footer        footerModel
    loader        *CyberLoader
    keymap        rootKeymap
    help          help.Model
    theme         Theme
    quitting      bool
    showHelp      bool
    showPalette   bool
    palette       *paletteModel
}

func newRootModel() rootModel {
    tabs := []tabState{
        {kind: TabTools, title: "Tools", sub: newToolsModel()},
        {kind: TabSessions, title: "Sessions", sub: newSessionsModel()},
        {kind: TabEFM, title: "EFM", sub: newEFMModel()},
        {kind: TabConfig, title: "Config", sub: newConfigModel()},
        {kind: TabHistory, title: "History", sub: newHistoryModel()},
    }
    return rootModel{
        tabs:    tabs,
        sidebar: newSidebarModel(),
        right:   newRightPanelModel(),
        footer:  newFooterModel(),
        loader:  NewCyberLoader(),
        keymap:  defaultRootKeymap(),
        help:    help.New(),
        theme:   defaultTheme,
    }
}

func (m rootModel) Init() tea.Cmd {
    return tea.Batch(
        m.loader.Tick(),
        m.tabs[m.activeTab].sub.Init(),
    )
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    // Help modal takes precedence.
    if m.showHelp {
        if km, ok := msg.(tea.KeyMsg); ok && km.String() == "?" {
            m.showHelp = false
            return m, nil
        }
        // Bubble the help model
        var cmd tea.Cmd
        m.help, cmd = m.help.Update(msg)
        return m, cmd
    }

    // Command palette modal.
    if m.showPalette {
        if km, ok := msg.(tea.KeyMsg); ok {
            if km.String() == "esc" {
                m.showPalette = false
                m.palette = nil
                return m, nil
            }
        }
        var cmd tea.Cmd
        m.palette, cmd = m.palette.Update(msg)
        if acted, action := m.palette.Action(); acted {
            return m.handleAction(action)
        }
        return m, cmd
    }

    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.relayout()
        // Propagate to sub-models.
        for i := range m.tabs {
            var cmd tea.Cmd
            m.tabs[i].sub, cmd = m.tabs[i].sub.Update(msg)
            cmds = append(cmds, cmd)
        }
        return m, tea.Batch(cmds...)

    case loaderTickMsg:
        m.loader.frame++
        return m, m.loader.Tick()

    case tea.KeyMsg:
        // Global keys first.
        switch {
        case key.Matches(msg, m.keymap.Quit):
            m.quitting = true
            return m, tea.Quit
        case key.Matches(msg, m.keymap.Help):
            m.showHelp = true
            return m, nil
        case key.Matches(msg, m.keymap.CommandPalette):
            m.palette = newPaletteModel()
            m.showPalette = true
            return m, m.palette.Init()
        case key.Matches(msg, m.keymap.NextTab):
            m.activeTab = TabKind((int(m.activeTab) + 1) % len(m.tabs))
            return m, m.tabs[m.activeTab].sub.Init()
        case key.Matches(msg, m.keymap.PrevTab):
            m.activeTab = TabKind((int(m.activeTab) - 1 + len(m.tabs)) % len(m.tabs))
            return m, m.tabs[m.activeTab].sub.Init()
        case key.Matches(msg, m.keymap.CycleTheme):
            m.theme = m.theme.Next()
            m.relayout() // re-apply theme to all sub-models
            return m, nil
        case key.Matches(msg, m.keymap.ToggleSidebar):
            m.sidebar.collapsed = !m.sidebar.collapsed
            m.relayout()
            return m, nil
        }
    }

    // Delegate to active tab.
    var cmd tea.Cmd
    m.tabs[m.activeTab].sub, cmd = m.tabs[m.activeTab].sub.Update(msg)
    cmds = append(cmds, cmd)
    return m, tea.Batch(cmds...)
}
```

### 10.2 The cyber loader (per §6)

```go
// internal/tui/loader.go (full file shown in §6.3)
type CyberLoader struct {
    frame     int
    width     int
    haloColor lipgloss.Color
    headColor lipgloss.Color
    textColor lipgloss.Color
}

// ... (as in §6.3)
```

### 10.3 Tab bar renderer

```go
// internal/tui/root.go — tabBarView()

func (m rootModel) tabBarView() string {
    var rendered []string
    for i, t := range m.tabs {
        style := m.theme.TabInactive
        if TabKind(i) == m.activeTab {
            style = m.theme.TabActive
        }
        rendered = append(rendered, style.Render(t.title))
    }
    // "+" button at the end (in v1, just decorative; future: add session).
    rendered = append(rendered, m.theme.TabInactive.Render(" + "))
    row := lipgloss.JoinHorizontal(lipgloss.Bottom, rendered...)
    // Bottom border to separate from main area.
    border := strings.Repeat("─", m.width)
    return row + "\n" + m.theme.Border.Render(border)
}
```

### 10.4 Footer (with loader + tokens + cost + agent)

```go
// internal/tui/footer.go
type footerModel struct {
    loader     *CyberLoader
    statusText string
    agent      Agent
    tokens     int
    cost       float64
    theme      Theme
    width      int
}

func (f footerModel) View() string {
    left := f.loader.View() + "  " + f.statusText
    center := fmt.Sprintf("%s %s • agent:%s",
                          agentIcons[f.agent], agentNames[f.agent], agentNames[f.agent])
    mid := fmt.Sprintf("%sK(%d%%)  $%.2f",
                       humanK(f.tokens), pct(f.tokens, 200_000), f.cost)
    right := "^p cmds  ? help  q quit"
    seg := func(content string, w int, style lipgloss.Style) string {
        return style.Width(w).Render(content)
    }
    return lipgloss.JoinHorizontal(lipgloss.Top,
        seg(left,  f.width/3,         f.theme.FooterLeft),
        seg(center, f.width/4,       f.theme.FooterCenter),
        seg(mid,    f.width/4,       f.theme.FooterMid),
        seg(right,  f.width-f.width/3-f.width/4-f.width/4, f.theme.FooterRight),
    )
}

func humanK(n int) string {
    if n < 1000 { return fmt.Sprintf("%d", n) }
    return fmt.Sprintf("%.1f", float64(n)/1000.0)
}
func pct(n, max int) int {
    if max == 0 { return 0 }
    return n * 100 / max
}
```

### 10.5 Sidebar — 3 collapsible lists

```go
// internal/tui/sidebar.go
type sidebarModel struct {
    collapsed  bool
    sessions   list.Model
    recent     list.Model
    helpKeys   help.Model
    width      int
    height     int
    focused    int // 0=sessions, 1=recent
}

func (m sidebarModel) View() string {
    if m.collapsed {
        // 1-column-wide icons
        return m.theme.SidebarCollapsed.Render("⚡\n●\n○\n…")
    }
    sections := []string{
        m.theme.SidebarHeader.Render("SESSIONS"),
        m.sessions.View(),
        m.theme.SidebarHeader.Render("RECENT"),
        m.recent.View(),
        m.theme.SidebarHeader.Render("COMMANDS"),
        m.theme.SidebarCommands.Render("/ filter  : palette  ? help  q quit"),
    }
    return strings.Join(sections, "\n")
}
```

### 10.6 Session runner — the goroutine bridge

```go
// internal/tui/runner.go
type ToolRunner struct {
    cmd     *exec.Cmd
    cancel  context.CancelFunc
    LineCh  chan string
    DoneCh  chan error
}

func NewToolRunner(name string, args []string) *ToolRunner {
    ctx, cancel := context.WithCancel(context.Background())
    cmd := exec.CommandContext(ctx, "sin-code", append([]string{name}, args...)...)
    r := &ToolRunner{
        cmd:    cmd,
        cancel: cancel,
        LineCh: make(chan string, 256),
        DoneCh: make(chan error, 1),
    }
    go r.run()
    return r
}

func (r *ToolRunner) run() {
    stdout, _ := r.cmd.StdoutPipe()
    stderr, _ := r.cmd.StderrPipe()
    if err := r.cmd.Start(); err != nil {
        r.DoneCh <- err
        return
    }
    go scanLines(stdout, r.LineCh)
    go scanLines(stderr, r.LineCh)
    r.DoneCh <- r.cmd.Wait()
}

func (r *ToolRunner) Kill() { r.cancel() }

// In sessionsModel.Update:
case toolLineMsg{line: l}:
    s.output = append(s.output, l)
    if len(s.output) > 10_000 {
        s.output = s.output[len(s.output)-10_000:]
    }
    s.refreshViewport()
case toolDoneMsg{err: e}:
    s.status = SessionDone
    if e != nil { s.status = SessionFailed }
    return m, nil
```

### 10.7 The "no TTY" fallback (preserved from current `tui.go`)

```go
// cmd/sin-code/tui.go (new, thin)
func runTUI() error {
    if !isatty(os.Stdout.Fd()) {
        return printPlainCatalog()
    }
    p := tea.NewProgram(newRootModel(), tea.WithAltScreen())
    _, err := p.Run()
    if err != nil {
        // Bubble Tea can fail on certain TTYs (e.g. CI). Don't crash.
        return printPlainCatalog()
    }
    return nil
}
```

---

## 11. Testing Strategy

### 11.1 Unit tests (per file)

Each sub-model gets at least 3 tests:

1. **Construction** — `newFooModel()` returns a non-nil, sane-default model
2. **Update** — feed a `tea.KeyMsg`, assert the model's state changed correctly
3. **View** — render with a fixed `width=120, height=40`, golden-compare to a fixture file

The **golden file** pattern (from `tests/golden/`):

```
internal/tui/testdata/
├── toolsModel_120x40.golden
├── toolsModel_filter_active.golden
├── sessionsModel_running.golden
├── footer_with_loader.golden
└── loader_frame_3.golden
```

Run with:
```bash
go test ./cmd/sin-code/internal/tui/... -update   # write fixtures
go test ./cmd/sin-code/internal/tui/...            # verify
```

### 11.2 Integration tests

- `runner_test.go`: spawn `sin-code discover --path .`, capture 5 lines, assert lines come in order and the `DoneCh` fires.
- `palette_test.go`: open the palette, type "theme", press Enter, assert the theme cycled.
- `sessions_test.go`: start a real session, wait for `SessionDone`, assert the output contains the expected line.

### 11.3 Visual tests with VHS

A `tests/vhs/` directory with `.tape` files (VHS = Video Home System, a TUI recorder from charmbracelet) for the major flows:

- `tui-launch.tape` — boot, see the tabs, see the loader
- `tui-tools-run.tape` — select `discover`, run, see the output stream
- `tui-sessions-multi.tape` — start 3 sessions, watch them all run
- `tui-efm.tape` — show 3 EFM containers in the table
- `tui-theme-cycle.tape` — cycle through all 5 themes

These double as **marketing screenshots** for the README.

---

## 12. Open Questions & Trade-offs

| Question | Default answer | Alternative |
|---|---|---|
| Bubbletea v1 vs v2? | **v1 first** (matches current `go.mod`), v2 in a follow-up PR | Jump to v2 immediately — blocks the redesign behind a `go.mod` bump |
| Single binary vs multiple sub-bins? | **Single `sin-code tui` binary**, internal package | Multiple binaries share `internal/tui/` — but harder to test |
| Store history where? | `~/.local/state/sinator/tui-history.jsonl` | `~/.config/sin/tui-history.jsonl` — but JSONL writes are append-only, so `state/` is the XDG-canonical place |
| Run tools in the same process or subprocess? | **Subprocess** (`exec.Command("sin-code", ...)`) | In-process via the cobra command tree — but more coupling |
| Cancel via SIGINT or context? | **context.CancelFunc** mapped to `SIGTERM` first, `SIGKILL` after 5s timeout | Just `cmd.Process.Kill()` — but loses the chance to clean up child processes |
| Tabs horizontally or vertically on left? | **Horizontally at top** (matches opencode, gh-dash) | Vertically on left (matches btop, lazygit) — but doesn't scale to 5+ tabs |
| Custom cyber loader vs stock spinner? | **Custom** (per §6) | Stock `spinner.Globe` is too generic for the brand |
| Multi-pane right panel always-on or toggleable? | **Always-on on `width >= 100`, hidden otherwise** | Always-on with a `[` / `]` toggle — but more state to manage |
| Theme persistence? | **Not in v1** (session only) | Write to `~/.config/sin/tui.toml` — extra code, not requested |

---

## 13. References

### Source code consulted

- [anomalyco/opencode](https://github.com/anomalyco/opencode) — `packages/opencode/src/cli/cmd/tui.ts`, `packages/opencode/src/cli/tui/`
- [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) — `UPGRADE_GUIDE_V2.md`, `examples/tabs/`, `examples/split-editors/`, `examples/composable-views/`, `examples/views/`, `examples/doom-fire/`, `examples/spinners/`, `examples/send-msg/`, `examples/chat/`
- [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) — `list`, `viewport`, `textinput`, `textarea`, `spinner`, `progress`, `table`, `paginator`, `cursor`, `filepicker`, `timer`, `stopwatch`, `help`, `key`
- [dlvhdr/gh-dash](https://github.com/dlvhdr/gh-dash) — per-section list pattern
- [jesseduffield/lazygit](https://github.com/jesseduffield/lazygit) — 3-pane layout, undo/redo
- [yorukot/superfile](https://github.com/yorukot/superfile) — multi-pane file-manager pattern

### Current sin-code files read

- `cmd/sin-code/tui.go` (264 lines)
- `cmd/sin-code/tui.doc.md` (71 lines)
- `cmd/sin-code/tui_test.go`, `tui_test.go.bak`
- `go.mod` — confirmed `bubbletea v1.3.10`, `bubbles v1.0.0`, `lipgloss v1.1.0`

### Internal cross-references

- `docs/CODOCS.md` — every new file under `internal/tui/` needs a `.doc.md` companion.
- `AGENTS.md` (project root) — CoDocs standard applies here.
- `docs/adr/ADR-006-gitnexus-mandatory-graph.md` — the design will not depend on GitNexus at runtime; only used in dev/test.

---

## 14. TL;DR — What To Build

1. **Multi-pane Bubbletea TUI** with 5 tabs (Tools / Sessions / EFM / Config / History), 1 left sidebar, 1 optional right panel, 1 footer.
2. **Cyber loader** — rotating half-block halo + binary particle stream — our brand.
3. **5 themes** (Sin-Default, Matrix, Cyberpunk, Amber-Terminal, Red-Alert), cycled with `t`.
4. **3 agents** (Build / Audit / Stats), cycled with `Ctrl+]` / `Ctrl+[`, shows in the footer.
5. **Multi-session execution** — run several tools in parallel via `exec.CommandContext` + `chan string` line bridges.
6. **Command palette** (Ctrl+P) + auto-generated help (`?`).
7. **History persistence** to `~/.local/state/sinator/tui-history.jsonl`.
8. **TTY fallback** preserved from current implementation.
9. **TTY tests with fixed `tea.WithColorProfile` + `tea.WithWindowSize`** for golden-file view tests.
10. **VHS tapes** for the README.

Compile against Bubbletea **v1.3.10** today; migrate to **v2** in a follow-up PR following the official `UPGRADE_GUIDE_V2.md`.

— *End of design.*
