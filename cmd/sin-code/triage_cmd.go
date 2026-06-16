// SPDX-License-Identifier: MIT
// Purpose: `sin-code triage` — read the open issue backlog via gh,
// score, group, and render. The default format is text; --format=md
// writes the markdown that lands in BACKLOG.md. See issue #162.
//
// Docs: cmd/sin-code/internal/triage/triage.doc.md
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/triage"
)

// NewTriageCmd builds the `triage` cobra subcommand.
func NewTriageCmd() *cobra.Command {
	var (
		format string
		repo   string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Read the open issue backlog via gh, score, group, and render",
		Long: "Reads the open issue backlog via `gh issue list`, scores each\n" +
			"issue by a heuristic (epic label, blocks count, acceptance\n" +
			"section, staleness, etc.), and renders a prioritized view.\n" +
			"The default format is text; --format=md writes the canonical\n" +
			"BACKLOG.md; --format=json is the machine-readable envelope.\n" +
			"Docs: cmd/sin-code/internal/triage/triage.doc.md",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*1_000_000_000) // 60s
			defer cancel()

			issues, err := triage.Loader(ctx, repo)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}
			if limit > 0 && len(issues) > limit {
				issues = issues[:limit]
			}

			list := triage.ScoreAll(issues, time.Now().UTC())
			return triage.Render(cmd.OutOrStdout(), list, triage.Format(format))
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text|md|json")
	cmd.Flags().StringVar(&repo, "repo", "", "owner/repo (defaults to the current repo)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max issues to render (0 = no cap)")
	return cmd
}
