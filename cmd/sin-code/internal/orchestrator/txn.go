// SPDX-License-Identifier: MIT
// Purpose: FsTxn — atomic filesystem transaction for chains that mutate
// multiple files. Snapshot on first touch, all-or-nothing semantics,
// idempotent Rollback safe to defer unconditionally.
package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type FsTxn struct {
	mu         sync.Mutex
	workdir    string
	snapshots  map[string][]byte
	committed  bool
	rolledBack bool
}

func BeginTxn(workdir string) *FsTxn {
	return &FsTxn{workdir: workdir, snapshots: map[string][]byte{}}
}

func (t *FsTxn) WriteFile(relPath string, content []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.committed || t.rolledBack {
		return fmt.Errorf("txn: already finished")
	}

	abs := filepath.Join(t.workdir, relPath)
	if _, seen := t.snapshots[relPath]; !seen {
		orig, err := os.ReadFile(abs)
		switch {
		case err == nil:
			t.snapshots[relPath] = orig
		case os.IsNotExist(err):
			t.snapshots[relPath] = nil
		default:
			return fmt.Errorf("txn snapshot %s: %w", relPath, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return fmt.Errorf("txn mkdir %s: %w", relPath, err)
	}
	if err := os.WriteFile(abs, content, 0o600); err != nil {
		return fmt.Errorf("txn write %s: %w", relPath, err)
	}
	return nil
}

func (t *FsTxn) DeleteFile(relPath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.committed || t.rolledBack {
		return fmt.Errorf("txn: already finished")
	}
	abs := filepath.Join(t.workdir, relPath)
	if _, seen := t.snapshots[relPath]; !seen {
		orig, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("txn snapshot-for-delete %s: %w", relPath, err)
		}
		t.snapshots[relPath] = orig
	}
	return os.Remove(abs)
}

func (t *FsTxn) Touched() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.snapshots))
	for p := range t.snapshots {
		out = append(out, p)
	}
	return out
}

func (t *FsTxn) Commit() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.rolledBack {
		return fmt.Errorf("txn: already rolled back")
	}
	t.committed = true
	t.snapshots = nil
	return nil
}

func (t *FsTxn) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.committed || t.rolledBack {
		return nil
	}
	var firstErr error
	for rel, orig := range t.snapshots {
		abs := filepath.Join(t.workdir, rel)
		var err error
		if orig == nil {
			err = os.Remove(abs)
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = os.WriteFile(abs, orig, 0o600)
		}
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("txn rollback %s: %w", rel, err)
		}
	}
	t.rolledBack = true
	return firstErr
}
