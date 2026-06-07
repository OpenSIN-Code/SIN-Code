// SPDX-License-Identifier: MIT
// Purpose: sin-code — unified Go binary for all SIN-Code analysis/manipulation tools.
// Replaces 13 separate binaries (discover, execute, map, grasp, scout, harvest,
// orchestrate, ibd, poc, sckg, adw, oracle, efm) with a single cobra-based CLI.
// Docs: cmd/sin-code/main.go.doc.md
package main

import (
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal"
	"github.com/spf13/cobra"
)

var Version = "dev" // Set at build time via -ldflags "-X main.Version=..."

var rootCmd = &cobra.Command{
	Use:   "sin-code",
	Short: "SIN-Code unified analysis & manipulation toolchain",
	Long: `sin-code is the unified Go binary for the SIN-Code tool suite.
It consolidates 13 specialized tools into a single cobra-based CLI:

  Core analysis:    discover, execute, map, grasp, scout, harvest, orchestrate
  Advanced tools:   ibd, poc, sckg, adw, oracle, efm

Each subcommand is also a thin pass-through to the standalone tool repos
for backwards compatibility — the standalone binaries are still maintained
but "sin-code" is now the primary distribution channel.`,
	Version: Version,
}

func init() {
	rootCmd.AddCommand(internal.DiscoverCmd)
	rootCmd.AddCommand(internal.ExecuteCmd)
	rootCmd.AddCommand(internal.MapCmd)
	rootCmd.AddCommand(internal.GraspCmd)
	rootCmd.AddCommand(internal.ScoutCmd)
	rootCmd.AddCommand(internal.HarvestCmd)
	rootCmd.AddCommand(internal.OrchestrateCmd)
	rootCmd.AddCommand(internal.IbdCmd)
	rootCmd.AddCommand(internal.PocCmd)
	rootCmd.AddCommand(internal.SckgCmd)
	rootCmd.AddCommand(internal.AdwCmd)
	rootCmd.AddCommand(internal.OracleCmd)
	rootCmd.AddCommand(internal.EfmCmd)
	rootCmd.AddCommand(internal.ServeCmd)
}

func main() {
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
	if err := rootCmd.Execute(); err != nil {
		internal.PrintError(err)
	}
}
