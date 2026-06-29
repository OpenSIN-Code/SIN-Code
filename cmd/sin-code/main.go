// SPDX-License-Identifier: MIT
// Purpose: sin-code — unified Go binary for all SIN-Code analysis/manipulation tools.
// Replaces 13 separate binaries (discover, execute, map, grasp, scout, harvest,
// orchestrate, ibd, poc, sckg, adw, oracle, efm) with a single cobra-based CLI.
// Docs: cmd/sin-code/main.go.doc.md
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sandbox"
)

var rootCmd = &cobra.Command{
	Use:   "sin-code",
	Short: "SIN-Code — verification-first coding agent",
	Long: `sin-code — verification-first coding agent

Every completed task passes a verification gate (PoC/Oracle proof) before reporting success.
54+ MCP tools, multi-agent orchestration, session management, and a beautiful TUI.`,
	Version: Version,
	// Args allows bare invocation with no subcommand; RunE is set
	// in main() to avoid a package-init cycle (rootCmd → runChat →
	// runChatTUI → getSubcommand → rootCmd). SilenceUsage/Errors
	// prevents cobra double-printing when RunE returns an error;
	// PrintError in main() is the single error sink.
	Args:           cobra.ArbitraryArgs,
	SilenceUsage:   true,
	SilenceErrors:  true,
}

func main() {
	// Sandbox shim: if invoked as the re-exec target (second arg =
	// "__sandbox_exec"), apply Landlock and exec the real command. The
	// parent process stays unconfined; only the child runs sandboxed.
	if len(os.Args) > 2 && os.Args[1] == "__sandbox_exec" {
		if err := sandbox.ApplyAndExec(); err != nil {
			fmt.Fprintf(os.Stderr, "sin-code sandbox: %v\n", err)
			os.Exit(126)
		}
		return // unreachable after successful exec
	}

	// If invoked via a symlink named after a subcommand (e.g. `discover` ->
	// `sin-code discover`), automatically route to that subcommand.
	if len(os.Args) > 0 {
		name := filepath.Base(os.Args[0])
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == name {
				args := append([]string{name}, os.Args[1:]...)
				rootCmd.SetArgs(args)
				break
			}
		}
	}

	checkUpdate()

	// When no subcommand is given (bare `sin-code`), launch chat
	// directly — matching the UX of `claude` → chat. Cobra only
	// calls RunE when no subcommand matches, so `sin-code discover`
	// etc. still route correctly. Set here (not in the rootCmd
	// literal) to avoid a package-init cycle. If args are present,
	// the first arg is an unknown subcommand — return a cobra-style
	// "unknown command" error so PrintError surfaces it cleanly.
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for %q\nRun 'sin-code --help' for usage.", args[0], cmd.CommandPath())
		}
		return runChat(cmd.Context(), &chatOptions{})
	}

	// Install categorized help: save cobra's default first so subcommand
	// help (e.g. `sin-code chat --help`) still uses the standard renderer.
	// Done in init() of help.go so it also works under testscript.RunMain.

	if err := rootCmd.Execute(); err != nil {
		internal.PrintError(err)
	}
}
