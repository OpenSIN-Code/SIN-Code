// SPDX-License-Identifier: MIT
// Purpose: `sin-code subagent` — operator-facing CLI for the
// isolated-context sub-agent (issue #192). Thin wrapper around
// agentloop.Loop.SpawnSubagent (issue #153).
//
// The sub-agent runs in its own SQLite session, with its own
// context window, and returns a compact summary. The parent's
// shell never sees the child's message history.
//
// Docs: cmd/sin-code/internal/agentloop/subagent.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// NewSubagentCmd builds the `subagent` cobra subcommand.
func NewSubagentCmd() *cobra.Command {
	var (
		workspace string
		maxTurns  int
		maxTokens int
		jsonOut   bool
	)
	cmd := &cobra.Command{
		Use:   "subagent <goal>",
		Short: "Run a subtask in an isolated session, return summary (issue #192)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			wd := workspace
			if wd == "" {
				wd, _ = os.Getwd()
			}
			store, err := session.Open(sessionPathFor(wd))
			if err != nil {
				return fmt.Errorf("open session store: %w", err)
			}
			defer store.Close()
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			// Minimal parent loop: nil gate (trust the Completion),
			// nil hooks (no audit for the shell call). The sub-agent
			// gets a fresh session in this store via SpawnSubagent.
			parent := &agentloop.Loop{
				Gate: verify.NewGate("poc",
					func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
					nil),
			}
			result, err := parent.SpawnSubagent(ctx, store, agentloop.SubagentRequest{
				Goal:      args[0],
				MaxTurns:  maxTurns,
				MaxTokens: maxTokens,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			fmt.Fprintf(c.OutOrStdout(), "subagent summary: %s\n", result.Summary)
			fmt.Fprintf(c.OutOrStdout(), "verified: %v\n", result.Verified)
			fmt.Fprintf(c.OutOrStdout(), "turns: %d\n", result.Turns)
			if len(result.OpenCriteria) > 0 {
				fmt.Fprintln(c.OutOrStdout(), "open criteria:")
				for _, oc := range result.OpenCriteria {
					fmt.Fprintf(c.OutOrStdout(), "  - %s\n", oc)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace path (default: $PWD)")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 0, "per-subagent turn cap (0 = default)")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", 0, "per-subagent token cap (0 = default)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

// sessionPathFor returns the default session db path for workspace.
// In v0 this is a single file at <workspace>/.sin/sessions.db; v1
// will honor $SIN_CODE_HOME per the AGENTS.md convention.
func sessionPathFor(workspace string) string {
	return workspace + "/.sin/sessions.db"
}
