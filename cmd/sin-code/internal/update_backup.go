// SPDX-License-Identifier: MIT
// Purpose: BackupManager — create / restore / cleanup update snapshots.
// Docs: update_backup.doc.md
package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type BackupManager struct {
	StateRoot string
	Now       func() string
}

func NewBackupManager() (*BackupManager, error) {
	root := os.Getenv("SIN_CODE_STATE_ROOT")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home dir: %w", err)
		}
		root = filepath.Join(home, ".local", "state", "sin-code")
	}
	return &BackupManager{StateRoot: root, Now: defaultNow}, nil
}

func defaultNow() string {
	return fmt.Sprintf("%d", time.Now().UTC().Unix())
}

func (b *BackupManager) Create() (string, error) {
	dir := SnapshotDir(b.StateRoot, b.Now())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}
	return dir, nil
}

func (b *BackupManager) List() ([]string, error) {
	root := filepath.Join(b.StateRoot, "updates")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	return dirs, nil
}

func (b *BackupManager) Latest() (string, error) {
	dirs, err := b.List()
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", nil
	}
	return dirs[0], nil
}

func (b *BackupManager) Prune(keep int) error {
	dirs, err := b.List()
	if err != nil {
		return err
	}
	if len(dirs) <= keep {
		return nil
	}
	for _, d := range dirs[keep:] {
		os.RemoveAll(d)
	}
	return nil
}
