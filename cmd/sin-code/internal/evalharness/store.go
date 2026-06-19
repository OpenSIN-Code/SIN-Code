// SPDX-License-Identifier: MIT
// Purpose: persist EvalSets and Runs as JSON. Layout mirrors the
// instinct package for operator parity.
// Docs: store.doc.md
package evalharness

import (
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)


// Store persists runs and loads eval sets from disk.
//
// Layout:
//
//	<base>/sets/<name>.json     (EvalSet definitions)
//	<base>/runs/<run-id>.json   (recorded Run results)
type Store struct{ base string }

// ResolveBaseDir precedence: SIN_EVAL_DIR | XDG | ~/.local/share.
func ResolveBaseDir() string {
	if v := os.Getenv("SIN_EVAL_DIR"); v != "" && filepath.IsAbs(v) {
		return v
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "sin-code-eval")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "sin-code-eval")
}

func NewStore(base string) *Store {
	if base == "" {
		base = ResolveBaseDir()
	}
	return &Store{base: base}
}

// LoadSet reads an EvalSet by name (or by explicit file path).
func (s *Store) LoadSet(nameOrPath string) (EvalSet, error) {
	path := nameOrPath
	if filepath.Ext(path) == "" {
		path = filepath.Join(s.base, "sets", nameOrPath+".json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return EvalSet{}, err
	}
	var set EvalSet
	if err := json.Unmarshal(data, &set); err != nil {
		return EvalSet{}, err
	}
	return set, nil
}

// SaveSet writes an EvalSet.
func (s *Store) SaveSet(set EvalSet) error {
	dir := filepath.Join(s.base, "sets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(set, "", "  ")
	return os.WriteFile(filepath.Join(dir, set.Name+".json"), b, filemode.Default())
}

// SaveRun persists a completed run.
func (s *Store) SaveRun(run Run) error {
	dir := filepath.Join(s.base, "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(run, "", "  ")
	return os.WriteFile(filepath.Join(dir, run.ID+".json"), b, filemode.Default())
}

// LoadRun reads a run by ID.
func (s *Store) LoadRun(id string) (Run, error) {
	data, err := os.ReadFile(filepath.Join(s.base, "runs", id+".json"))
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, err
	}
	return run, nil
}

// ListRuns returns run IDs for a set (empty setName = all), newest first.
func (s *Store) ListRuns(setName string) ([]Run, error) {
	dir := filepath.Join(s.base, "runs")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var runs []Run
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(".json")]
		run, err := s.LoadRun(id)
		if err != nil {
			continue
		}
		if setName == "" || run.SetName == setName {
			runs = append(runs, run)
		}
	}
	sort.SliceStable(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	return runs, nil
}

// LatestRun returns the most recent run for a set, if any.
func (s *Store) LatestRun(setName string) (Run, bool, error) {
	runs, err := s.ListRuns(setName)
	if err != nil || len(runs) == 0 {
		return Run{}, false, err
	}
	return runs[0], true, nil
}
