// SPDX-License-Identifier: MIT

package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var IndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage persistent incremental code index",
	Long: `Builds, refreshes, and inspects a persistent gob-persisted
index at <root>/.sin-code/index.bin. Trigram + symbol table
for instant lookups.`,
}

func init() {
	IndexCmd.AddCommand(indexBuildCmd)
	IndexCmd.AddCommand(indexRefreshCmd)
	IndexCmd.AddCommand(indexStatusCmd)
	IndexCmd.AddCommand(indexWatchCmd)
	IndexCmd.AddCommand(indexClearCmd)
}

var indexBuildCmd = &cobra.Command{
	Use:   "build [root]",
	Short: "Build index from scratch",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		idx, err := buildIndex(root)
		if err != nil {
			return err
		}
		if err := saveIndex(idx); err != nil {
			return err
		}
		setFileIndex(idx)
		fmt.Printf("Indexed %d files in %s\n", idx.len(), root)
		return nil
	},
}

var indexRefreshCmd = &cobra.Command{
	Use:   "refresh [root]",
	Short: "Incremental refresh (stat-based)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		idx, existed, err := getFileIndex(root)
		if err != nil {
			return err
		}
		if !existed {
			fmt.Println("No existing index. Run 'sin-code index build' first.")
			return nil
		}
		idx, added, removed, err := refreshIndex(idx)
		if err != nil {
			return err
		}
		if err := saveIndex(idx); err != nil {
			return err
		}
		setFileIndex(idx)
		fmt.Printf("Refreshed: +%d -%d files. Total %d\n", added, removed, idx.len())
		return nil
	},
}

var indexStatusCmd = &cobra.Command{
	Use:   "status [root]",
	Short: "Show index status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		idx, existed, err := getFileIndex(root)
		if err != nil {
			return err
		}
		if !existed || idx.len() == 0 {
			fmt.Println("No index found.")
			return nil
		}
		fmt.Printf("Index: %s\n", indexPath(root))
		fmt.Printf("Files: %d\n", idx.len())
		fmt.Printf("Created: %s\n", idx.createdAt.Format(time.RFC3339))
		return nil
	},
}

var indexWatchCmd = &cobra.Command{
	Use:   "watch [root]",
	Short: "Auto-refresh every 30s (foreground daemon)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		for {
			idx, existed, err := getFileIndex(root)
			if err != nil {
				return err
			}
			if !existed {
				idx, err = buildIndex(root)
				if err != nil {
					return err
				}
			} else {
				idx, _, _, err = refreshIndex(idx)
				if err != nil {
					return err
				}
			}
			if err := saveIndex(idx); err != nil {
				return err
			}
			setFileIndex(idx)
			time.Sleep(30 * time.Second)
		}
	},
}

var indexClearCmd = &cobra.Command{
	Use:   "clear [root]",
	Short: "Delete index file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		p := indexPath(root)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		setFileIndex(nil)
		fmt.Println("Index cleared.")
		return nil
	},
}
