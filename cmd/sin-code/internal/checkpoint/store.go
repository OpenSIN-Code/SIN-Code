// SPDX-License-Identifier: MIT
// Purpose: content-addressed workspace snapshots for safe rewind (issue #194).
// Snapshots file bytes BEFORE they are mutated (auto) or the full dirty set
// (manual), and can restore any prior state. M2-safe: modernc.org/sqlite
// for the index (CGO-free, already a project dependency) + stdlib for blob
// I/O. No git, no external tooling, single static binary preserved.
package checkpoint

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	root string
}

type Snapshot struct {
	ID        string
	SessionID string
	Label     string
	CreatedAt time.Time
	Files     []FileRef
}

type FileRef struct {
	Path string
	Hash string
}

func Open(workspace string) (*Store, error) {
	root := filepath.Join(workspace, ".sin-code", "checkpoints")
	if err := os.MkdirAll(filepath.Join(root, "blobs"), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "index.db"))
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, root: root}
	return s, s.migrate()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS snapshots (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  label TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS files (
  snapshot_id TEXT NOT NULL,
  path TEXT NOT NULL,
  hash TEXT NOT NULL,
  PRIMARY KEY (snapshot_id, path)
);
CREATE INDEX IF NOT EXISTS idx_files_snapshot ON files(snapshot_id);`)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) putBlob(content []byte) (string, error) {
	sum := sha256.Sum256(content)
	h := hex.EncodeToString(sum[:])
	dst := filepath.Join(s.root, "blobs", h)
	if _, err := os.Stat(dst); err == nil {
		return h, nil
	}
	return h, os.WriteFile(dst, content, 0o644)
}

func (s *Store) Capture(ctx context.Context, workspace, sessionID, label string, paths []string) (string, error) {
	id := fmt.Sprintf("ckpt-%d", time.Now().UnixNano())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO snapshots(id, session_id, label, created_at) VALUES(?,?,?,?)`,
		id, sessionID, label, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return "", err
	}

	for _, rel := range paths {
		abs := filepath.Join(workspace, rel)
		var hash string
		if b, rerr := os.ReadFile(abs); rerr == nil {
			if hash, err = s.putBlob(b); err != nil {
				return "", err
			}
		}
		if _, err = tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO files(snapshot_id, path, hash) VALUES(?,?,?)`,
			id, rel, hash); err != nil {
			return "", err
		}
	}
	return id, tx.Commit()
}

func (s *Store) Restore(ctx context.Context, workspace, id string) error {
	rows, err := s.db.QueryContext(ctx, `SELECT path, hash FROM files WHERE snapshot_id=?`, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var refs []FileRef
	for rows.Next() {
		var f FileRef
		if err := rows.Scan(&f.Path, &f.Hash); err != nil {
			return err
		}
		refs = append(refs, f)
	}
	if len(refs) == 0 {
		return fmt.Errorf("checkpoint %q not found or empty", id)
	}

	for _, f := range refs {
		abs := filepath.Join(workspace, f.Path)
		if f.Hash == "" {
			_ = os.Remove(abs)
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.root, "blobs", f.Hash))
		if err != nil {
			return fmt.Errorf("read blob for %s: %w", f.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) List(ctx context.Context, sessionID string, limit int) ([]Snapshot, error) {
	q := `SELECT id, session_id, label, created_at FROM snapshots`
	args := []any{}
	if sessionID != "" {
		q += ` WHERE session_id=?`
		args = append(args, sessionID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var sn Snapshot
		var ts string
		if err := rows.Scan(&sn.ID, &sn.SessionID, &sn.Label, &ts); err != nil {
			return nil, err
		}
		sn.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, sn)
	}
	return out, nil
}

func (s *Store) Prune(hash string) error {
	if hash == "" {
		return nil
	}
	return os.Remove(filepath.Join(s.root, "blobs", hash))
}
