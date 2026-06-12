// SPDX-License-Identifier: MIT
// Purpose: `sin-code goal` — manage the autonomous goal queue.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/spf13/cobra"
)

func NewGoalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goal",
		Short: "Manage the autonomous goal queue",
	}

	var priority, retries int
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
			id, err := q.Add(cmd.Context(), args[0], ws, priority, retries)
			if err != nil {
				return err
			}
			fmt.Printf("goal %d enqueued (priority %d, retries %d)\n", id, priority, retries)
			return nil
		},
	}
	addCmd.Flags().IntVar(&priority, "priority", 0, "higher runs sooner")
	addCmd.Flags().IntVar(&retries, "retries", 3, "retry budget")

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

	cmd.AddCommand(addCmd, listCmd)
	return cmd
}
