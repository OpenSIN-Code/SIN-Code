// SPDX-License-Identifier: MIT
// Purpose: `sin-code mcp` — inspect and debug external MCP servers
// (mandate C5): list effective configs, show live connection status and
// discovered tools, and invoke a single tool for smoke testing.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/spf13/cobra"
)

func NewMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Inspect and debug external MCP servers",
	}

	var jsonOut bool
	var timeout time.Duration

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List effective server configs (defaults + user + workspace merge)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := os.Getwd()
			if err != nil {
				return err
			}
			cfgs := mcpclient.LoadConfigs(ws)
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(cfgs)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-8s %s\n", "NAME", "TYPE", "TARGET")
			for _, c := range cfgs {
				target := c.URL
				if c.Transport == "stdio" {
					target = c.Command
					for _, a := range c.Args {
						target += " " + a
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-8s %s\n", c.Name, c.Transport, target)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Connect to all servers and report reachability + tool counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := os.Getwd()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			mgr := mcpclient.NewManager(mcpclient.LoadConfigs(ws))
			if err := mgr.ConnectAll(ctx); err != nil {
				return err
			}
			defer mgr.Close()

			byServer := map[string]int{}
			for _, t := range mgr.Tools() {
				byServer[t.Server]++
			}
			type row struct {
				Name  string `json:"name"`
				Up    bool   `json:"up"`
				Tools int    `json:"tools"`
			}
			var rows []row
			for _, c := range mcpclient.LoadConfigs(ws) {
				n := byServer[c.Name]
				rows = append(rows, row{Name: c.Name, Up: n > 0, Tools: n})
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-6s %s\n", "NAME", "UP", "TOOLS")
			for _, r := range rows {
				up := "no"
				if r.Up {
					up = "yes"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-6s %d\n", r.Name, up, r.Tools)
			}
			return nil
		},
	}
	statusCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	statusCmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "connect timeout")

	callCmd := &cobra.Command{
		Use:   "call <server__tool> [json-args]",
		Short: "Invoke a single external tool for smoke testing",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if len(args) == 2 {
				if err := json.Unmarshal([]byte(args[1]), &toolArgs); err != nil {
					return fmt.Errorf("args must be a JSON object: %w", err)
				}
			}
			ws, err := os.Getwd()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			mgr := mcpclient.NewManager(mcpclient.LoadConfigs(ws))
			if err := mgr.ConnectAll(ctx); err != nil {
				return err
			}
			defer mgr.Close()
			out, err := mgr.Call(ctx, args[0], toolArgs)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	callCmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "total timeout")

	cmd.AddCommand(listCmd, statusCmd, callCmd)
	return cmd
}
