// SPDX-License-Identifier: MIT
// Purpose: memory edit versioning history (issue #358).
// SQLite-backed (modernc.org/sqlite, M2-safe). Tracks every edit
// to a memory so old versions can be restored.
package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// MemoryVersion is a single snapshot of a memory's content at a
// point in time. Version numbers are per-memory, incrementing from 1.
type MemoryVersion struct {
	ID         string `json:"id"`
	MemoryID   string `json:"memory_id"`
	Content    string `json:"content"`
	Version    int    `json:"version"`
	EditedAt   string `json:"edited_at"`
	EditReason string `json:"edit_reason"`
}

// VersioningStore tracks edit history for memories, backed by SQLite.
// The *sql.DB is inherently thread-safe (M7).
type VersioningStore struct {
	db *sql.DB
}

// NewVersioningStore creates the schema and returns a store wrapping
// the given *sql.DB. The caller is responsible for closing the DB.
func NewVersioningStore(db *sql.DB) (*VersioningStore, error) {
	if db == nil {
		return nil, fmt.Errorf("versioning: nil db")
	}
	schema := `
CREATE TABLE IF NOT EXISTS memory_versions (
    id          TEXT PRIMARY KEY,
    memory_id   TEXT NOT NULL,
    content     TEXT NOT NULL,
    version     INTEGER NOT NULL,
    edited_at   TEXT NOT NULL,
    edit_reason TEXT
);
CREATE INDEX IF NOT EXISTS idx_versions_mem ON memory_versions(memory_id, version);
`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("versioning: create schema: %w", err)
	}
	return &VersioningStore{db: db}, nil
}

func versionID(memID string, version int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", memID, version)))
	return "ver-" + hex.EncodeToString(h[:12])
}

// SaveVersion records a snapshot of a memory's content before an edit.
// The oldContent is stored as the versioned snapshot; newContent is
// used only for the diff in the edit reason if reason is empty.
// Version numbers auto-increment per memory_id.
func (s *VersioningStore) SaveVersion(ctx context.Context, memID, oldContent, newContent, reason string) error {
	if memID == "" {
		return fmt.Errorf("versioning: memory id required")
	}
	if reason == "" {
		reason = "edit"
	}

	var maxVersion int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM memory_versions WHERE memory_id = ?`,
		memID,
	).Scan(&maxVersion)
	if err != nil {
		return fmt.Errorf("versioning: query max version: %w", err)
	}

	version := maxVersion + 1
	id := versionID(memID, version)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO memory_versions (id, memory_id, content, version, edited_at, edit_reason) VALUES (?, ?, ?, ?, ?, ?)`,
		id, memID, oldContent, version, now, reason,
	)
	if err != nil {
		return fmt.Errorf("versioning: insert version: %w", err)
	}
	return nil
}

// History returns all versions for a memory, ordered by version
// number descending (newest first).
func (s *VersioningStore) History(ctx context.Context, memID string) ([]MemoryVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, memory_id, content, version, edited_at, edit_reason FROM memory_versions WHERE memory_id = ? ORDER BY version DESC`,
		memID,
	)
	if err != nil {
		return nil, fmt.Errorf("versioning: query history: %w", err)
	}
	defer rows.Close()

	var out []MemoryVersion
	for rows.Next() {
		var v MemoryVersion
		if err := rows.Scan(&v.ID, &v.MemoryID, &v.Content, &v.Version, &v.EditedAt, &v.EditReason); err != nil {
			return nil, fmt.Errorf("versioning: scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Restore creates a new version entry containing the content of the
// specified version, effectively "restoring" it. The caller can then
// read the latest version's content and apply it to the memory store.
// Returns an error if the version does not exist.
func (s *VersioningStore) Restore(ctx context.Context, memID string, version int) error {
	var content string
	err := s.db.QueryRowContext(ctx,
		`SELECT content FROM memory_versions WHERE memory_id = ? AND version = ?`,
		memID, version,
	).Scan(&content)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("versioning: version %d not found for %s", version, memID)
		}
		return fmt.Errorf("versioning: query restore: %w", err)
	}

	return s.SaveVersion(ctx, memID, content, content, fmt.Sprintf("restore from version %d", version))
}

// Diff returns a textual diff between two versions of a memory.
// The output uses unified-diff-like format with "-" for lines only
// in v1 and "+" for lines only in v2.
func (s *VersioningStore) Diff(ctx context.Context, memID string, v1, v2 int) (string, error) {
	var content1, content2 string

	err := s.db.QueryRowContext(ctx,
		`SELECT content FROM memory_versions WHERE memory_id = ? AND version = ?`,
		memID, v1,
	).Scan(&content1)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("versioning: version %d not found for %s", v1, memID)
		}
		return "", fmt.Errorf("versioning: query diff v1: %w", err)
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT content FROM memory_versions WHERE memory_id = ? AND version = ?`,
		memID, v2,
	).Scan(&content2)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("versioning: version %d not found for %s", v2, memID)
		}
		return "", fmt.Errorf("versioning: query diff v2: %w", err)
	}

	return lineDiff(content1, content2), nil
}

// lineDiff produces a simple unified-diff-like string between two texts.
func lineDiff(old, new string) string {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	lcs := computeLCS(oldLines, newLines)

	var b strings.Builder
	i, j := 0, 0
	for k := 0; k < len(lcs); k++ {
		for i < len(oldLines) && oldLines[i] != lcs[k] {
			b.WriteString("- " + oldLines[i] + "\n")
			i++
		}
		for j < len(newLines) && newLines[j] != lcs[k] {
			b.WriteString("+ " + newLines[j] + "\n")
			j++
		}
		b.WriteString("  " + lcs[k] + "\n")
		i++
		j++
	}
	for i < len(oldLines) {
		b.WriteString("- " + oldLines[i] + "\n")
		i++
	}
	for j < len(newLines) {
		b.WriteString("+ " + newLines[j] + "\n")
		j++
	}
	return b.String()
}

// computeLCS returns the longest common subsequence of two string slices.
func computeLCS(a, b []string) []string {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}
	var lcs []string
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			lcs = append([]string{a[i-1]}, lcs...)
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	return lcs
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
