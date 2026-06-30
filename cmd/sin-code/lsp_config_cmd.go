// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lspconfig"
	"github.com/spf13/cobra"
)

func newLSPConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lsp-config",
		Short: "Auto-detect and configure LSP servers for the LLM (issue #492)",
	}
	cmd.AddCommand(newLSPDetectCmd())
	cmd.AddCommand(newLSPConfigGenCmd())
	return cmd
}

func newLSPDetectCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Scan system for available LSP servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			detector := lspconfig.NewDetector()
			servers := detector.Detect()
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(servers)
			}
			if len(servers) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No LSP servers found on PATH.")
				fmt.Fprintln(cmd.OutOrStdout(), "Install one: go install golang.org/x/tools/gopls@latest")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-30s %-15s %-20s %s\n", "NAME", "LANGUAGE", "VERSION", "PATH")
			for _, s := range servers {
				ver := s.Version
				if len(ver) > 20 {
					ver = ver[:20] + "..."
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-30s %-15s %-20s %s\n", s.Name, s.Language, ver, s.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newLSPConfigGenCmd() *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Generate and write LSP configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			detector := lspconfig.NewDetector()
			servers := detector.Detect()
			if len(servers) == 0 {
				return fmt.Errorf("no LSP servers detected")
			}
			config := lspconfig.GenerateConfig(servers)
			if outPath == "" {
				home, _ := os.UserHomeDir()
				outPath = filepath.Join(home, ".config", "sin-code", "lsp.json")
			}
			if err := lspconfig.WriteConfig(config, outPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "LSP config written to %s (%d servers)\n", outPath, len(config.Servers))
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "Output file path")
	return cmd
}
