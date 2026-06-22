// SPDX-License-Identifier: MIT
// Purpose: `sin instinct search` — RAG-backed top-N search over
// active instincts (issue #160). Embeds the query with a HashEmbedder
// (deterministic, dependency-free) and ranks the active instincts
// by cosine similarity. The first call lazy-builds the index; the
// JSON file lives at $SIN_CODE_HOME/instinct-embeddings.json.
//
// On any subsequent invocation, the index is rehydrated from disk,
// updated for any new or changed active instincts, and queried.
// The whole flow is bounded by the active-instinct count (typically
// <100), so it is fast even without ONNX.
//
// Docs: cli.doc.md (and cmd/sin-code/internal/rag/rag.doc.md)
package instinct

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/rag"
)

// instinctEmbeddingFile is the on-disk location of the embedding
// index, relative to the SIN-Code home dir. The path matches the
// other dotfile directories in the binary (sessions.db, lessons.db,
// goals.db, ledger.db) so the operator finds it in one place.
const instinctEmbeddingFile = "instinct-embeddings.json"

// embeddingFile is the JSON envelope persisted by the search index.
type embeddingFile struct {
	Version   int                  `json:"version"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	Entries   map[string][]float32 `json:"entries"`
}

// jsonPersister implements rag.Persister by writing/reading
// embeddingFile JSON at path.
type jsonPersister struct {
	path string
}

func (j *jsonPersister) Save(entries []rag.Entry) error {
	ef := embeddingFile{
		Version:   1,
		UpdatedAt: time.Now().UTC(),
		Entries:   map[string][]float32{},
	}
	for _, e := range entries {
		ef.Entries[e.ID] = e.Vector
	}
	b, err := json.MarshalIndent(ef, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: temp file + rename, so a crash mid-write
	// never leaves a half-written file behind.
	dir := filepath.Dir(j.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, b, filemode.Default()); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}

func (j *jsonPersister) Load() ([]rag.Entry, error) {
	b, err := os.ReadFile(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // first run, no file yet
		}
		return nil, err
	}
	var ef embeddingFile
	if err := json.Unmarshal(b, &ef); err != nil {
		return nil, err
	}
	out := make([]rag.Entry, 0, len(ef.Entries))
	for id, vec := range ef.Entries {
		out = append(out, rag.Entry{ID: id, Vector: vec})
	}
	return out, nil
}

// instinctSearchPath returns the path of the embedding index file
// for the current user. SIN_CODE_HOME is honored, then XDG_DATA_HOME,
// then ~/.local/share/sin-code/ (the same fallback the rest of the
// binary uses for sessions.db etc.).
func instinctSearchPath() (string, error) {
	if v := os.Getenv("SIN_CODE_HOME"); v != "" {
		return filepath.Join(v, instinctEmbeddingFile), nil
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "sin-code", instinctEmbeddingFile), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "sin-code", instinctEmbeddingFile), nil
}

// rebuildIndex ensures the rag.Index contains an entry for every
// active instinct. New / changed instincts are embedded; missing
// entries (deleted) are dropped. The function is idempotent and
// cheap: at <100 active instincts, embedding is microseconds.
func rebuildIndex(ctx context.Context, m *Manager, idx *rag.Index) error {
	active, err := m.Active()
	if err != nil {
		return err
	}
	e := rag.NewHashEmbedder()
	seen := map[string]bool{}
	for _, inst := range active {
		// Embed on the (Trigger + Action) of the instinct. Trigger
		// is the discriminator; Action is the consequence. The
		// cosine sim against a user query finds the instincts
		// that semantically match the user's request.
		text := inst.Trigger + " " + inst.Action
		vec, err := e.Embed(ctx, text)
		if err != nil {
			return err
		}
		idx.Set(inst.ID, vec)
		seen[inst.ID] = true
	}
	// Drop entries for instincts that are no longer active. The
	// index size is bounded by the active count.
	for _, id := range allIDs(idx) {
		if !seen[id] {
			idx.Delete(id)
		}
	}
	return nil
}

func allIDs(idx *rag.Index) []string { return idx.Keys() }

func cmdSearch() *cobra.Command {
	var limit int
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Top-N RAG search over active instincts (issue #160)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			m, err := mgr()
			if err != nil {
				return err
			}
			path, err := instinctSearchPath()
			if err != nil {
				return err
			}
			idx := rag.NewIndex(&jsonPersister{path: path})
			if err := rebuildIndex(ctx, m, idx); err != nil {
				return err
			}
			if err := idx.Persist(); err != nil {
				return err
			}
			ret := rag.NewRetriever(rag.NewHashEmbedder(), idx)
			hits, err := ret.TopN(ctx, args[0], limit)
			if err != nil {
				return err
			}
			if len(hits) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "No matching instincts.")
				return nil
			}
			// Map ids back to the Instinct structs for printing.
			byID := map[string]*Instinct{}
			active, _ := m.Active()
			for _, inst := range active {
				byID[inst.ID] = inst
			}
			fmt.Fprintf(c.OutOrStdout(), "Top %d instincts for %q (cosine-similarity, top-N from %d total):\n",
				len(hits), args[0], len(active))
			for i, h := range hits {
				inst, ok := byID[h.ID]
				if !ok {
					continue
				}
				line := fmt.Sprintf("  %d. [%.3f] %s — %s",
					i+1, h.Score, inst.ID, trim(inst.Trigger, 60))
				if strings.TrimSpace(inst.Action) != "" {
					line += "\n     → " + trim(inst.Action, 80)
				}
				fmt.Fprintln(c.OutOrStdout(), line)
			}
			return nil
		},
	}
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
