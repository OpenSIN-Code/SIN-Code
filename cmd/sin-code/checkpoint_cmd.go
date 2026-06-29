// SPDX-License-Identifier: MIT
// Purpose: `sin-code checkpoint` and `sin-code rewind` — manual snapshot
// + restore of the workspace, plus a timeline view (issue #194). Pairs
// with the auto-checkpoint wired in the loop so users always have an
// escape hatch after a bad multi-file edit.
//
// v3.27.0 (issue #483): added git-based subcommands under
// `sin-code checkpoint`:
//   - checkpoint create [message]  — git tag + metadata
//   - checkpoint list              — list git-based checkpoints
//   - checkpoint rollback <id>     — restore workspace to checkpoint
//   - checkpoint diff <id>         — show changes since checkpoint
//   - checkpoint delete <id>       — delete a checkpoint
// The legacy `sin-code checkpoint [label]` (blob-based, issue #194) is
// preserved as the default action when no subcommand is given.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/checkpoint"
)

func NewCheckpointCmd() *cobra.Command {
	var workspace string
	var session string
	cmd := &cobra.Command{
		Use:   "checkpoint [label]",
		Short: "Snapshot the current workspace state",
		RunE: func(cmd *cobra.Command, args []string) error {
			label := "manual"
			if len(args) > 0 {
				label = args[0]
			}
			store, err := checkpoint.Open(workspace)
			if err != nil {
				return err
			}
			defer store.Close()
			paths, err := dirtyPaths(workspace)
			if err != nil {
				return err
			}
			id, err := store.Capture(context.Background(), workspace, session, label, paths)
			if err != nil {
				return err
			}
			fmt.Printf("checkpoint %s captured (%d files)\n", id, len(paths))
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	cmd.Flags().StringVar(&session, "session", "manual", "session id for this checkpoint")

	// --- git-based subcommands (issue #483) ---------------------------
	cmd.AddCommand(newCheckpointCreateCmd())
	cmd.AddCommand(newCheckpointListCmd())
	cmd.AddCommand(newCheckpointRollbackCmd())
	cmd.AddCommand(newCheckpointDiffCmd())
	cmd.AddCommand(newCheckpointDeleteCmd())
	return cmd
}

// newCheckpointCreateCmd implements `sin-code checkpoint create [message]`.
// Creates a git tag (sin-checkpoint-<id>) on the current HEAD and
// records metadata in the SQLite store.
func newCheckpointCreateCmd() *cobra.Command {
	var workspace string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "create [message]",
		Short: "Create a git-based workspace checkpoint",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			message := "checkpoint"
			if len(args) > 0 {
				message = args[0]
			}
			store, err := checkpoint.OpenGit(workspace)
			if err != nil {
				return err
			}
			defer store.Close()
			cp, err := store.Create(context.Background(), message)
			if err != nil {
				return err
			}
			if jsonOut {
				fmt.Printf(`{"id":"%s","message":"%s","git_ref":"%s","created_at":"%s"}`+"\n",
					cp.ID, cp.Message, cp.GitRef, cp.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
			} else {
				fmt.Printf("checkpoint %s created (ref: %s)\n", cp.ID, cp.GitRef)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

// newCheckpointListCmd implements `sin-code checkpoint list`.
func newCheckpointListCmd() *cobra.Command {
	var workspace string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all git-based checkpoints for this workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := checkpoint.OpenGit(workspace)
			if err != nil {
				return err
			}
			defer store.Close()
			list, err := store.List(context.Background())
			if err != nil {
				return err
			}
			if len(list) == 0 {
				if jsonOut {
					fmt.Println("[]")
				} else {
					fmt.Println("no checkpoints")
				}
				return nil
			}
			if jsonOut {
				fmt.Print("[")
				for i, c := range list {
					if i > 0 {
						fmt.Print(",")
					}
					fmt.Printf(`{"id":"%s","message":"%s","git_ref":"%s","created_at":"%s"}`,
						c.ID, c.Message, c.GitRef, c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
				}
				fmt.Println("]")
			} else {
				for _, c := range list {
					fmt.Printf("%s  %s  %q  (ref: %s)\n",
						c.CreatedAt.Format("2006-01-02 15:04:05"),
						c.ID, c.Message, c.GitRef)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

// newCheckpointRollbackCmd implements `sin-code checkpoint rollback <id>`.
// This is a DESTRUCTIVE operation (M4). The --force flag bypasses the
// confirmation prompt. In headless mode, the caller must pass --force
// (or --yolo at the chat level) — the permission engine handles this.
func newCheckpointRollbackCmd() *cobra.Command {
	var workspace string
	var force bool
	cmd := &cobra.Command{
		Use:   "rollback <id>",
		Short: "Restore the workspace to a checkpoint (destructive)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Fprintf(os.Stderr, "WARNING: rollback is destructive and will overwrite uncommitted changes.\n")
				fmt.Fprintf(os.Stderr, "Pass --force to confirm.\n")
				return fmt.Errorf("rollback requires --force")
			}
			store, err := checkpoint.OpenGit(workspace)
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Rollback(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("workspace restored to checkpoint %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	cmd.Flags().BoolVar(&force, "force", false, "confirm destructive rollback (required)")
	return cmd
}

// newCheckpointDiffCmd implements `sin-code checkpoint diff <id>`.
func newCheckpointDiffCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "diff <id>",
		Short: "Show what changed since the checkpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := checkpoint.OpenGit(workspace)
			if err != nil {
				return err
			}
			defer store.Close()
			diff, err := store.Diff(context.Background(), args[0])
			if err != nil {
				return err
			}
			if diff == "" {
				fmt.Println("no changes since checkpoint")
			} else {
				fmt.Print(diff)
				if diff[len(diff)-1] != '\n' {
					fmt.Println()
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	return cmd
}

// newCheckpointDeleteCmd implements `sin-code checkpoint delete <id>`.
func newCheckpointDeleteCmd() *cobra.Command {
	var workspace string
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a checkpoint (removes git tag + metadata)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Fprintf(os.Stderr, "Pass --force to confirm deletion of checkpoint %s.\n", args[0])
				return fmt.Errorf("delete requires --force")
			}
			store, err := checkpoint.OpenGit(workspace)
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Delete(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("checkpoint %s deleted\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	cmd.Flags().BoolVar(&force, "force", false, "confirm deletion (required)")
	return cmd
}

func NewRewindCmd() *cobra.Command {
	var workspace string
	var list bool
	cmd := &cobra.Command{
		Use:   "rewind [checkpoint-id]",
		Short: "Restore the workspace to a prior checkpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := checkpoint.Open(workspace)
			if err != nil {
				return err
			}
			defer store.Close()
			if list || len(args) == 0 {
				snaps, err := store.List(context.Background(), "", 50)
				if err != nil {
					return err
				}
				if len(snaps) == 0 {
					fmt.Println("no checkpoints")
					return nil
				}
				for _, s := range snaps {
					fmt.Printf("%s  %s  %q  (%d files)\n",
						s.CreatedAt.Format("2006-01-02 15:04:05"),
						s.ID, s.Label, len(s.Files))
				}
				return nil
			}
			if err := store.Restore(context.Background(), workspace, args[0]); err != nil {
				return err
			}
			fmt.Printf("workspace restored to %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	cmd.Flags().BoolVar(&list, "list", false, "list checkpoints instead of restoring")
	return cmd
}

// dirtyPaths returns the workspace-relative list of files that exist
// under the workspace, excluding .sin-code/, .git/, and the
// checkpoint store itself. This is a conservative v0: we snapshot
// every regular file we can find. v1 should integrate with the
// existing change detector (LSP / git status).
func dirtyPaths(workspace string) ([]string, error) {
	var paths []string
	skip := map[string]bool{
		".sin-code":    true,
		".git":         true,
		"node_modules": true,
	}
	err := filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			if skip[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return nil
		}
		// Skip the checkpoint store itself (recursion guard).
		if len(rel) >= 11 && rel[:11] == ".sin-code/c" {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	return paths, err
}
