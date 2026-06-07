// SPDX-License-Identifier: MIT
// Purpose: tui — interactive Bubbletea menu for all sin-code subcommands.
// When a TTY is available, shows a searchable list; otherwise falls back to
// a plain text catalog so scripts and CI don't crash.
// Docs: tui.doc.md
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// tuiItem is one row in the interactive menu.
type tuiItem struct {
	name        string
	description string
	helpText    string
}

func (t tuiItem) Title() string       { return t.name }
func (t tuiItem) Description() string { return t.description }
func (t tuiItem) FilterValue() string { return t.name + " " + t.description }

// tuiModel is the Bubbletea model for the sin-code TUI.
type tuiModel struct {
	list     list.Model
	quitting bool
}

func newTUIModel() tuiModel {
	items := []list.Item{
		tuiItem{name: "discover", description: "Discover files with relevance scoring", helpText: "sin-code discover <path> [--pattern ...]"},
		tuiItem{name: "execute", description: "Safe shell execution with redaction", helpText: "sin-code execute <command> [--timeout ...]"},
		tuiItem{name: "map", description: "Architecture map + dependency graph", helpText: "sin-code map <path> [--action map]"},
		tuiItem{name: "grasp", description: "Deep single-file analysis", helpText: "sin-code grasp <path> [--format json]"},
		tuiItem{name: "scout", description: "Regex/semantic/symbol code search", helpText: "sin-code scout <query> [--search_type regex]"},
		tuiItem{name: "harvest", description: "URL fetch + cache + structure extract", helpText: "sin-code harvest <url> [--method GET]"},
		tuiItem{name: "orchestrate", description: "Task management with dependencies", helpText: "sin-code orchestrate [--action list]"},
		tuiItem{name: "ibd", description: "Intent-based diffing", helpText: "sin-code ibd [--before <path> --after <path>]"},
		tuiItem{name: "poc", description: "Proof-of-correctness verification", helpText: "sin-code poc [--spec <path> --code <path>]"},
		tuiItem{name: "sckg", description: "Semantic codebase knowledge graph", helpText: "sin-code sckg <path> [--action build]"},
		tuiItem{name: "adw", description: "Architectural debt watchdogs", helpText: "sin-code adw <path> [--strict]"},
		tuiItem{name: "oracle", description: "Verification oracle", helpText: "sin-code oracle [--claim <text>]"},
		tuiItem{name: "efm", description: "Ephemeral full-stack mocking", helpText: "sin-code efm [--action list]"},
		tuiItem{name: "serve", description: "Start MCP server (stdio)", helpText: "sin-code serve [--transport stdio]"},
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "⚡ sin-code — choose a tool (↑/↓ to navigate, Enter to run --help, q to quit)"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))
	return tuiModel{list: l}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

// runnableWithoutArgs lists commands that can be executed without arguments.
var runnableWithoutArgs = map[string]bool{
	"serve":       true, // starts MCP server with defaults
	"orchestrate": true, // lists tasks by default
	"tui":         true, // recursive, but technically valid
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "enter" {
			if i, ok := m.list.SelectedItem().(tuiItem); ok {
				// Show --help for the selected subcommand
				cmd := getSubcommand(i.name)
				if cmd != nil {
					cmd.SetArgs([]string{"--help"})
					cmd.SetOut(os.Stdout)
					_ = cmd.Execute()
				}
				m.quitting = true
				return m, tea.Quit
			}
		}
		if msg.String() == "r" {
			if i, ok := m.list.SelectedItem().(tuiItem); ok {
				if runnableWithoutArgs[i.name] {
					cmd := getSubcommand(i.name)
					if cmd != nil {
						cmd.SetArgs([]string{})
						cmd.SetOut(os.Stdout)
						_ = cmd.Execute()
					}
					m.quitting = true
					return m, tea.Quit
				}
			}
		}
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}
	var hint string
	if i, ok := m.list.SelectedItem().(tuiItem); ok && runnableWithoutArgs[i.name] {
		hint = "Enter: show --help, r: run without args, q: quit"
	} else {
		hint = "Enter: show --help, q: quit"
	}
	return m.list.View() + "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(hint)
}

// getSubcommand returns the cobra.Command for a given subcommand name.
func getSubcommand(name string) *cobra.Command {
	for _, c := range rootCmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI menu for all sin-code subcommands",
	Long: `Launch an interactive Bubbletea menu that lists every sin-code subcommand.

Controls:
  ↑/↓ or k/j    Navigate the list
  /             Filter/search
  Enter         Show --help for the selected command
  r             Run the selected command without arguments (if supported)
  q or Ctrl+C   Quit

Commands that support 'r' (run without args): serve, orchestrate

If no TTY is detected, a plain text catalog is printed instead.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		// Try to run the interactive TUI; if TTY is unavailable, fall back to plain text.
		p := tea.NewProgram(newTUIModel(), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			// TTY not available — print plain text catalog instead of crashing.
			fmt.Fprintln(out, "sin-code subcommands (TUI not available, showing plain text):")
			fmt.Fprintln(out)
			for _, c := range rootCmd.Commands() {
				if c.Name() == "tui" || c.Name() == "help" {
					continue
				}
				fmt.Fprintf(out, "  %-14s  %s\n", c.Name(), c.Short)
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Run `sin-code <command> --help` for details.")
			return nil
		}
		return nil
	},
}


