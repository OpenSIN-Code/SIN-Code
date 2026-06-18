// SPDX-License-Identifier: MIT
// Purpose: `sin-code tool-search` — debug command for the lazy tool loader's
// relevance ranking. Searches the effective tool catalog (built-in + reachable
// MCP servers) and prints the top-k results using the same offline semantic
// index used by `sin-code chat --lazy-tools --semantic-tools` (issue #364).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
)

func NewToolSearchCmd() *cobra.Command {
	var (
		k       int
		jsonOut bool
		keyword bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "tool-search <query>",
		Short: "Debug tool relevance ranking",
		Long: `Search the effective tool catalog (built-in + reachable MCP servers) and
print the top-k relevant tools. By default the offline semantic index is used;
use --keyword to compare the legacy keyword index.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			ws, err := mcpHookVars.getwd()
			if err != nil {
				return err
			}

			mgr := mcpHookVars.newManager(mcpHookVars.loadConfigs(ws))
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			_ = mgr.ConnectAll(ctx)
			defer mgr.Close()

			specs := toolSearchSpecs(mgr.Tools())
			var results []mcpclient.ToolSpec
			if keyword {
				loader := mcpclient.NewLazyToolLoader(specs)
				results = loader.Search(query, k)
			} else {
				idx := mcpclient.NewSemanticIndex(specs)
				results = idx.Search(query, k)
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no matching tools")
				return nil
			}
			for _, s := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "%s - %s\n", s.Name, s.Description)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&k, "k", 10, "maximum number of results")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	cmd.Flags().BoolVar(&keyword, "keyword", false, "use keyword retrieval instead of semantic")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "MCP connection timeout")
	return cmd
}

// toolSearchSpecs merges built-in specs with the external tools reported by the
// MCP manager into the LazyToolLoader's internal ToolSpec representation.
func toolSearchSpecs(external []mcpclient.Tool) []mcpclient.ToolSpec {
	specs := make([]mcpclient.ToolSpec, 0, len(builtinSpecs())+len(external))
	for _, s := range builtinSpecs() {
		specs = append(specs, mcpclient.ToolSpec{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: s.InputSchema,
		})
	}
	for _, t := range external {
		desc := t.Description
		if desc == "" {
			desc = fmt.Sprintf("External MCP tool %s on server %s", t.Name, t.Server)
		}
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		specs = append(specs, mcpclient.ToolSpec{
			Name:        t.Qualified,
			Description: desc,
			InputSchema: schema,
		})
	}
	return specs
}
