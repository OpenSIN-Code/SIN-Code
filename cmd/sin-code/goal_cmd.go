// SPDX-License-Identifier: MIT
// Purpose: `sin-code goal` — manage the autonomous goal queue.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
)

func NewGoalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goal",
		Short: "Manage the autonomous goal queue",
	}

	var priority, retries int
	var criteria []string
	var contractFile string
	addCmd := &cobra.Command{
		Use:   "add <prompt>",
		Short: "Enqueue a goal for the daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			ws, _ := os.Getwd()

			// Build the Definition-of-Done contract from flags. --contract-file
			// supplies a full JSON contract; --criteria adds semantic criteria
			// the stop-gate's independent evaluator must confirm.
			contractJSON := ""
			if contractFile != "" {
				raw, rerr := os.ReadFile(contractFile)
				if rerr != nil {
					return fmt.Errorf("read contract file: %w", rerr)
				}
				c, perr := goalcontract.Unmarshal(string(raw))
				if perr != nil {
					return fmt.Errorf("invalid contract file: %w", perr)
				}
				c.SemanticCriteria = append(c.SemanticCriteria, criteria...)
				contractJSON, _ = c.Marshal()
			} else if len(criteria) > 0 {
				c := &goalcontract.GoalContract{SemanticCriteria: criteria}
				contractJSON, _ = c.Marshal()
			}

			var id int64
			if contractJSON != "" {
				id, err = q.AddWithContract(cmd.Context(), args[0], ws, priority, retries, contractJSON)
			} else {
				id, err = q.Add(cmd.Context(), args[0], ws, priority, retries)
			}
			if err != nil {
				return err
			}
			if contractJSON != "" {
				fmt.Printf("goal %d enqueued with contract (priority %d, retries %d)\n", id, priority, retries)
			} else {
				fmt.Printf("goal %d enqueued (priority %d, retries %d)\n", id, priority, retries)
			}
			return nil
		},
	}
	addCmd.Flags().IntVar(&priority, "priority", 0, "higher runs sooner")
	addCmd.Flags().IntVar(&retries, "retries", 3, "retry budget")
	addCmd.Flags().StringArrayVar(&criteria, "criteria", nil, "acceptance criterion the stop-gate evaluator must confirm (repeatable)")
	addCmd.Flags().StringVar(&contractFile, "contract-file", "", "path to a JSON Definition-of-Done contract")

	var status string
	var jsonOut bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List goals",
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			goals, err := q.List(cmd.Context(), autonomy.GoalStatus(status))
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(goals)
			}
			if len(goals) == 0 {
				fmt.Println("no goals")
				return nil
			}
			fmt.Printf("%-5s %-10s %-4s %-8s %s\n", "ID", "STATUS", "TRY", "PRIO", "PROMPT")
			for _, g := range goals {
				fmt.Printf("%-5d %-10s %d/%-2d %-8d %.60s\n", g.ID, g.Status, g.Attempts, g.MaxRetries, g.Priority, g.Prompt)
			}
			return nil
		},
	}
	listCmd.Flags().StringVar(&status, "status", "", "filter: pending|running|verified|failed|exhausted")
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	var dryRun bool
	var discoverRetries int
	discoverCmd := &cobra.Command{
		Use:   "discover",
		Short: "Scan the repo for latent work (TODO/FIXME, MASTER_TODO) and enqueue goals",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, _ := os.Getwd()
			findings, err := autonomy.Discover(autonomy.DiscoverConfig{
				Workspace:    ws,
				ScanComments: true,
				ScanMaster:   true,
			})
			if err != nil {
				return err
			}
			if len(findings) == 0 {
				fmt.Println("no work discovered")
				return nil
			}
			if dryRun {
				fmt.Printf("%d finding(s) (dry-run, not enqueued):\n", len(findings))
				for _, f := range findings {
					fmt.Printf("  [%s] %.80s\n", f.Source, f.Prompt)
				}
				return nil
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			n, err := autonomy.EnqueueFindings(cmd.Context(), q, ws, findings, discoverRetries)
			if err != nil {
				return err
			}
			fmt.Printf("discovered %d finding(s), enqueued %d new goal(s)\n", len(findings), n)
			return nil
		},
	}
	discoverCmd.Flags().BoolVar(&dryRun, "dry-run", false, "list findings without enqueueing")
	discoverCmd.Flags().IntVar(&discoverRetries, "retries", 3, "retry budget for enqueued goals")

	// goal status <id> — show one goal with subtasks (issue #140 fusion).
	var statusJsonOut bool
	statusCmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Show one goal's progress, attempts, and children (issue #140 fusion)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := parseGoalID(args[0])
			if perr != nil {
				return perr
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			g, err := q.Get(cmd.Context(), id)
			if err != nil {
				return err
			}
			children, err := q.Children(cmd.Context(), id)
			if err != nil {
				return err
			}
			if statusJsonOut {
				payload := map[string]any{
					"goal":     g,
					"children": children,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Goal %d [%s] attempts=%d/%d priority=%d\n",
				g.ID, g.Status, g.Attempts, g.MaxRetries, g.Priority)
			fmt.Fprintf(cmd.OutOrStdout(), "  prompt: %s\n", g.Prompt)
			if len(g.LastError) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  last_error: %s\n", g.LastError)
			}
			if len(children) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  (no subtasks)")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  subtasks (%d):\n", len(children))
			for _, c := range children {
				fmt.Fprintf(cmd.OutOrStdout(), "    %-5d [%-10s] %.60s\n", c.ID, c.Status, c.Prompt)
			}
			return nil
		},
	}
	statusCmd.Flags().BoolVar(&statusJsonOut, "json", false, "emit JSON")

	// goal complete <id> — mark a goal as verified/done (issue #140 fusion).
	// Mirrors goal_complete in the external Python MCP server.
	var completeSession string
	completeCmd := &cobra.Command{
		Use:   "complete <id>",
		Short: "Mark a goal as verified/done (issue #140 fusion; maps to q.Complete)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := parseGoalID(args[0])
			if perr != nil {
				return perr
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			if err := q.Complete(cmd.Context(), id, completeSession); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "goal %d marked complete (session %q)\n", id, completeSession)
			return nil
		},
	}
	completeCmd.Flags().StringVar(&completeSession, "session", "", "session id of the worker that completed the goal")

	// goal subtask <parent-id> <prompt> — add a subtask to a parent (issue #140 fusion).
	// Mirrors goal_subtask in the external Python MCP server.
	var subtaskPriority, subtaskRetries int
	var subtaskCriteria []string
	subtaskCmd := &cobra.Command{
		Use:   "subtask <parent-id> <prompt>",
		Short: "Add a subtask under a parent goal (issue #140 fusion; maps to q.AddSub)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			parentID, perr := parseGoalID(args[0])
			if perr != nil {
				return perr
			}
			contractJSON := ""
			if len(subtaskCriteria) > 0 {
				c := &goalcontract.GoalContract{SemanticCriteria: subtaskCriteria}
				contractJSON, _ = c.Marshal()
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			id, err := q.AddSub(cmd.Context(), parentID, args[1], subtaskPriority, subtaskRetries, contractJSON)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "subtask %d enqueued under parent %d\n", id, parentID)
			return nil
		},
	}
	subtaskCmd.Flags().IntVar(&subtaskPriority, "priority", 0, "higher runs sooner")
	subtaskCmd.Flags().IntVar(&subtaskRetries, "retries", 3, "retry budget")
	subtaskCmd.Flags().StringArrayVar(&subtaskCriteria, "criteria", nil, "acceptance criterion (repeatable)")

	// goal report — emit a JSON or Markdown progress report (issue #140 fusion).
	// Maps to goal_report in the external Python MCP server. v0 ships the
	// JSON variant; Markdown rendering is a follow-up.
	var reportFormat string
	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a progress report across all goals (issue #140 fusion)",
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			goals, err := q.List(cmd.Context(), "")
			if err != nil {
				return err
			}
			byStatus := map[string]int{}
			for _, g := range goals {
				byStatus[string(g.Status)]++
			}
			switch reportFormat {
			case "json":
				payload := map[string]any{
					"total":     len(goals),
					"by_status": byStatus,
					"goals":     goals,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			case "md", "markdown", "":
				fmt.Fprintf(cmd.OutOrStdout(), "# Goal Report\n\n")
				fmt.Fprintf(cmd.OutOrStdout(), "Total: %d goal(s)\n\n", len(goals))
				fmt.Fprintf(cmd.OutOrStdout(), "## By status\n\n")
				for status, n := range byStatus {
					fmt.Fprintf(cmd.OutOrStdout(), "- **%s**: %d\n", status, n)
				}
				return nil
			default:
				return fmt.Errorf("unsupported format %q (json|md)", reportFormat)
			}
		},
	}
	reportCmd.Flags().StringVar(&reportFormat, "format", "md", "output format: md|json")

	cmd.AddCommand(addCmd, listCmd, discoverCmd, statusCmd, completeCmd, subtaskCmd, reportCmd)
	return cmd
}

// parseGoalID parses a numeric goal id. Strings like "#42" or "42" are
// both accepted (the "#" prefix is tolerated for operator ergonomics).
func parseGoalID(s string) (int64, error) {
	// Order matters: TrimSpace first, then TrimPrefix("#") — otherwise
	// "  #42  " would not match the leading "#".
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid goal id %q: %w", s, err)
	}
	return id, nil
}
