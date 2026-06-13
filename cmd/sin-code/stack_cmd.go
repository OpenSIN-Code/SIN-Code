// SPDX-License-Identifier: MIT
// Purpose: `sin-code stack` — one-shot install/doctor for the 3-layer
// methodology stack (DOX context + Superpowers methodology + Vane
// research sidecar) on top of the SIN-Code tool layer.
// Docs: stack.doc.md
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/stack"
	"github.com/spf13/cobra"
)

// NewStackCmd builds the `stack` cobra subcommand. Pattern matches
// NewVaneCmd: returns *cobra.Command with install + doctor attached.
func NewStackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stack",
		Short: "Manage the DOX + Superpowers + Vane methodology stack",
		Long: `sin-code stack is the one-shot manager for the methodology stack:

  Layer 1 (context):    DOX         — hierarchical AGENTS.md trees
  Layer 2 (methodology): Superpowers — mandatory workflows (TDD, debugging)
  Layer 3 (tools):      SIN-Code    — verified sin_* MCP tools
  Sidecar (research):   Vane        — citation-backed answering engine

'install' sets up all layers (idempotent). 'doctor' validates them
without modifying anything. Graceful degradation: a missing or down
optional layer is reported, never fatal.`,
	}

	cmd.AddCommand(newStackInstallCmd())
	cmd.AddCommand(newStackDoctorCmd())
	return cmd
}

func newStackInstallCmd() *cobra.Command {
	var (
		skipSuperpowers bool
		skipDox         bool
		skipVane        bool
		agentsMDPath    string
		vaneURL         string
		jsonOut         bool
	)
	c := &cobra.Command{
		Use:   "install",
		Short: "Install/refresh all stack layers (idempotent)",
		RunE: func(_ *cobra.Command, _ []string) error {
			opts := stack.InstallOptions{
				SkipSuperpowers: skipSuperpowers,
				SkipDox:         skipDox,
				SkipVane:        skipVane,
				AgentsMDPath:    agentsMDPath,
				VaneURL:         vaneURL,
			}
			report := stack.Install(opts)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			fmt.Print(stack.Format(report))
			if !report.AllOK {
				return fmt.Errorf("stack install completed with errors")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&skipSuperpowers, "skip-superpowers", false, "Do not install Superpowers")
	c.Flags().BoolVar(&skipDox, "skip-dox", false, "Do not inject the DOX protocol")
	c.Flags().BoolVar(&skipVane, "skip-vane", false, "Do not configure the Vane bridge")
	c.Flags().StringVar(&agentsMDPath, "agents-md", "", "Target AGENTS.md (default: ./AGENTS.md)")
	c.Flags().StringVar(&vaneURL, "vane-url", "", "Vane base URL (default: existing config)")
	c.Flags().BoolVar(&jsonOut, "json", false, "Machine-readable report")
	return c
}

func newStackDoctorCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "doctor [root]",
		Short: "Validate all stack layers without modifying anything",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			report := stack.Doctor(root)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			fmt.Print(stack.Format(report))
			if !report.AllOK {
				return fmt.Errorf("stack doctor found problems")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "Machine-readable report")
	return c
}
