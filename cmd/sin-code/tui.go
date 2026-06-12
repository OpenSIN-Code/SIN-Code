package main

import (
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui"
)

func getSubcommand(name string) *cobra.Command {
	for _, c := range rootCmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func runNewTUI(out io.Writer) {
	pm := tui.NewModel()
	pm.OnRun = func(name string, args []string) error {
		c := getSubcommand(name)
		if c == nil {
			return fmt.Errorf("unknown subcommand: %s", name)
		}
		c.SetArgs(args)
		c.SetOut(out)
		c.SetErr(out)
		return c.Execute()
	}
	prog := tea.NewProgram(pm)
	pm.Program = tui.ProgramFromTeaProgram(prog)
	if _, err := prog.Run(); err != nil {
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
	}
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive multi-pane TUI (Tools, Sessions, EFM, Config, History)",
	Long: `Launch the interactive multi-pane TUI for sin-code.

Panes:
  - Top tab bar: multi-session tabs (press + to add)
  - Left sidebar: views (collapsible with ctrl+b)
  - Center: Tools / Sessions / EFM / Config / History
  - Right: tool details (Tools view)
  - Bottom: footer with agent + tokens + cost + hints

Keys:
  Tab / Shift+Tab   switch view
  1-5               jump to view
  ctrl+b            toggle sidebar
  ctrl+p            command palette
  ctrl+x            subagents popup
  t                 cycle theme
  a                 cycle agent (Build/Audit/Stats)
  r                 run selected tool
  Enter             show --help for selected tool
  q / ctrl+c        quit
  Esc               interrupt

If no TTY is available, a plain text catalog is printed instead.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		runNewTUI(cmd.OutOrStdout())
		return nil
	},
}
