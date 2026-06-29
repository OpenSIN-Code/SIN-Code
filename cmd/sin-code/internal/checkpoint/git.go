// SPDX-License-Identifier: MIT
// Purpose: git-based workspace checkpoints (issue #483).
//
// General-purpose workspace snapshots that can be created and rolled
// back at any time. Uses git tags (sin-checkpoint-<id>) as the
// content-addressed reference and a SQLite metadata store at
// ~/.local/share/sin-code/checkpoints.db (per-workspace, keyed by
// a sha256 prefix of the absolute workspace path).
//
// M2: modernc.org/sqlite (CGO-free). M5: module path
// github.com/OpenSIN-Code/SIN-Code. M3: rollback does NOT bypass
// the verification gate — it is a workspace-restore primitive, not
// a loop-completion signal. M4: rollback is destructive and must
// be gated by the permission engine / --yolo in headless mode.
package checkpoint

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// GitCheckpoint is the metadata row stored in the SQLite index.
type GitCheckpoint struct {
	ID        string
	Message   string
	GitRef    string
	Workspace string
	CreatedAt time.Time
}

// GitStore is the git-backed checkpoint metadata store. It lives at
// ~/.local/share/sin-code/checkpoints.db and is shared across
// workspaces (the workspace column disambiguates rows).
type GitStore struct {
	db        *sql.DB
	workspace string // absolute path
	wsKey     string // 12-char sha256 prefix for indexing
}

// defaultGitDBPath returns the shared checkpoint DB path under the
// user's local share directory, matching the pattern used by
// session/lessons/ledger stores.
func defaultGitDBPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", "sin-code")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "checkpoints.db")
}

// workspaceKey returns a 12-char sha256 prefix of the absolute
// workspace path. Two projects never share a row.
func workspaceKey(workspace string) string {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		abs = workspace
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:12]
}

// OpenGit opens (or creates) the git-based checkpoint store for the
// given workspace. The SQLite DB is shared; rows are keyed by the
// workspace hash.
func OpenGit(workspace string) (*GitStore, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		abs = workspace
	}
	dbPath := defaultGitDBPath()
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	s := &GitStore{
		db:        db,
		workspace: abs,
		wsKey:     workspaceKey(workspace),
	}
	return s, s.migrate()
}

func (s *GitStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS git_checkpoints (
  id TEXT PRIMARY KEY,
  message TEXT,
  git_ref TEXT NOT NULL,
  workspace TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gc_workspace ON git_checkpoints(workspace);`)
	return err
}

// Close closes the underlying database handle.
func (s *GitStore) Close() error { return s.db.Close() }

// --- git helpers ----------------------------------------------------

// gitRun executes a git command in the workspace directory and returns
// trimmed stdout. A non-zero exit produces a descriptive error.
func gitRun(ctx context.Context, workspace string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		if stderr != "" {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), stderr)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitHEAD returns the current HEAD commit SHA (full) or an error if
// the workspace is not a git repo.
func gitHEAD(ctx context.Context, workspace string) (string, error) {
	return gitRun(ctx, workspace, "rev-parse", "HEAD")
}

// isInGitRepo returns true if the workspace is inside a git work tree.
func isInGitRepo(ctx context.Context, workspace string) bool {
	_, err := gitRun(ctx, workspace, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// checkpointID computes a short SHA from timestamp + message + git HEAD.
// This makes IDs deterministic per (moment, message, commit) and
// collision-resistant in practice.
func checkpointID(timestamp int64, message, head string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s", timestamp, message, head)
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// tagPrefix is the git tag namespace for checkpoints.
const tagPrefix = "sin-checkpoint-"

// --- public API -----------------------------------------------------

// Create captures a git-based checkpoint. It creates a lightweight git
// tag `sin-checkpoint-<id>` on the current HEAD and records metadata
// in the SQLite store. If the workspace is not a git repo, it returns
// ErrNotAGitRepo so the caller can fall back to the blob-based store
// (issue #194) if desired.
func (s *GitStore) Create(ctx context.Context, message string) (*GitCheckpoint, error) {
	if !isInGitRepo(ctx, s.workspace) {
		return nil, fmt.Errorf("checkpoint create: %w", ErrNotAGitRepo)
	}
	head, err := gitHEAD(ctx, s.workspace)
	if err != nil {
		return nil, fmt.Errorf("checkpoint create: get HEAD: %w", err)
	}
	now := time.Now().UTC()
	id := checkpointID(now.UnixNano(), message, head)
	ref := tagPrefix + id

	// Create a lightweight tag on the current HEAD.
	if _, err := gitRun(ctx, s.workspace, "tag", ref, head); err != nil {
		// Tag may already exist if the same (timestamp, message, head)
		// triple recurs — treat as idempotent success.
		if !strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("checkpoint create: git tag: %w", err)
		}
	}

	created := now.Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO git_checkpoints(id, message, git_ref, workspace, created_at) VALUES(?,?,?,?,?)`,
		id, message, ref, s.wsKey, created); err != nil {
		// Best-effort cleanup of the tag if the DB write fails.
		_, _ = gitRun(ctx, s.workspace, "tag", "-d", ref)
		return nil, fmt.Errorf("checkpoint create: db insert: %w", err)
	}

	return &GitCheckpoint{
		ID:        id,
		Message:   message,
		GitRef:    ref,
		Workspace: s.wsKey,
		CreatedAt: now,
	}, nil
}

// List returns all checkpoints for this workspace, newest first.
func (s *GitStore) List(ctx context.Context) ([]GitCheckpoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, message, git_ref, workspace, created_at FROM git_checkpoints WHERE workspace=? ORDER BY created_at DESC`,
		s.wsKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GitCheckpoint
	for rows.Next() {
		var c GitCheckpoint
		var ts string
		if err := rows.Scan(&c.ID, &c.Message, &c.GitRef, &c.Workspace, &ts); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get retrieves a single checkpoint by ID.
func (s *GitStore) Get(ctx context.Context, id string) (*GitCheckpoint, error) {
	var c GitCheckpoint
	var ts string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, message, git_ref, workspace, created_at FROM git_checkpoints WHERE id=? AND workspace=?`,
		id, s.wsKey).Scan(&c.ID, &c.Message, &c.GitRef, &c.Workspace, &ts)
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &c, nil
}

// Rollback restores the workspace to the state captured at the given
// checkpoint. It runs `git checkout <ref> -- .` to restore tracked
// files, then `git checkout HEAD -- .` is NOT run (we want the
// checkpoint's tree, not HEAD's).
//
// This is a DESTRUCTIVE operation (M4). The caller is responsible for
// permission-gating and confirmation prompts. M3: rollback does NOT
// signal task completion — it is a workspace-restore primitive only.
func (s *GitStore) Rollback(ctx context.Context, id string) error {
	cp, err := s.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("checkpoint rollback: %w", err)
	}
	// Restore the working tree to the tagged commit. We use
	// `git checkout <ref> -- .` which restores both the index and
	// the working tree to the state at <ref> without moving HEAD.
	if _, err := gitRun(ctx, s.workspace, "checkout", cp.GitRef, "--", "."); err != nil {
		return fmt.Errorf("checkpoint rollback: git checkout: %w", err)
	}
	return nil
}

// Diff returns the git diff between the checkpoint's tree and the
// current HEAD. The output is raw `git diff` text (unified format).
func (s *GitStore) Diff(ctx context.Context, id string) (string, error) {
	cp, err := s.Get(ctx, id)
	if err != nil {
		return "", fmt.Errorf("checkpoint diff: %w", err)
	}
	return gitRun(ctx, s.workspace, "diff", cp.GitRef+"..HEAD")
}

// Delete removes a checkpoint: deletes the git tag and the SQLite row.
func (s *GitStore) Delete(ctx context.Context, id string) error {
	cp, err := s.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("checkpoint delete: %w", err)
	}
	// Delete the git tag (best-effort — tag may already be gone).
	if _, err := gitRun(ctx, s.workspace, "tag", "-d", cp.GitRef); err != nil {
		// Not fatal if the tag is already deleted.
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("checkpoint delete: git tag -d: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM git_checkpoints WHERE id=? AND workspace=?`,
		id, s.wsKey); err != nil {
		return fmt.Errorf("checkpoint delete: db delete: %w", err)
	}
	return nil
}

// ErrNotAGitRepo is returned when the workspace is not inside a git
// work tree and git-based checkpoints cannot be created.
var ErrNotAGitRepo = fmt.Errorf("workspace is not a git repository")
