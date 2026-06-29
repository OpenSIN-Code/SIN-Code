package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/logger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui"
)

var (
	tuiPort     int
	tuiHostname string
	tuiExternal bool
	tuiMDNS     bool
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
	logger.SetLevel(logger.LevelError)

	if ov := loadTUIKeyOverrides(); ov != nil {
		km := tui.DefaultKeymap()
		km.ApplyOverrides(*ov)
		tui.SetKeymap(km)
	}

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
	opts := tui.ProgramOptions{
		ExternalMode:  tuiExternal,
		Port:          tuiPort,
		Hostname:      tuiHostname,
		MDNS:          tuiMDNS,
		Sigusr2Reload: true,
	}
	guard := tui.SetupPlatformGuard()
	defer guard.Cleanup()
	if err := tui.RunProgram(pm, opts); err != nil {
		if strings.Contains(err.Error(), "no TTY") {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return
		}
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

func loadTUIKeyOverrides() *tui.KeyOverrides {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, ".config", "sin-code", "tui-keys.json")
	ov, err := tui.LoadKeyOverrides(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("tui: invalid key overrides %s: %v", path, err)
		}
		return nil
	}
	log.Printf("tui: loaded %d key override(s) from %s", countKeyOverrides(ov), path)
	return &ov
}

func countKeyOverrides(ov tui.KeyOverrides) int {
	n := 0
	for _, s := range [][]string{
		ov.Quit, ov.Help, ov.Palette, ov.ToggleSidebar, ov.CycleTheme, ov.CycleAgent, ov.Interrupt,
		ov.NextView, ov.PrevView, ov.ViewTools, ov.ViewSessions, ov.ViewEFM, ov.ViewConfig, ov.ViewHistory, ov.ViewTodos, ov.ViewChat,
		ov.RunTool, ov.ShowHelp, ov.ToolUp, ov.ToolDown,
		ov.Submit, ov.Cancel, ov.Search, ov.CopyMessage, ov.ScrollUp, ov.ScrollDown,
		ov.NewSession, ov.CloseSession, ov.SessionSwitch,
		ov.ModelSelect, ov.Subagents,
	} {
		if len(s) > 0 {
			n++
		}
	}
	return n
}

func init() {
	tuiCmd.Flags().IntVarP(&tuiPort, "port", "p", 0, "Port for external TUI mode")
	tuiCmd.Flags().StringVar(&tuiHostname, "hostname", "localhost", "Hostname for external TUI")
	tuiCmd.Flags().BoolVar(&tuiExternal, "external", false, "Run TUI in external mode (browser)")
	tuiCmd.Flags().BoolVar(&tuiMDNS, "mdns", false, "Advertise via mDNS (experimental)")
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
  Enter             send message (in chat view)
  Shift+Enter       insert newline (in chat view)
  Tab / Shift+Tab   switch view
  1-7               jump to view
  ctrl+b            toggle sidebar
  ctrl+p            command palette
  ctrl+m            switch model
  ctrl+g            switch session
  ctrl+a            subagents popup
  t                 cycle theme
  a                 cycle agent (Build/Audit/Stats)
  r                 run selected tool
  q / ctrl+c / ctrl+x  quit
  Esc               interrupt

External mode:
  --external --port 8080   serve the TUI in a browser via SSE
  --hostname 0.0.0.0       bind to a public interface
  Send SIGUSR2 to the process to hot-reload config and theme.

If no TTY is available, an error is printed. Use 'sin-code chat -p' for headless mode.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		runNewTUI(cmd.OutOrStdout())
		return nil
	},
}
