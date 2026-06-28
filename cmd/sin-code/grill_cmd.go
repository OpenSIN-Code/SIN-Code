// SPDX-License-Identifier: MIT
// Code extracted from commands.go — Grill section.

package main

// sin-debt: shrink, upgrade: when a second grill-related command is added, merge into a shared file

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/grill"
)

// ============================================================================
// Grill command (sin-code grill)
// ============================================================================

// grillDir returns the directory for grilling sessions. Honors
// SIN_CODE_HOME, then XDG_DATA_HOME, then ~/.local/share/sin-code/grill.
func grillDir() (string, error) {
	if v := os.Getenv("SIN_CODE_HOME"); v != "" {
		return filepath.Join(v, "grill"), nil
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "sin-code", "grill"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "sin-code", "grill"), nil
}

// NewGrillCmd builds the `grill` cobra subcommand.
func NewGrillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grill",
		Short: "Native adversarial design-review interview (issue #141 fusion)",
		Long: `sin-code grill is the native Go implementation of the
external SIN-Code-Grill-Me-Skill Python MCP server. Use it to
stress-test a plan, design, or decision before building it.

Subcommands:
  start <topic>            begin a grilling session, print the session id
  next <id>                ask the next adversarial question
  answer <id> <d-id> <text>  record the operator's response
                            (use "done" to resolve a decision)
  status <id>              show resolved + open decision branches
  synthesize <id>          produce a summary of decisions + assumptions`,
	}
	cmd.AddCommand(newGrillStartCmd())
	cmd.AddCommand(newGrillNextCmd())
	cmd.AddCommand(newGrillAnswerCmd())
	cmd.AddCommand(newGrillStatusCmd())
	cmd.AddCommand(newGrillSynthesizeCmd())
	return cmd
}

func newGrillStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <topic>",
		Short: "Begin a grilling session on the given topic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			s, err := m.Start(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "started session %s\n", s.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "  topic: %s\n", s.Topic)
			fmt.Fprintf(cmd.OutOrStdout(), "  seed question: %s\n", s.Decisions[0].Question)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintf(cmd.OutOrStdout(), "  next: sin-code grill next %s\n", s.ID)
			return nil
		},
	}
}

func newGrillNextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "next <id>",
		Short: "Ask the next adversarial question",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			child, parent, err := m.Next(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sub-question under %s:\n", parent)
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", child.ID, child.Question)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintf(cmd.OutOrStdout(), "  answer: sin-code grill answer %s %s \"<your response>\"\n", args[0], child.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "  resolve: sin-code grill answer %s %s done\n", args[0], child.ID)
			return nil
		},
	}
}

func newGrillAnswerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "answer <id> <decision-id> <text>",
		Short: "Record the operator's response to a decision (use 'done' to resolve)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			if err := m.Answer(args[0], args[1], args[2]); err != nil {
				return err
			}
			action := "answered"
			if args[2] == "done" || args[2] == "skip" {
				action = "resolved"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s decision %s in session %s\n", action, args[1], args[0])
			return nil
		},
	}
}

func newGrillStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Show resolved + open decision branches",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			s, err := m.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Session %s\n", s.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "  topic: %s\n", s.Topic)
			fmt.Fprintf(cmd.OutOrStdout(), "  started: %s\n", s.StartedAt.Format("2006-01-02 15:04:05"))
			fmt.Fprintf(cmd.OutOrStdout(), "  decisions: %d (open=%d)\n", len(s.Decisions), s.OpenQuestions)
			for _, d := range s.Decisions {
				marker := "[ ]"
				switch d.Status {
				case "answered":
					marker = "[~]"
				case "resolved":
					marker = "[x]"
				case "deferred":
					marker = "[>]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "    %s %s: %s\n", marker, d.ID, d.Question)
				if d.Answer != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "         answer: %s\n", d.Answer)
				}
			}
			return nil
		},
	}
}

func newGrillSynthesizeCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "synthesize <id>",
		Short: "Produce a summary of decisions, assumptions, and open questions",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			syn, err := m.Synthesize(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(syn)
			}
			fmt.Fprintln(c.OutOrStdout(), "# Grilling Synthesis")
			fmt.Fprintln(c.OutOrStdout(), "")
			fmt.Fprintln(c.OutOrStdout(), "## Resolved")
			for _, r := range syn.Resolved {
				fmt.Fprintf(c.OutOrStdout(), "- %s\n", r)
			}
			if len(syn.Assumptions) > 0 {
				fmt.Fprintln(c.OutOrStdout(), "")
				fmt.Fprintln(c.OutOrStdout(), "## Assumptions")
				for _, a := range syn.Assumptions {
					fmt.Fprintf(c.OutOrStdout(), "- %s\n", a)
				}
			}
			if len(syn.Open) > 0 {
				fmt.Fprintln(c.OutOrStdout(), "")
				fmt.Fprintln(c.OutOrStdout(), "## Open")
				for _, o := range syn.Open {
					fmt.Fprintf(c.OutOrStdout(), "- %s\n", o)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}
