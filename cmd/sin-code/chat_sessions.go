// SPDX-License-Identifier: MIT
// Purpose: `sin-code sessions` — list/show/rm/fork/tree for persisted
// agent sessions.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
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
			fmt.Printf("%-28s %-22s %-22s %-22s %s\n", "ID", "CREATED", "UPDATED", "PARENT", "TITLE")
			for _, i := range infos {
				parent := i.ParentID
				if parent == "" {
					parent = "-"
				}
				fmt.Printf("%-28s %-22s %-22s %-22s %s\n", i.ID, i.CreatedAt, i.UpdatedAt, parent, i.Title)
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

	var forkTurn int
	var forkTitle string
	forkCmd := &cobra.Command{
		Use:   "fork <src-session-id>",
		Short: "Fork a session. Clones the first N (or all) messages and records parent_id lineage.",
		Long: "Fork a session at message N. Default --turn=-1 means \"copy entire history\" " +
			"(equivalent to clamping to len(history)). Records parent_id automatically so " +
			"`sessions tree` can recover the ancestry chain. The WebUI v2 /api/v1/sessions/fork " +
			"endpoint (issue #52) calls the same Store.Fork via sessionForkHook — both surfaces " +
			"now share one parent-tracking contract.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			child, err := store.ForkEx(args[0], forkTurn, forkTitle)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(struct {
					ParentID     string `json:"parent_id"`
					ID           string `json:"id"`
					ForkedAtTurn int    `json:"forked_at_turn"`
					Title        string `json:"title,omitempty"`
				}{args[0], child.ID, forkTurn, forkTitle})
			}
			fmt.Printf("forked %s → %s  (parent=%s, turn=%d)\n", args[0], child.ID, args[0], forkTurn)
			return nil
		},
	}
	forkCmd.Flags().IntVar(&forkTurn, "turn", -1, "fork at message N (negative = end-of-history)")
	forkCmd.Flags().StringVar(&forkTitle, "title", "", "optional title for the forked session")

	treeCmd := &cobra.Command{
		Use:   "tree <session-id>",
		Short: "Walk the parent_id chain upward; emit root → ... → self.",
		Long: "Print the lineage of a session as a tree walk following parent_id links. " +
			"Terminates on missing parent, empty parent_id (root reached), cycle break, " +
			"or self-reference. Useful for `--json` piping into further analysis.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			chain, err := store.Tree(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(chain)
			}
			for i, n := range chain {
				prefix := "  "
				marker := "└─"
				label := fmt.Sprintf("%s [%s] %q", n.ID, n.UpdatedAt, n.Title)
				if i == 0 {
					fmt.Printf("root: %s [%s] %q\n", n.ID, n.CreatedAt, n.Title)
					continue
				}
				if i == len(chain)-1 {
					fmt.Printf("%s%s self: %s\n", prefix, marker, label)
				} else {
					fmt.Printf("%s%s %s\n", prefix, marker, label)
				}
			}
			return nil
		},
	}

	cmd.AddCommand(listCmd, showCmd, rmCmd, forkCmd, treeCmd)
	return cmd
}
