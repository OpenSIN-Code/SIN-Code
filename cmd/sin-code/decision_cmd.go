// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/decision"
	"github.com/spf13/cobra"
)

func newDecisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decision",
		Short: "Manage architectural decisions across sessions",
	}
	cmd.AddCommand(newDecisionListCmd())
	cmd.AddCommand(newDecisionSearchCmd())
	return cmd
}

func newDecisionListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recorded architectural decisions",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := os.Getwd()
			if err != nil {
				return err
			}
			store, err := decision.Open(ws)
			if err != nil {
				return err
			}
			defer store.Close()
			decisions, err := store.List(context.Background(), ws, 50)
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(decisions)
			}
			for _, d := range decisions {
				fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n  %s\n\n", d.Timestamp.Format("2006-01-02 15:04"), d.Decision, d.Rationale)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newDecisionSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search architectural decisions",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := os.Getwd()
			if err != nil {
				return err
			}
			store, err := decision.Open(ws)
			if err != nil {
				return err
			}
			defer store.Close()
			decisions, err := store.Search(context.Background(), ws, args[0])
			if err != nil {
				return err
			}
			for _, d := range decisions {
				fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n  %s\n\n", d.Timestamp.Format("2006-01-02 15:04"), d.Decision, d.Rationale)
			}
			return nil
		},
	}
}
