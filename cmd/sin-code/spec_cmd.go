// SPDX-License-Identifier: MIT
// Purpose: `sin-code spec` — the Spec-Layer CLI (issue #122). It parses,
// validates, and renders *.spec.md files that capture the contract a change
// must satisfy, bridging human intent and machine-checkable verification.
// Docs: docs/spec-layer.md
package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec"
)

// NewSpecCmd builds the `spec` cobra subcommand (validate + show).
func NewSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Author, validate & inspect *.spec.md contracts (Spec-Layer)",
		Long: `sin-code spec is the Spec-Layer: a *.spec.md file captures the contract a
change must satisfy — Objective, Requirements, Acceptance Criteria (with
optional verify commands), and hard Invariants. It is the bridge between
human intent and machine-checkable verification consumed by the agent and
autopilot.

  sin-code spec validate feature.spec.md     # structural check, non-zero on error
  sin-code spec show feature.spec.md          # parsed summary
  sin-code spec show --json feature.spec.md   # parsed spec as JSON`,
	}
	cmd.AddCommand(newSpecValidateCmd())
	cmd.AddCommand(newSpecShowCmd())
	return cmd
}

func newSpecValidateCmd() *cobra.Command {
	var quiet bool
	c := &cobra.Command{
		Use:   "validate <file.spec.md>",
		Short: "Validate a spec file for structural completeness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := spec.Load(args[0])
			if err != nil {
				return err
			}
			res := spec.Validate(s)
			out := cmd.OutOrStdout()
			if !quiet {
				for _, iss := range res.Issues {
					fmt.Fprintln(out, iss.String())
				}
			}
			if !res.OK() {
				return fmt.Errorf("spec %s: %d error(s)", args[0], len(res.Errors()))
			}
			if !quiet {
				fmt.Fprintf(out, "spec %s: OK (%d requirements, %d criteria)\n",
					args[0], len(s.Requirements), len(s.Criteria))
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress output; rely on exit code")
	return c
}

func newSpecShowCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "show <file.spec.md>",
		Short: "Print a parsed spec (summary or --json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := spec.Load(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(s)
			}
			title := s.Title
			if title == "" {
				title = "(untitled spec)"
			}
			fmt.Fprintf(out, "%s\n", title)
			fmt.Fprintf(out, "  objective:    %s\n", firstLine(s.Objective))
			fmt.Fprintf(out, "  requirements: %d\n", len(s.Requirements))
			for _, r := range s.Requirements {
				fmt.Fprintf(out, "    %s [%s] %s\n", r.ID, r.Priority, r.Text)
			}
			fmt.Fprintf(out, "  criteria:     %d\n", len(s.Criteria))
			for _, cr := range s.Criteria {
				if cr.Verify != "" {
					fmt.Fprintf(out, "    %s %s  (verify: %s)\n", cr.ID, cr.Text, cr.Verify)
				} else {
					fmt.Fprintf(out, "    %s %s\n", cr.ID, cr.Text)
				}
			}
			if len(s.Invariants) > 0 {
				fmt.Fprintf(out, "  invariants:   %d\n", len(s.Invariants))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit the parsed spec as JSON")
	return c
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	if s == "" {
		return "(none)"
	}
	return s
}
