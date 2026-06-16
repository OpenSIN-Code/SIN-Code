// SPDX-License-Identifier: MIT
// Purpose: on-disk persistence for PRPs. Files live in-repo under
// .sin/prp/<id>.md so the plan is reviewable and diffable.
// Docs: store.doc.md
package prp

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store persists PRPs under <workdir>/.sin/prp/<id>.md (in-repo,
// reviewable).
type Store struct{ dir string }

// NewStore returns a Store rooted at <workdir>/.sin/prp. Pass "" for
// the current working directory.
func NewStore(workdir string) *Store {
	if workdir == "" {
		workdir = "."
	}
	return &Store{dir: filepath.Join(workdir, ".sin", "prp")}
}

// Save writes a PRP atomically (temp + rename).
func (s *Store) Save(p *PRP) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := Marshal(p)
	if err != nil {
		return err
	}
	dst := filepath.Join(s.dir, p.ID+".md")
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// Load reads a PRP by ID.
func (s *Store) Load(id string) (*PRP, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, id+".md"))
	if err != nil {
		return nil, err
	}
	return Unmarshal(data)
}

// List returns all PRPs, newest-updated first.
func (s *Store) List() ([]*PRP, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*PRP
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		if p, err := s.Load(id); err == nil {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
