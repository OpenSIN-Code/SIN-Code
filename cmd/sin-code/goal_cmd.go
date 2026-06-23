// SPDX-License-Identifier: MIT
// Purpose: `sin-code goal` — manage the autonomous goal queue.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
)

// goalCmdHooks are package-level hooks to make error branches and external
// calls testable without spinning up the real goal queue.
var (
	goalOpenHook = func(path string) (*autonomy.Queue, error) { return autonomy.Open(path) }
	goalDiscoverHook = func(cfg autonomy.DiscoverConfig) ([]autonomy.Finding, error) {
		return autonomy.Discover(cfg)
	}
	goalEnqueueFindingsHook = func(ctx context.Context, q *autonomy.Queue, workspace string, findings []autonomy.Finding, maxRetries int) (int, error) {
		return autonomy.EnqueueFindings(ctx, q, workspace, findings, maxRetries)
	}
	goalAddHook = func(ctx context.Context, q *autonomy.Queue, prompt, workspace string, priority, maxRetries int) (int64, error) {
		return q.Add(ctx, prompt, workspace, priority, maxRetries)
	}
	goalAddWithContractHook = func(ctx context.Context, q *autonomy.Queue, prompt, workspace string, priority, maxRetries int, contract string) (int64, error) {
		return q.AddWithContract(ctx, prompt, workspace, priority, maxRetries, contract)
	}
	goalListHook = func(ctx context.Context, q *autonomy.Queue, status autonomy.GoalStatus) ([]autonomy.Goal, error) {
		return q.List(ctx, status)
	}
	goalReadContractFileHook = func(path string) ([]byte, error) { return os.ReadFile(path) }
	goalContractUnmarshalHook = func(s string) (*goalcontract.GoalContract, error) { return goalcontract.Unmarshal(s) }
	goalContractMarshalHook = func(c *goalcontract.GoalContract) (string, error) { return c.Marshal() }
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
				raw, rerr := goalReadContractFileHook(contractFile)
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
			findings, err := goalDiscoverHook(autonomy.DiscoverConfig{
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

	cmd.AddCommand(addCmd, listCmd, discoverCmd)
	return cmd
}
