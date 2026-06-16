// SPDX-License-Identifier: MIT
// Purpose: `sin-code checkpoint` and `sin-code rewind` — manual snapshot
// + restore of the workspace, plus a timeline view (issue #194). Pairs
// with the auto-checkpoint wired in the loop so users always have an
// escape hatch after a bad multi-file edit.
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
// under the workspace, excluding .sin-code/, .git/, and node_modules.
// v0: we snapshot every regular file. v1 should integrate with the
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
			return nil
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
		if len(rel) >= 11 && rel[:11] == ".sin-code/c" {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	return paths, err
}
