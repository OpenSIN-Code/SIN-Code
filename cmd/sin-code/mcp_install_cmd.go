// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpinstall"
	"github.com/spf13/cobra"
)

func newMCPInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-install",
		Short: "Discover and install MCP servers (issue #490)",
	}
	cmd.AddCommand(newMCPDiscoverCmd())
	cmd.AddCommand(newMCPServerInstallCmd())
	cmd.AddCommand(newMCPServerUninstallCmd())
	return cmd
}

func newMCPDiscoverCmd() *cobra.Command {
	var category string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "List available MCP servers from the registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := mcpinstall.NewRegistry()
			servers := registry.List()
			if category != "" {
				var filtered []mcpinstall.MCPServerInfo
				for _, s := range servers {
					if s.Category == category {
						filtered = append(filtered, s)
					}
				}
				servers = filtered
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(servers)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %s\n", "NAME", "CATEGORY", "DESCRIPTION")
			for _, s := range servers {
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %s\n", s.Name, s.Category, s.Description)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "Filter by category")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newMCPServerInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <name>",
		Short: "Install an MCP server from the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := mcpinstall.NewRegistry()
			info, ok := registry.Get(args[0])
			if !ok {
				return fmt.Errorf("MCP server %q not found in registry. Use 'sin-code mcp-install discover' to list available servers.", args[0])
			}
			installer, err := mcpinstall.NewInstaller()
			if err != nil {
				return err
			}
			if err := installer.Install(info); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed MCP server: %s (%s)\n", info.Name, info.DisplayName)
			return nil
		},
	}
}

func newMCPServerUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove an MCP server from the config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			installer, err := mcpinstall.NewInstaller()
			if err != nil {
				return err
			}
			if err := installer.Uninstall(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Uninstalled MCP server: %s\n", args[0])
			return nil
		},
	}
}
