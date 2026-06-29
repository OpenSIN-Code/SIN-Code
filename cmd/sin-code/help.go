// SPDX-License-Identifier: MIT
// Purpose: Custom categorized help for `sin-code --help`.
// Replaces the flat 90+ command dump with grouped, scannable output.
// Subcommand help (e.g. `sin-code chat --help`) delegates to cobra's
// default renderer so per-command flags and examples are preserved.
package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// helpCategories defines the command groupings shown in root help output.
// Commands not listed here appear under "Other".
var helpCategories = []struct {
	Title    string
	Commands []string
}{
	{
		Title:    "Chat & Agent",
		Commands: []string{"chat", "sessions", "mcp", "tool-search", "goal", "daemon", "skill", "skills", "swarm"},
	},
	{
		Title:    "Analysis & Verification",
		Commands: []string{"discover", "execute", "map", "grasp", "scout", "harvest", "orchestrate", "ibd", "poc", "sckg", "adw", "oracle", "efm"},
	},
	{
		Title:    "Code Quality",
		Commands: []string{"audit", "ceo-audit", "review", "debt", "compress", "diff", "benchmark", "cover"},
	},
	{
		Title:    "Security",
		Commands: []string{"security", "sbom", "tokens", "permission"},
	},
	{
		Title:    "Configuration & Setup",
		Commands: []string{"config", "doctor", "status", "stack", "install", "update", "self-update", "profile"},
	},
	{
		Title:    "Ecosystem",
		Commands: []string{"hub", "ledger", "summary", "catalog", "gh", "vane", "superpowers", "dox", "fusion", "research", "headroom", "autodev"},
	},
	{
		Title:    "Frontend",
		Commands: []string{"serve", "tui", "webui"},
	},
	{
		Title:    "Lifecycle & Memory",
		Commands: []string{"todo", "notifications", "memory", "assets", "hooks", "eval", "trace", "evalset", "instinct", "prp"},
	},
	{
		Title:    "Utility",
		Commands: []string{"read", "write", "edit", "lsp", "plugin", "index", "rules", "checkpoint", "rewind", "image-graph", "analyse", "analyse-image", "auto"},
	},
	{
		Title:    "Advanced",
		Commands: []string{"codegraph", "spec", "triage", "grill", "subagent", "autopr", "compile-spec", "rtk", "orchestrator-run", "orchestrator-agents", "orchestrator-plan"},
	},
}

// defaultRootHelp is the original cobra help function, captured before the
// custom func is installed. Used to render subcommand help (e.g.
// `sin-code chat --help`) with cobra's default template.
var defaultRootHelp func(*cobra.Command, []string)

func init() {
	// Install categorized help in init() so it is active in every entry
	// point — both main() and testscript.RunMain (which calls
	// rootCmd.Execute() directly, bypassing main()).
	defaultRootHelp = rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(customHelpFunc)
}

// customHelpFunc prints categorized help for the root command and delegates
// to cobra's default help for subcommands.
func customHelpFunc(cmd *cobra.Command, args []string) {
	// Subcommands get cobra's default help (flags, examples, sub-subcommands).
	if cmd != rootCmd {
		if defaultRootHelp != nil {
			defaultRootHelp(cmd, args)
		}
		return
	}

	out := cmd.OutOrStdout()

	// Header
	fmt.Fprintf(out, "SIN-Code — verification-first coding agent\n\n")
	fmt.Fprintf(out, "Every completed task passes a verification gate (PoC/Oracle proof)\n")
	fmt.Fprintf(out, "before reporting success. 54+ MCP tools, multi-agent orchestration,\n")
	fmt.Fprintf(out, "session management, and a beautiful TUI.\n\n")

	// Usage
	fmt.Fprintf(out, "Usage:\n")
	fmt.Fprintf(out, "  sin-code [command]\n")
	fmt.Fprintf(out, "  sin-code [flags]                  Launch chat (interactive TUI or REPL)\n")
	fmt.Fprintf(out, "  sin-code chat -p \"prompt\"         Headless one-shot\n")
	fmt.Fprintf(out, "  sin-code chat -p \"prompt\" --json  JSON output\n\n")

	// Categorized commands
	allCmds := cmd.Commands()
	shown := make(map[string]bool)

	for _, cat := range helpCategories {
		var cmds []*cobra.Command
		for _, name := range cat.Commands {
			for _, c := range allCmds {
				if c.Name() == name && c.IsAvailableCommand() {
					cmds = append(cmds, c)
					shown[name] = true
					break
				}
			}
		}
		if len(cmds) == 0 {
			continue
		}
		fmt.Fprintf(out, "%s:\n", cat.Title)
		for _, c := range cmds {
			fmt.Fprintf(out, "  %-20s %s\n", c.Name(), c.Short)
		}
		fmt.Fprintln(out)
	}

	// Uncategorized commands (e.g. cobra auto-generated completion/help)
	var other []*cobra.Command
	for _, c := range allCmds {
		if c.IsAvailableCommand() && !shown[c.Name()] {
			other = append(other, c)
		}
	}
	if len(other) > 0 {
		fmt.Fprintf(out, "Other:\n")
		for _, c := range other {
			fmt.Fprintf(out, "  %-20s %s\n", c.Name(), c.Short)
		}
		fmt.Fprintln(out)
	}

	// Flags
	localFlags := cmd.LocalFlags()
	if localFlags != nil && localFlags.HasFlags() {
		usages := strings.TrimRight(localFlags.FlagUsages(), " \t\n")
		if usages != "" {
			fmt.Fprintf(out, "Flags:\n%s\n\n", usages)
		}
	}

	// Footer
	fmt.Fprintf(out, "Run 'sin-code <command> --help' for command-specific help.\n")
	fmt.Fprintf(out, "Run 'sin-code' (no args) to start chatting.\n")
}
