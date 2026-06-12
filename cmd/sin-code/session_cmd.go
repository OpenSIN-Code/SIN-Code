// SPDX-License-Identifier: MIT
// Purpose: `sin-code sessions` — list/show/rm for persisted agent sessions
// stored in ~/.local/share/sin-code/sessions.db (issue #44, mandate C2).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/spf13/cobra"
)

func NewSessionsCmd() *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage persisted agent sessions",
	}
	cmd.PersistentFlags().StringVar(&dbPath, "db", "", "sessions db path (default ~/.local/share/sin-code/sessions.db)")

	openStore := func() (*session.Store, error) {
		p := dbPath
		if p == "" {
			p = session.DefaultPath()
		}
		return session.Open(p)
	}

	var jsonOut bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			infos, err := store.List()
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(infos)
			}
			if len(infos) == 0 {
				fmt.Println("no sessions")
				return nil
			}
			fmt.Printf("%-28s %-22s %-22s %s\n", "ID", "CREATED", "UPDATED", "TITLE")
			for _, i := range infos {
				fmt.Printf("%-28s %-22s %-22s %s\n", i.ID, i.CreatedAt, i.UpdatedAt, i.Title)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	showCmd := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show the message history of a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			sess, err := store.StartOrResume(args[0])
			if err != nil {
				return err
			}
			for _, m := range sess.History() {
				content := m.Content
				if content == "" && len(m.ToolCalls) > 0 {
					content = "[tool calls] " + string(m.ToolCalls)
				}
				fmt.Printf("--- %s %s\n%s\n", strings.ToUpper(m.Role), m.ToolCallID, content)
			}
			return nil
		},
	}

	rmCmd := &cobra.Command{
		Use:   "rm <session-id>",
		Short: "Delete a session and its messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("deleted session %s\n", args[0])
			return nil
		},
	}

	cmd.AddCommand(listCmd, showCmd, rmCmd)
	return cmd
}
