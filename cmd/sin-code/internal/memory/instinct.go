// SPDX-License-Identifier: MIT
// Purpose: instinct model with confidence scoring and project→global
// promotion (issue #348). SQLite-backed (modernc.org/sqlite, M2-safe).
// An instinct is an atomic behavioural rule whose confidence evolves
// with confirming/contradicting evidence. High-confidence project
// instincts auto-promote to global scope.
package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite, M2 (CGO_ENABLED=0)
)

const (
	instinctScopeProject = "project"
	instinctScopeGlobal  = "global"

	instinctInitialConfidence  = 0.3
	instinctConfirmIncrement   = 0.05
	instinctContradictDecrement = 0.10
	instinctPromoteThreshold   = 0.8
	instinctDemoteThreshold    = 0.5
	instinctMaxConfidence      = 1.0
)

var ErrInstinctNotFound = errors.New("instinct: not found")
var ErrInstinctLowConfidence = errors.New("instinct: confidence below promotion threshold")

// Instinct is an atomic behavioural rule with confidence scoring.
type Instinct struct {
	ID         string     `json:"id"`
	Content    string     `json:"content"`
	Confidence float64    `json:"confidence"`
	Scope      string     `json:"scope"`
	Source     string     `json:"source"`
	CreatedAt  time.Time  `json:"created_at"`
	PromotedAt *time.Time `json:"promoted_at,omitempty"`
}

// InstinctStore manages instincts with confidence scoring, backed by
// SQLite. The *sql.DB is inherently thread-safe (M7).
type InstinctStore struct {
	db *sql.DB
}

// NewInstinctStore creates the schema and returns a store wrapping the
// given *sql.DB. The caller is responsible for closing the DB.
func NewInstinctStore(db *sql.DB) (*InstinctStore, error) {
	if db == nil {
		return nil, fmt.Errorf("instinct: nil db")
	}
	schema := `
CREATE TABLE IF NOT EXISTS instincts (
    id          TEXT PRIMARY KEY,
    content     TEXT NOT NULL,
    confidence  REAL NOT NULL,
    scope       TEXT NOT NULL DEFAULT 'project',
    source      TEXT NOT NULL DEFAULT 'observation',
    created_at  TEXT NOT NULL,
    promoted_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_instincts_scope ON instincts(scope);
CREATE INDEX IF NOT EXISTS idx_instincts_confidence ON instincts(confidence DESC);
`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("instinct: create schema: %w", err)
	}
	return &InstinctStore{db: db}, nil
}

func instinctID(content string) string {
	h := sha256.Sum256([]byte(content))
	return "instinct-" + hex.EncodeToString(h[:12])
}

// Record creates a new instinct or increments the confidence of an
// existing one with the same content. New instincts start at 0.3;
// each subsequent recording adds 0.05, capped at 1.0.
func (s *InstinctStore) Record(ctx context.Context, content string) error {
	if content == "" {
		return fmt.Errorf("instinct: empty content")
	}
	id := instinctID(content)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO instincts (id, content, confidence, scope, source, created_at, promoted_at)
VALUES (?, ?, ?, 'project', 'observation', ?, NULL)
ON CONFLICT(id) DO UPDATE SET confidence = MIN(confidence + ?, 1.0)
`, id, content, instinctInitialConfidence, now, instinctConfirmIncrement)
	if err != nil {
		return fmt.Errorf("instinct: record: %w", err)
	}
	return nil
}

// Get retrieves a single instinct by ID.
func (s *InstinctStore) Get(ctx context.Context, id string) (*Instinct, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, content, confidence, scope, source, created_at, promoted_at
FROM instincts WHERE id = ?
`, id)
	inst, err := scanInstinct(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInstinctNotFound
		}
		return nil, fmt.Errorf("instinct: get: %w", err)
	}
	return inst, nil
}

// List returns instincts filtered by scope. An empty scope returns all.
func (s *InstinctStore) List(ctx context.Context, scope string) ([]Instinct, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if scope != "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT id, content, confidence, scope, source, created_at, promoted_at
FROM instincts WHERE scope = ? ORDER BY confidence DESC, created_at DESC
`, scope)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT id, content, confidence, scope, source, created_at, promoted_at
FROM instincts ORDER BY confidence DESC, created_at DESC
`)
	}
	if err != nil {
		return nil, fmt.Errorf("instinct: list: %w", err)
	}
	defer rows.Close()

	var out []Instinct
	for rows.Next() {
		inst, err := scanInstinct(rows)
		if err != nil {
			return nil, fmt.Errorf("instinct: scan: %w", err)
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// Promote moves a project-scoped instinct to global scope. The
// instinct's confidence must exceed 0.8.
func (s *InstinctStore) Promote(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("instinct: promote begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var confidence float64
	err = tx.QueryRowContext(ctx, `SELECT confidence FROM instincts WHERE id = ?`, id).Scan(&confidence)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInstinctNotFound
		}
		return fmt.Errorf("instinct: promote select: %w", err)
	}
	if confidence <= instinctPromoteThreshold {
		return fmt.Errorf("%w: %.2f <= %.2f", ErrInstinctLowConfidence, confidence, instinctPromoteThreshold)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE instincts SET scope = 'global', promoted_at = ? WHERE id = ?`, now, id); err != nil {
		return fmt.Errorf("instinct: promote update: %w", err)
	}
	return tx.Commit()
}

// Demote reduces an instinct's confidence. If confidence drops below
// 0.5 and the instinct was global, it is returned to project scope.
func (s *InstinctStore) Demote(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("instinct: demote begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var scope string
	err = tx.QueryRowContext(ctx, `SELECT scope FROM instincts WHERE id = ?`, id).Scan(&scope)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInstinctNotFound
		}
		return fmt.Errorf("instinct: demote select: %w", err)
	}
	newConf := fmt.Sprintf("MAX(confidence - %f, 0)", instinctContradictDecrement)
	if scope == instinctScopeGlobal {
		if _, err := tx.ExecContext(ctx, `
UPDATE instincts SET confidence = `+newConf+`,
    scope = CASE WHEN confidence - ? < ? THEN 'project' ELSE scope END,
    promoted_at = CASE WHEN confidence - ? < ? THEN NULL ELSE promoted_at END
WHERE id = ?`, instinctContradictDecrement, instinctDemoteThreshold,
			instinctContradictDecrement, instinctDemoteThreshold, id); err != nil {
			return fmt.Errorf("instinct: demote update: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE instincts SET confidence = `+newConf+` WHERE id = ?`, id); err != nil {
			return fmt.Errorf("instinct: demote update: %w", err)
		}
	}
	return tx.Commit()
}

type instinctScanner interface {
	Scan(dest ...any) error
}

func scanInstinct(sc instinctScanner) (*Instinct, error) {
	var (
		inst        Instinct
		createdStr  string
		promotedStr sql.NullString
	)
	if err := sc.Scan(&inst.ID, &inst.Content, &inst.Confidence, &inst.Scope, &inst.Source, &createdStr, &promotedStr); err != nil {
		return nil, err
	}
	created, err := time.Parse(time.RFC3339, createdStr)
	if err != nil {
		return nil, fmt.Errorf("instinct: parse created_at: %w", err)
	}
	inst.CreatedAt = created
	if promotedStr.Valid && promotedStr.String != "" {
		promoted, err := time.Parse(time.RFC3339, promotedStr.String)
		if err == nil {
			inst.PromotedAt = &promoted
		}
	}
	return &inst, nil
}
