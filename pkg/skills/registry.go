package skills

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	skillsDir string            // e.g., ~/.sin/skills or ./skills
	skills    map[string]*Skill // name -> Skill
	index     map[string]string // name -> path
	builtins  map[string]bool   // built-in skills
}

// NewRegistry creates a skill registry pointing to a directory.
func NewRegistry(skillsDir string) (*Registry, error) {
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return nil, err
	}
	r := &Registry{
		skillsDir: skillsDir,
		skills:    make(map[string]*Skill),
		index:     make(map[string]string),
		builtins:  make(map[string]bool),
	}
	if err := r.scan(); err != nil {
		return nil, err
	}
	return r, nil
}

// scan walks the skills directory and loads all SKILL.md files.
func (r *Registry) scan() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	err := filepath.WalkDir(r.skillsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "SKILL.md" {
			skill, err := ParseSkillFile(path)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			r.skills[skill.Name] = skill
			r.index[skill.Name] = path
		}
		return nil
	})
	return err
}

// Get returns a skill by name.
func (r *Registry) Get(name string) (*Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.skills[name]; ok {
		return s, nil
	}
	// Try to load fresh if not in memory
	path := filepath.Join(r.skillsDir, name, "SKILL.md")
	if _, err := os.Stat(path); err == nil {
		skill, err := ParseSkillFile(path)
		if err != nil {
			return nil, err
		}
		r.skills[skill.Name] = skill
		r.index[skill.Name] = path
		return skill, nil
	}
	return nil, fmt.Errorf("skill %q not found", name)
}

// List returns all skill names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.skills))
	for n := range r.skills {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Install clones or copies a skill repository into the registry.
func (r *Registry) Install(source string) error {
	// source can be a local path or git URL
	// For simplicity: if source is a directory containing SKILL.md, copy it
	// In real implementation, use `git clone` for URLs.
	// We assume source is a path to a skill directory.
	skillName := filepath.Base(source)
	targetDir := filepath.Join(r.skillsDir, skillName)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}
	srcFile := filepath.Join(source, "SKILL.md")
	dstFile := filepath.Join(targetDir, "SKILL.md")
	data, err := os.ReadFile(srcFile)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dstFile, data, 0644); err != nil {
		return err
	}
	return r.scan()
}

// Remove deletes a skill from the registry.
func (r *Registry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path, ok := r.index[name]
	if !ok {
		return fmt.Errorf("skill %s not installed", name)
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		return err
	}
	delete(r.skills, name)
	delete(r.index, name)
	return nil
}

// SaveIndex writes the registry index to disk for fast loading.
func (r *Registry) SaveIndex() error {
	data, err := json.MarshalIndent(r.index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.skillsDir, ".sin-skill-index.json"), data, 0644)
}
