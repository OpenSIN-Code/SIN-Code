// SPDX-License-Identifier: MIT
// Purpose: Rollback — list snapshots and restore pre-update state.
// Docs: update_rollback.doc.md
package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func runRollback(ctx context.Context, opts UpdateOptions) ([]*PhaseResult, error) {
	bm, err := NewBackupManager()
	if err != nil {
		return nil, err
	}
	if opts.StateRoot != "" {
		bm.StateRoot = opts.StateRoot
	}

	latestDir, err := bm.Latest()
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	if latestDir == "" {
		fmt.Println("No snapshot to rollback to.")
		return nil, nil
	}

	manifest, err := ReadManifest(latestDir)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	fmt.Printf("Rolling back to snapshot %s (created %s)\n", manifest.Timestamp, latestDir)

	results := []*PhaseResult{}
	result := &PhaseResult{Name: "rollback"}

	if manifest.Pre.GoBins != nil {
		binDir := binDirPath()
		for tool, version := range manifest.Pre.GoBins {
			src := filepath.Join(latestDir, tool)
			dst := filepath.Join(binDir, tool)
			if _, err := os.Stat(src); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: backup not found at %s (version %s)", tool, src, version))
				fmt.Fprintf(os.Stderr, "[warn] %s: backup not found at %s -- skipping\n", tool, src)
				continue
			}
			if err := copyFile(src, dst); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: restore failed: %v", tool, err))
				continue
			}
			os.Chmod(dst, 0755)
			result.Updated++
		}
	}

	results = append(results, result)
	if result.Failed > 0 {
		fmt.Fprintf(os.Stderr, "[warn] rollback completed with %d failures\n", result.Failed)
	} else {
		fmt.Println("Rollback completed successfully.")
	}
	return results, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}
