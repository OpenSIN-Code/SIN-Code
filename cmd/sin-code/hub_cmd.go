// SPDX-License-Identifier: MIT
// Purpose: `sin-code hub` — tool catalog subcommand. Lists, searches, and
// prints detailed information about every sin-code command and relevant MCP surface.
// Docs: hub_cmd.doc.md
package main

import (
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hub"
	"github.com/spf13/cobra"
)

// NewHubCmd builds the `hub` cobra subcommand.
func NewHubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Tool catalog and landing page for sin-code",
		Long: `sin-code hub is a read-only catalog of all 36+ subcommands and
relevant MCP skill surfaces. Use it to discover commands, search by keyword,
or show detailed info for a specific tool. The catalog is static and mirrors
the command surface; dynamic MCP servers are listed via 'sin-code mcp status'.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("SIN-Code Tool Catalog")
			fmt.Println(strings.Repeat("═", 60))
			fmt.Print(hub.FormatCategories(hub.DefaultCatalog()))
			return nil
		},
	}

	cmd.AddCommand(newHubListCmd())
	cmd.AddCommand(newHubSearchCmd())
	cmd.AddCommand(newHubInfoCmd())
	return cmd
}

func newHubListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Flat list of all tools",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Print(hub.FormatList(hub.AllTools()))
			return nil
		},
	}
}

func newHubSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <keyword>",
		Short: "Search tools by name, short, or description",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			res := hub.Search(args[0])
			if len(res) == 0 {
				fmt.Println("No tools matched.")
				return nil
			}
			fmt.Printf("Matched %d tool(s):\n\n", len(res))
			fmt.Print(hub.FormatList(res))
			return nil
		},
	}
}

func newHubInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <tool>",
		Short: "Show detailed info for a single tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := strings.ToLower(args[0])
			for _, t := range hub.AllTools() {
				if strings.EqualFold(t.Name, name) {
					fmt.Print(hub.FormatDetail(t))
					return nil
				}
			}
			return fmt.Errorf("unknown tool: %q (run 'sin-code hub list')", args[0])
		},
	}
}
