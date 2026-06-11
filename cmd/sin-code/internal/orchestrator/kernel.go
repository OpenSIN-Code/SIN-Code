// SPDX-License-Identifier: MIT
// Purpose: Session Kernel — atomic repo+agent checkpoints with verified
// time-travel. Rewind restores BOTH the working tree AND the agent's
// state (episode cursor, leases, verdict log). "Rewind to last green"
// is a first-class operation, not file archaeology.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type AgentState struct {
	TaskID         string          `json:"task_id"`
	EpisodeCursor  int64           `json:"episode_cursor"`
	ScratchpadHash string          `json:"scratchpad_hash"`
	HeldLeases     []int64         `json:"held_leases"`
	LastVerdict    json.RawMessage `json:"last_verdict,omitempty"`
}

type Checkpoint struct {
	ID        int64
	Label     string
	TreeSHA   string
	State     AgentState
	Green     bool
	CreatedAt time.Time
}

type Kernel struct {
	db      *sql.DB
	Workdir string
}

func NewKernel(db *sql.DB, workdir string) (*Kernel, error) {
	if db == nil {
		return &Kernel{Workdir: workdir}, nil
	}
	const schema = `
CREATE TABLE IF NOT EXISTS checkpoints (
	id INTEGER PRIMARY KEY,
	label TEXT NOT NULL,
	tree_sha TEXT NOT NULL,
	state_json TEXT NOT NULL,
	green INTEGER NOT NULL,
	created_at TEXT NOT NULL
);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("kernel schema: %w", err)
	}
	return &Kernel{db: db, Workdir: workdir}, nil
}

func (k *Kernel) Capture(ctx context.Context, label string, state AgentState, green bool) (*Checkpoint, error) {
	if k == nil || k.Workdir == "" {
		return &Checkpoint{Label: label, State: state, Green: green, CreatedAt: timeNow()}, nil
	}

	tree, err := k.writeTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("kernel capture tree: %w", err)
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}

	if k.db == nil {
		return &Checkpoint{Label: label, TreeSHA: tree, State: state, Green: green, CreatedAt: timeNow()}, nil
	}

	res, err := k.db.ExecContext(ctx, `
		INSERT INTO checkpoints (label, tree_sha, state_json, green, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		label, tree, string(stateJSON), boolToInt(green), timeNow().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	return &Checkpoint{
		ID: id, Label: label, TreeSHA: tree, State: state,
		Green: green, CreatedAt: timeNow(),
	}, nil
}

func (k *Kernel) Rewind(ctx context.Context, checkpointID int64) (*AgentState, error) {
	if k == nil || k.db == nil {
		return nil, fmt.Errorf("kernel: no DB")
	}
	var treeSHA, stateJSON string
	err := k.db.QueryRowContext(ctx,
		`SELECT tree_sha, state_json FROM checkpoints WHERE id = ?`, checkpointID).
		Scan(&treeSHA, &stateJSON)
	if err != nil {
		return nil, fmt.Errorf("kernel rewind: checkpoint %d: %w", checkpointID, err)
	}

	if k.Workdir != "" && treeSHA != "" {
		if err := k.git(ctx, "read-tree", treeSHA); err != nil {
			return nil, fmt.Errorf("kernel read-tree: %w", err)
		}
		if err := k.git(ctx, "checkout-index", "-a", "-f"); err != nil {
			return nil, fmt.Errorf("kernel checkout-index: %w", err)
		}
		_ = k.git(ctx, "clean", "-fd")
	}

	var state AgentState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return nil, fmt.Errorf("kernel state decode: %w", err)
	}
	return &state, nil
}

func (k *Kernel) LastGreen(ctx context.Context) (int64, string, error) {
	if k == nil || k.db == nil {
		return 0, "", fmt.Errorf("kernel: no DB")
	}
	var id int64
	var label string
	err := k.db.QueryRowContext(ctx,
		`SELECT id, label FROM checkpoints WHERE green = 1 ORDER BY id DESC LIMIT 1`).
		Scan(&id, &label)
	if err == sql.ErrNoRows {
		return 0, "", fmt.Errorf("kernel: no green checkpoint exists")
	}
	return id, label, err
}

func (k *Kernel) Timeline(ctx context.Context, limit int) (string, error) {
	if k == nil || k.db == nil {
		return "", nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := k.db.QueryContext(ctx, `
		SELECT id, label, green, created_at FROM checkpoints
		ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString("## Session timeline (newest first)\n")
	for rows.Next() {
		var id int64
		var label, createdAt string
		var green int
		if err := rows.Scan(&id, &label, &green, &createdAt); err != nil {
			return "", err
		}
		mark := "red"
		if green == 1 {
			mark = "GREEN"
		}
		fmt.Fprintf(&b, "- #%d [%s] %s (%s)\n", id, mark, label, createdAt)
	}
	return b.String(), rows.Err()
}

func (k *Kernel) writeTree(ctx context.Context) (string, error) {
	if k.Workdir == "" {
		return "", nil
	}
	headIndex, _ := k.gitOut(ctx, "write-tree")
	if err := k.git(ctx, "add", "-A"); err != nil {
		return "", err
	}
	tree, err := k.gitOut(ctx, "write-tree")
	if err != nil {
		return "", err
	}
	if headIndex != "" {
		_ = k.git(ctx, "read-tree", headIndex)
	}
	return tree, nil
}

func (k *Kernel) git(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = k.Workdir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %s: %w", args, string(out), err)
	}
	return nil
}

func (k *Kernel) gitOut(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = k.Workdir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func HashScratchpad(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:8])
}
