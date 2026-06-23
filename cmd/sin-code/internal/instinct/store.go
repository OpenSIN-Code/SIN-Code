// SPDX-License-Identifier: MIT
// Purpose: disk persistence — atomic Markdown writes in a project-scoped
// or global directory. Layout mirrors affaan-m/ecc for operator parity.
// Docs: store.doc.md
package instinct

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store persists instincts on disk, project-scoped + global.
//
// Layout:
//
//	<base>/global/instincts/<id>.md
//	<base>/projects/<project_id>/meta.json
//	<base>/projects/<project_id>/instincts/<id>.md
type Store struct {
	base string
}

// ResolveBaseDir precedence:
//  1. SIN_INSTINCT_DIR (absolute)
//  2. $XDG_DATA_HOME/sin-code/instinct
//  3. ~/.local/share/sin-code/instinct
func ResolveBaseDir() string {
	if v := os.Getenv("SIN_INSTINCT_DIR"); v != "" && filepath.IsAbs(v) {
		return v
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "sin-code", "instinct")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "sin-code", "instinct")
}

// NewStore returns a Store rooted at base (defaults to ResolveBaseDir).
func NewStore(base string) *Store {
	if base == "" {
		base = ResolveBaseDir()
	}
	return &Store{base: base}
}

// Base exposes the root for tests and the audit log path.
func (s *Store) Base() string { return s.base }

func (s *Store) globalDir() string { return filepath.Join(s.base, "global", "instincts") }
func (s *Store) projectDir(id string) string {
	return filepath.Join(s.base, "projects", id, "instincts")
}
func (s *Store) projectMetaPath(id string) string {
	return filepath.Join(s.base, "projects", id, "meta.json")
}

func (s *Store) dirFor(i *Instinct) string {
	if i.Scope == ScopeGlobal {
		return s.globalDir()
	}
	return s.projectDir(i.ProjectID)
}

// Save writes an instinct atomically (temp + rename). Same-process
// concurrency is handled by the per-call temp file; cross-process
// serialization is unnecessary because the SQLite-backed ledger and
// the modernc/sqlite build use a single writer goroutine in SIN-Code.
func (s *Store) Save(i *Instinct) error {
	if i.ID == "" {
		i.ID = i.computeID()
	}
	dir := s.dirFor(i)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := Marshal(i)
	if err != nil {
		return err
	}
	dst := filepath.Join(dir, i.ID+".md")
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// SaveProjectMeta records the human-readable project identity.
func (s *Store) SaveProjectMeta(p Project) error {
	if p.ID == "" {
		return nil
	}
	dir := filepath.Dir(s.projectMetaPath(p.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(s.projectMetaPath(p.ID), b, 0o644)
}

// LoadGlobal returns all global instincts.
func (s *Store) LoadGlobal() ([]*Instinct, error) { return loadDir(s.globalDir()) }

// LoadProject returns instincts for one project.
func (s *Store) LoadProject(id string) ([]*Instinct, error) { return loadDir(s.projectDir(id)) }

// LoadEffective returns the instincts that apply in a project: its own
// + all global ones, global winning on signature collisions.
func (s *Store) LoadEffective(projectID string) ([]*Instinct, error) {
	proj, err := s.LoadProject(projectID)
	if err != nil {
		return nil, err
	}
	glob, err := s.LoadGlobal()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]*Instinct, 0, len(proj)+len(glob))
	for _, g := range glob {
		seen[g.SignatureKey()] = true
		out = append(out, g)
	}
	for _, p := range proj {
		if seen[p.SignatureKey()] {
			continue
		}
		out = append(out, p)
	}
	SortByConfidence(out)
	return out, nil
}

// ListProjects enumerates known projects with their metadata.
func (s *Store) ListProjects() ([]Project, error) {
	root := filepath.Join(s.base, "projects")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := Project{ID: e.Name()}
		if b, err := os.ReadFile(s.projectMetaPath(e.Name())); err == nil {
			_ = json.Unmarshal(b, &p)
		}
		out = append(out, p)
	}
	return out, nil
}

// Delete removes an instinct file. Missing-file is a no-op.
func (s *Store) Delete(i *Instinct) error {
	path := filepath.Join(s.dirFor(i), i.ID+".md")
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// LoadAll returns every instinct across global + all projects.
func (s *Store) LoadAll() ([]*Instinct, error) {
	out, err := s.LoadGlobal()
	if err != nil {
		return nil, err
	}
	projects, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		pi, err := s.LoadProject(p.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, pi...)
	}
	return out, nil
}

func loadDir(dir string) ([]*Instinct, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Instinct
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		i, err := Unmarshal(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, i)
	}
	return out, nil
}
